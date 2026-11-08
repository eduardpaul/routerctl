// Package poller is the routerctl daemon: one poll loop per device, with
// per-part intervals and backoff on errors. Each poll is diffed into
// events (package events), recorded in the inventory, and fed to the
// sinks. Reconnection is left to the drivers.
package poller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerkit"
)

// DefaultTimeout is the request timeout assumed when a Device has none.
const DefaultTimeout = 30 * time.Second

// Device is one device with its driver and polling plan.
type Device struct {
	Name   string
	Driver routerkit.Driver
	// Intervals: how often each part is polled; a missing part or a
	// value <= 0 is not polled
	Intervals map[routerkit.Part]time.Duration
	// Timeout of one device request (0 = DefaultTimeout); a poll gets 4x
	Timeout time.Duration
}

// Presence debounces the client presence events.
type Presence struct {
	// OfflineGrace: a client must stay offline this long before
	// device_offline fires
	OfflineGrace time.Duration
	// OnlineDebounce: a client must stay online this long before
	// device_online fires
	OnlineDebounce time.Duration
}

// Options configure the daemon.
type Options struct {
	Presence Presence
	// Baseline: when the inventory starts empty, the first poll of each
	// device records its clients as known without device_new
	Baseline bool
	Store    *inventory.Store
	Sinks    []Sink
	Logger   *slog.Logger
	// Now is the clock (default time.Now)
	Now func() time.Time
	// BackoffMin / BackoffMax bound the retry delay after a failed poll
	// (defaults 5s / 5m)
	BackoffMin, BackoffMax time.Duration
	// SaveEvery bounds how often the state file is written when nothing
	// notable happened (default 1m)
	SaveEvery time.Duration
	// ActionRepoll: delay before re-polling after a successful action
	// (default 2s; negative disables)
	ActionRepoll time.Duration
}

// Daemon runs the poll loops.
type Daemon struct {
	opts    Options
	log     *slog.Logger
	devices map[string]*runner
	order   []string
	saveMu  sync.Mutex
	// event ids: <boot>-<seq>
	boot  string
	seqMu sync.Mutex
	seq   uint64
}

type runner struct {
	d   *Daemon
	dev Device
	log *slog.Logger
	// op serializes the device operations (poll, connect, actions) of a
	// SingleSession device
	op sync.Mutex

	mu       sync.Mutex
	caps     routerkit.Capabilities
	state    DeviceState
	engine   events.State
	baseline bool
	poke     chan struct{}
	lastSave time.Time
}

// reconnectAfter is the number of consecutive failed polls after which
// the daemon reconnects the driver explicitly.
const reconnectAfter = 3

// New builds the daemon. Sinks are used as given (wrap them with
// sink.Async to decouple slow outputs).
func New(devs []Device, opts Options) (*Daemon, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.BackoffMin <= 0 {
		opts.BackoffMin = 5 * time.Second
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = 5 * time.Minute
	}
	if opts.SaveEvery <= 0 {
		opts.SaveEvery = time.Minute
	}
	if opts.ActionRepoll == 0 {
		opts.ActionRepoll = 2 * time.Second
	}
	if opts.Store == nil {
		opts.Store, _ = inventory.Open("")
	}
	d := &Daemon{opts: opts, log: opts.Logger, devices: map[string]*runner{}, boot: bootID()}
	baseline := opts.Baseline && opts.Store.Len() == 0
	for _, dev := range devs {
		name := dev.Name
		if _, dup := d.devices[name]; dup {
			return nil, fmt.Errorf("duplicate device %q", name)
		}
		r := &runner{
			d: d, dev: dev, log: opts.Logger.With("device", name),
			baseline: baseline,
			poke:     make(chan struct{}, 1),
			state: DeviceState{
				Name: name, Driver: dev.Driver.Info().Name,
				PartErrors: map[routerkit.Part]uint64{},
			},
		}
		if raw := opts.Store.EngineState(name); len(raw) > 0 {
			if err := json.Unmarshal(raw, &r.engine); err != nil {
				r.log.Warn("ignoring unreadable event state", "err", err)
			}
		}
		if r.engine.Clients == nil {
			r.engine.Clients = map[string]events.ClientState{}
		}
		if r.engine.Nodes == nil {
			r.engine.Nodes = map[string]bool{}
		}
		d.devices[name] = r
		d.order = append(d.order, name)
	}
	sort.Strings(d.order)
	return d, nil
}

// Devices implements Control.
func (d *Daemon) Devices() []string { return append([]string(nil), d.order...) }

func (d *Daemon) runner(device string) (*runner, error) {
	r, ok := d.devices[device]
	if !ok {
		return nil, fmt.Errorf("%q: %w", device, ErrUnknownDevice)
	}
	return r, nil
}

// Do implements Control: run a bound action on a device. The action must
// be listed in the device capabilities; the arguments were bound by the
// caller (drivers bind them again, see routerkit.BindInvocation). The
// dangerous-action policy is service/commands' job.
func (d *Daemon) Do(ctx context.Context, device string, inv routerkit.Invocation) (routerkit.Result, error) {
	r, err := d.runner(device)
	if err != nil {
		return routerkit.Result{}, err
	}
	r.mu.Lock()
	caps := r.caps
	r.mu.Unlock()
	if len(caps.Actions) == 0 && len(caps.Parts) == 0 {
		return routerkit.Result{}, fmt.Errorf("%s: %w", device, ErrNotReady)
	}
	if _, ok := caps.Action(inv.Name); !ok {
		return routerkit.Result{}, fmt.Errorf("%s: unknown action %q: %w", device, inv.Name, routerkit.ErrUnsupported)
	}
	unlock := r.lockOp()
	out, err := r.dev.Driver.Do(ctx, inv)
	unlock()
	if err == nil {
		err = out.Err(inv.Name)
	}
	data := map[string]any{"action": inv.Name, "args": inv.Args.Strings(), "ok": err == nil}
	if src := sourceOf(ctx); src != "" {
		data["source"] = src
	}
	if id := requestOf(ctx); id != "" {
		data["request_id"] = id
	}
	if err != nil {
		data["error"] = err.Error()
	} else if out.Pending {
		data["pending"] = true
	}
	d.emit(events.Event{Type: events.ActionDone, Time: d.opts.Now(), Device: device, Data: data})
	if err == nil && d.opts.ActionRepoll > 0 {
		time.AfterFunc(d.opts.ActionRepoll, func() {
			select {
			case r.poke <- struct{}{}:
			default:
			}
		})
	}
	return out, err
}

// Get implements Control: read a native source (serialized with the
// polls of a single-session device).
func (d *Daemon) Get(ctx context.Context, device, source string) (any, error) {
	r, err := d.runner(device)
	if err != nil {
		return nil, err
	}
	gctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	unlock := r.lockOp()
	defer unlock()
	return r.dev.Driver.Get(gctx, source)
}

// State implements Control: the current state of a device.
func (d *Daemon) State(device string) (DeviceState, bool) {
	r, ok := d.devices[device]
	if !ok {
		return DeviceState{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyState(), true
}

// Inventory implements Control.
func (d *Daemon) Inventory() Inventory { return daemonInventory{d} }

// daemonInventory edits the store and saves the state file at once, so
// a running daemon and `routerctl devices` agree.
type daemonInventory struct{ d *Daemon }

func (i daemonInventory) List() []inventory.Entry { return i.d.opts.Store.List() }
func (i daemonInventory) Get(mac string) (inventory.Entry, bool) {
	return i.d.opts.Store.Get(mac)
}
func (i daemonInventory) Label(mac, label string) error {
	return i.save(i.d.opts.Store.Label(mac, label))
}
func (i daemonInventory) SetKnown(mac string, known bool) error {
	return i.save(i.d.opts.Store.SetKnown(mac, known))
}
func (i daemonInventory) Forget(mac string) error { return i.save(i.d.opts.Store.Forget(mac)) }

func (i daemonInventory) save(err error) error {
	if err != nil {
		return err
	}
	return i.d.save()
}

// Run starts the sinks and the poll loops and blocks until ctx is done.
// On return every sink is closed and the state file saved.
func (d *Daemon) Run(ctx context.Context) error {
	var started []Sink
	for _, s := range d.opts.Sinks {
		if err := s.Start(ctx, d); err != nil {
			for _, st := range started {
				_ = st.Close(context.Background())
			}
			return fmt.Errorf("sink %s: %w", s.Name(), err)
		}
		started = append(started, s)
	}
	var wg sync.WaitGroup
	for _, name := range d.order {
		r := d.devices[name]
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.loop(ctx)
		}()
	}
	<-ctx.Done()
	d.log.Info("shutting down")
	wg.Wait()

	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, r := range d.devices {
		r.saveEngine()
		if err := r.dev.Driver.Close(closeCtx); err != nil {
			r.log.Debug("driver close", "err", err)
		}
	}
	if err := d.save(); err != nil {
		errs = append(errs, err)
	}
	for _, s := range started {
		if err := s.Close(closeCtx); err != nil {
			errs = append(errs, fmt.Errorf("sink %s: %w", s.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// bootID is a short random id of this daemon run (event ids).
func bootID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano()&0xffffffff, 16)
	}
	return hex.EncodeToString(b[:])
}

func (d *Daemon) save() error {
	d.saveMu.Lock()
	defer d.saveMu.Unlock()
	if err := d.opts.Store.Save(); err != nil {
		d.log.Error("cannot save the state file", "path", d.opts.Store.Path(), "err", err)
		return err
	}
	return nil
}

func (d *Daemon) emit(e events.Event) {
	d.seqMu.Lock()
	d.seq++
	e.ID = fmt.Sprintf("%s-%d", d.boot, d.seq)
	d.seqMu.Unlock()
	if e.Summary == "" {
		e.Summary = e.Describe()
	}
	e.Time = e.Time.UTC()
	d.log.Info("event", "type", string(e.Type), "device", e.Device, "data", e.Data)
	for _, s := range d.opts.Sinks {
		s.Event(e)
	}
}

func (d *Daemon) publish(st DeviceState) {
	for _, s := range d.opts.Sinks {
		s.State(st)
	}
}

// ---- per-device loop ----

// lockOp serializes device operations when the device allows a single
// session (Capabilities.SingleSession); it returns the unlock function.
func (r *runner) lockOp() func() {
	r.mu.Lock()
	single := r.caps.SingleSession
	r.mu.Unlock()
	if !single {
		return func() {}
	}
	r.op.Lock()
	return r.op.Unlock
}

// parts returns the polling plan: the configured parts the device
// supports, each at least Capabilities.MinInterval apart.
func (r *runner) parts() map[routerkit.Part]time.Duration {
	r.mu.Lock()
	caps := r.caps
	r.mu.Unlock()
	out := map[routerkit.Part]time.Duration{}
	for _, p := range caps.Filter(routerkit.AllParts) {
		if iv := r.dev.Intervals[p]; iv > 0 {
			out[p] = max(iv, caps.MinInterval)
		}
	}
	return out
}

func (r *runner) minInterval() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.caps.MinInterval
}

// configured reports whether any part is configured to be polled.
func (r *runner) configured() bool {
	for _, p := range routerkit.AllParts {
		if r.dev.Intervals[p] > 0 {
			return true
		}
	}
	return false
}

// sleep waits d (or until poked); alive is false once ctx is done.
func sleep(ctx context.Context, d time.Duration, poke <-chan struct{}) (alive, poked bool) {
	if d <= 0 {
		return ctx.Err() == nil, false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, false
	case <-t.C:
		return true, false
	case <-poke:
		return true, true
	}
}

func (r *runner) loop(ctx context.Context) {
	opts := r.d.opts
	if !r.configured() {
		r.log.Warn("every part is disabled; nothing to poll")
		return
	}

	// Connect (with backoff) and read the identity once
	backoff := opts.BackoffMin
	for {
		err := r.connect(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return
		}
		r.log.Warn("cannot connect", "err", err, "retry_in", backoff.String())
		r.markOffline(err)
		if alive, _ := sleep(ctx, backoff, nil); !alive {
			return
		}
		backoff = min(backoff*2, opts.BackoffMax)
	}
	backoff = opts.BackoffMin
	parts := r.parts()
	if len(parts) == 0 {
		r.log.Warn("the device supports none of the configured parts; nothing to poll")
		<-ctx.Done()
		return
	}
	for p := range r.dev.Intervals {
		if _, ok := parts[p]; !ok && r.dev.Intervals[p] > 0 {
			r.log.Info("part not supported by the device; not polled", "part", string(p))
		}
	}
	// Consecutive failed polls; past reconnectAfter the loop reconnects
	// explicitly (drivers recover expired sessions themselves, but not
	// e.g. a latched credentials refusal). Rate-limited by the backoff.
	failures := 0

	next := map[routerkit.Part]time.Time{}
	now := opts.Now()
	for p := range parts {
		next[p] = now
	}
	var lastPoll time.Time
	for ctx.Err() == nil {
		now = opts.Now()
		var due []routerkit.Part
		for _, p := range routerkit.AllParts {
			if t, ok := next[p]; ok && !t.After(now) {
				due = append(due, p)
			}
		}
		// Capabilities.MinInterval: never poll sooner than that after the
		// last poll (backoffs and action re-polls included)
		var gate time.Time
		if mi := r.minInterval(); len(due) > 0 && mi > 0 && !lastPoll.IsZero() && now.Sub(lastPoll) < mi {
			due, gate = nil, lastPoll.Add(mi)
		}
		if len(due) > 0 {
			lastPoll = now
			err := r.poll(ctx, due)
			now = opts.Now()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				r.log.Warn("poll failed", "err", err, "retry_in", backoff.String())
				for _, p := range due {
					next[p] = now.Add(backoff)
				}
				backoff = min(backoff*2, opts.BackoffMax)
				failures++
				if failures >= reconnectAfter {
					if cerr := r.connect(ctx); cerr != nil {
						if ctx.Err() != nil {
							return
						}
						r.log.Warn("reconnect failed", "err", cerr)
					} else {
						r.log.Info("reconnected after repeated poll failures")
						failures = 0
						for _, p := range due {
							next[p] = now
						}
					}
				}
			} else {
				failures = 0
				backoff = opts.BackoffMin
				for _, p := range due {
					next[p] = now.Add(parts[p])
				}
			}
		}
		wait := time.Duration(-1)
		for _, t := range next {
			if w := t.Sub(now); wait < 0 || w < wait {
				wait = w
			}
		}
		if g := gate.Sub(now); !gate.IsZero() && g > wait {
			wait = g
		}
		alive, poked := sleep(ctx, wait, r.poke)
		if !alive {
			return
		}
		if poked {
			// An action ran: refresh every part now
			for p := range next {
				next[p] = opts.Now()
			}
		}
	}
}

func (r *runner) connect(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	unlock := r.lockOp()
	defer unlock()
	if err := r.dev.Driver.Connect(cctx); err != nil {
		return err
	}
	id, err := r.dev.Driver.Identity(cctx)
	if err != nil {
		r.log.Warn("cannot read the identity", "err", err)
	}
	caps, err := r.dev.Driver.Capabilities(cctx)
	if err != nil {
		r.log.Warn("cannot read the capabilities", "err", err)
	}
	r.mu.Lock()
	r.caps = caps
	r.state.Identity = id
	r.state.Driver = r.dev.Driver.Info().Name
	r.state.Capabilities = caps
	r.mu.Unlock()
	r.log.Info("connected", "vendor", id.Vendor, "model", id.Model, "firmware", id.Firmware)
	return nil
}

func (r *runner) timeout() time.Duration {
	t := r.dev.Timeout
	if t <= 0 {
		t = DefaultTimeout
	}
	// A poll reads several parts; give it room
	return 4 * t
}

func (r *runner) markOffline(err error) {
	r.mu.Lock()
	r.state.Online = false
	r.state.LastError = err.Error()
	r.state.PollFailures++
	st := r.copyState()
	r.mu.Unlock()
	r.d.publish(st)
}

// copyState snapshots the state for sinks (r.mu held).
func (r *runner) copyState() DeviceState {
	st := r.state
	st.PartErrors = make(map[routerkit.Part]uint64, len(r.state.PartErrors))
	for k, v := range r.state.PartErrors {
		st.PartErrors[k] = v
	}
	st.Presence = make(map[string]bool, len(r.engine.Clients))
	for mac, c := range r.engine.Clients {
		st.Presence[mac] = c.Reported
	}
	st.Inventory = r.d.opts.Store.Snapshot()
	return st
}

// merge folds a partial snapshot into the latest one.
func merge(latest *routerkit.Snapshot, s routerkit.Snapshot, polled []routerkit.Part) {
	latest.Time = s.Time
	for _, p := range polled {
		if pe, failed := s.Errors[p]; failed {
			if latest.Errors == nil {
				latest.Errors = map[routerkit.Part]routerkit.PartError{}
			}
			latest.Errors[p] = pe
			continue
		}
		delete(latest.Errors, p)
		switch p {
		case routerkit.PartClients:
			if s.Clients != nil {
				latest.Clients = s.Clients
			}
		case routerkit.PartWAN:
			if s.WAN != nil {
				latest.WAN = s.WAN
			}
		case routerkit.PartSystem:
			if s.System != nil {
				latest.System = s.System
			}
		case routerkit.PartTraffic:
			if s.Traffic != nil {
				latest.Traffic = s.Traffic
			}
		case routerkit.PartNodes:
			if s.Nodes != nil {
				latest.Nodes = s.Nodes
			}
		case routerkit.PartFirmware:
			if s.Firmware != nil {
				latest.Firmware = s.Firmware
			}
		}
	}
	if len(latest.Errors) == 0 {
		latest.Errors = nil
	}
}

func (r *runner) poll(ctx context.Context, parts []routerkit.Part) error {
	opts := r.d.opts
	pctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	unlock := r.lockOp()
	start := time.Now()
	snap, err := r.dev.Driver.Poll(pctx, routerkit.PollRequest{Parts: parts})
	dur := time.Since(start)
	unlock()
	if err != nil {
		r.mu.Lock()
		r.state.PollDuration = dur
		r.mu.Unlock()
		r.markOffline(err)
		return err
	}
	now := opts.Now()
	if snap.Time.IsZero() {
		snap.Time = now
	}
	for p, pe := range snap.Errors {
		r.log.Debug("part failed", "part", string(p), "code", string(pe.Code), "err", pe.Message)
	}
	// Normalize client MACs once (the key everywhere)
	for i := range snap.Clients {
		if m := routerkit.NormalizeMAC(snap.Clients[i].MAC); m != "" {
			snap.Clients[i].MAC = m
		}
	}

	r.mu.Lock()
	baseline := r.baseline
	cfg := events.Config{
		OfflineGrace:   opts.Presence.OfflineGrace,
		OnlineDebounce: opts.Presence.OnlineDebounce,
		Baseline:       baseline,
	}
	inv := opts.Store.Snapshot()
	next, evs := events.Diff(r.engine, snap, inv, now, cfg, r.dev.Name)
	r.engine = next
	var added []string
	_, clientsFailed := snap.Errors[routerkit.PartClients]
	if snap.Clients != nil && !clientsFailed {
		added = opts.Store.Observe(r.dev.Name, snap.Clients, now, baseline)
		if baseline {
			r.baseline = false
			r.log.Info("inventory baseline recorded", "clients", len(added))
		}
	}
	merge(&r.state.Snapshot, snap, parts)
	r.state.Online = true
	r.state.LastError = ""
	r.state.LastPoll = now
	r.state.Polls++
	r.state.PollDuration = dur
	for p := range snap.Errors {
		r.state.PartErrors[p]++
	}
	st := r.copyState()
	saveNow := len(evs) > 0 || len(added) > 0 || now.Sub(r.lastSave) >= opts.SaveEvery
	if saveNow {
		r.lastSave = now
	}
	r.mu.Unlock()

	if saveNow {
		r.saveEngine()
		_ = r.d.save()
	}
	for _, e := range evs {
		r.d.emit(e)
	}
	r.d.publish(st)
	return nil
}

func (r *runner) saveEngine() {
	r.mu.Lock()
	raw, err := json.Marshal(r.engine)
	r.mu.Unlock()
	if err == nil {
		r.d.opts.Store.SetEngineState(r.dev.Name, raw)
	}
}
