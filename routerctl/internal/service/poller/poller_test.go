package poller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

const (
	laptop = "02:00:00:00:10:01"
	phone  = "02:00:00:00:10:02"
	office = "02:00:00:00:00:02"
)

// memSink records what the daemon feeds it (non-blocking).
type memSink struct {
	mu     sync.Mutex
	events []events.Event
	states []DeviceState
	cmd    Control
	notify chan events.Event
}

func (m *memSink) Name() string { return "memory" }

func (m *memSink) Start(_ context.Context, cmd Control) error {
	m.mu.Lock()
	m.cmd = cmd
	m.mu.Unlock()
	return nil
}

func (m *memSink) State(st DeviceState) {
	m.mu.Lock()
	m.states = append(m.states, st)
	m.mu.Unlock()
}

func (m *memSink) Event(e events.Event) {
	m.mu.Lock()
	m.events = append(m.events, e)
	m.mu.Unlock()
	select {
	case m.notify <- e:
	default:
	}
}

func (m *memSink) Close(context.Context) error { return nil }

func (m *memSink) Events() []events.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]events.Event(nil), m.events...)
}

func (m *memSink) States() []DeviceState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DeviceState(nil), m.states...)
}

func testDevice(drv routerkit.Driver) Device {
	iv := 15 * time.Millisecond
	return Device{
		Name: "home", Driver: drv, Timeout: time.Second,
		Intervals: map[routerkit.Part]time.Duration{
			routerkit.PartClients: iv, routerkit.PartWAN: iv, routerkit.PartSystem: iv,
			routerkit.PartTraffic: iv, routerkit.PartNodes: iv, routerkit.PartFirmware: iv,
		},
	}
}

type harness struct {
	t      *testing.T
	fake   *fake.Fake
	mem    *memSink
	ch     chan events.Event
	cancel context.CancelFunc
	done   chan error
	d      *Daemon
	store  *inventory.Store
}

func start(t *testing.T, statePath string, f *fake.Fake) *harness {
	t.Helper()
	store, err := inventory.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan events.Event, 100)
	mem := &memSink{notify: ch}
	d, err := New([]Device{testDevice(f)}, Options{
		Presence:     Presence{OfflineGrace: 60 * time.Millisecond},
		Baseline:     true,
		Store:        store,
		Sinks:        []Sink{mem},
		BackoffMin:   10 * time.Millisecond,
		BackoffMax:   20 * time.Millisecond,
		ActionRepoll: -1,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{t: t, fake: f, mem: mem, ch: ch, cancel: cancel, done: make(chan error, 1), d: d, store: store}
	go func() { h.done <- d.Run(ctx) }()
	return h
}

func (h *harness) stop() {
	h.cancel()
	select {
	case err := <-h.done:
		if err != nil {
			h.t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		h.t.Fatal("Run did not return after cancel")
	}
}

// expect waits for an event of type typ (other events are skipped).
func (h *harness) expect(typ events.Type) events.Event {
	h.t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case e := <-h.ch:
			if e.Type == typ {
				return e
			}
		case <-timeout:
			h.t.Fatalf("no %s event", typ)
		}
	}
}

func (h *harness) waitState(cond func(DeviceState) bool) DeviceState {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := h.d.State("home"); ok && cond(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatal("state condition not reached")
	return DeviceState{}
}

func TestWatchEndToEnd(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	f := fake.New()
	h := start(t, statePath, f)

	// First poll: baseline, every part present
	st := h.waitState(func(st DeviceState) bool {
		s := st.Snapshot
		return st.Online && s.Clients != nil && s.WAN != nil && s.System != nil && s.Traffic != nil && s.Nodes != nil && s.Firmware != nil
	})
	if st.Identity.Model != "FAKE-1000" || len(st.Capabilities.Actions) == 0 {
		t.Fatalf("identity/actions: %+v", st.Identity)
	}
	if e, ok := h.store.Get(laptop); !ok || !e.Known {
		t.Fatalf("baseline entry: %+v", e)
	}

	// New device
	f.UpsertClient(routerkit.Client{MAC: "02:00:00:00:20:01", IP: "192.168.50.99", Name: "visitor", Online: true})
	e := h.expect(events.DeviceNew)
	if e.Device != "home" || e.Data["mac"] != "02:00:00:00:20:01" || e.Data["known"] != false {
		t.Errorf("device_new: %+v", e)
	}

	// Presence with grace
	f.SetClientOnline(phone, false)
	begin := time.Now()
	e = h.expect(events.DeviceOffline)
	if e.Data["mac"] != phone {
		t.Errorf("device_offline: %+v", e)
	}
	if time.Since(begin) < 50*time.Millisecond {
		t.Errorf("offline fired before the grace: %v", time.Since(begin))
	}
	h.waitState(func(st DeviceState) bool { p, ok := st.Presence[phone]; return ok && !p })
	f.SetClientOnline(phone, true)
	h.expect(events.DeviceOnline)

	// IP change of a known device
	c := f.Clients()[0]
	c.IP = "192.168.50.77"
	f.UpsertClient(c)
	e = h.expect(events.DeviceIPChanged)
	if e.Data["old_ip"] != "192.168.50.10" || e.Data["new_ip"] != "192.168.50.77" {
		t.Errorf("ip change: %+v", e)
	}

	// WAN
	f.SetWAN(false, "")
	h.expect(events.WANDown)
	f.SetWAN(true, "203.0.113.8")
	h.expect(events.WANUp)
	e = h.expect(events.WANIPChanged)
	if e.Data["new_ip"] != "203.0.113.8" {
		t.Errorf("wan ip: %+v", e)
	}

	// Firmware and mesh
	f.SetFirmwareUpdate("1.1.0")
	h.expect(events.FirmwareUpdate)
	f.SetNodeOnline(office, false)
	h.expect(events.NodeOffline)
	f.SetNodeOnline(office, true)
	h.expect(events.NodeOnline)

	// Commands go through the daemon (as the MQTT sink would)
	inv := func(name string, args map[string]string) routerkit.Invocation {
		return routerkit.Invocation{Name: name, Args: routerkit.RawArgs(args)}
	}
	if _, err := h.mem.cmd.Do(WithRequestID(WithSource(context.Background(), "test"), "r-9"), "home", inv("led", map[string]string{"state": "off"})); err != nil {
		t.Fatal(err)
	}
	done := h.expect(events.ActionDone)
	if done.Data["ok"] != true || done.Data["source"] != "test" || done.Data["request_id"] != "r-9" || done.Data["action"] != "led" || done.ID == "" || done.Summary != "Action led done on home" {
		t.Errorf("action_done: %+v", done)
	}
	if _, err := h.mem.cmd.Do(context.Background(), "home", inv("led", map[string]string{"state": "dim"})); err == nil {
		t.Error("invalid args accepted")
	}
	if failed := h.expect(events.ActionDone); failed.Data["ok"] != false || failed.Data["error"] == "" {
		t.Errorf("failed action_done: %+v", failed)
	}
	if _, err := h.mem.cmd.Do(context.Background(), "home", inv("no-such", nil)); !errors.Is(err, routerkit.ErrUnsupported) {
		t.Errorf("unlisted action: %v", err)
	}
	if _, err := h.mem.cmd.Do(context.Background(), "nope", inv("led", nil)); !errors.Is(err, ErrUnknownDevice) {
		t.Errorf("unknown device: %v", err)
	}
	if v, err := h.mem.cmd.Get(context.Background(), "home", "fake_led"); err != nil || v.(map[string]any)["state"] != false {
		t.Errorf("get: %v %v", v, err)
	}
	if err := h.mem.cmd.Inventory().Label(laptop, "Work laptop"); err != nil {
		t.Fatal(err)
	}
	if e, _ := h.mem.cmd.Inventory().Get(laptop); e.Label != "Work laptop" || !e.Known {
		t.Errorf("label: %+v", e)
	}
	if devs := h.mem.cmd.Devices(); len(devs) != 1 || devs[0] != "home" {
		t.Errorf("devices: %v", devs)
	}
	if calls := f.Calls(); len(calls) != 1 || calls[0].Name != "led" || calls[0].Args["state"] != "off" {
		t.Errorf("calls: %+v", calls)
	}

	// Unreachable device: offline state, backoff, recovery; no client events
	f.SetUnreachable(fake.ErrUnreachable)
	h.waitState(func(st DeviceState) bool { return !st.Online && st.LastError != "" })
	f.SetUnreachable(nil)
	st = h.waitState(func(st DeviceState) bool { return st.Online })
	if st.PollFailures == 0 {
		t.Error("poll failures not counted")
	}

	// A failing part is reported, not fatal
	f.SetPartError(routerkit.PartSystem, "boom")
	h.waitState(func(st DeviceState) bool {
		return st.Snapshot.Errors[routerkit.PartSystem].Message == "boom" && st.PartErrors[routerkit.PartSystem] > 0
	})
	f.SetPartError(routerkit.PartSystem, "")
	h.stop()

	// Nothing unexpected happened along the way
	for _, e := range h.mem.Events() {
		if e.Type == events.DeviceOffline && e.Data["mac"] != phone {
			t.Errorf("unexpected offline: %+v", e)
		}
	}
	if len(h.mem.States()) == 0 {
		t.Error("no states delivered")
	}

	// Restart on the same state file: no replays of old events
	h2 := start(t, statePath, f)
	h2.waitState(func(st DeviceState) bool {
		return st.Online && st.Snapshot.Firmware != nil && st.Snapshot.Clients != nil
	})
	time.Sleep(80 * time.Millisecond)
	h2.stop()
	for _, e := range h2.mem.Events() {
		t.Errorf("event after restart: %s %+v", e.Type, e.Data)
	}
	if e, _ := h2.store.Get("02:00:00:00:20:01"); e.Known {
		t.Error("the visitor should still be unknown")
	}
}

func TestConnectRetry(t *testing.T) {
	f := fake.New()
	f.SetUnreachable(errors.New("no route"))
	h := start(t, "", f)
	h.waitState(func(st DeviceState) bool { return !st.Online && st.PollFailures >= 2 })
	f.SetUnreachable(nil)
	h.waitState(func(st DeviceState) bool { return st.Online })
	h.stop()
}

// A failure that only an explicit reconnect clears (e.g. the ASUS latched
// credentials refusal) must not take the device offline for good.
func TestReconnectAfterRepeatedPollFailures(t *testing.T) {
	f := fake.New()
	h := start(t, "", f)
	h.waitState(func(st DeviceState) bool { return st.Online })
	before := f.Connects()
	f.Latch(errors.New("credentials were refused before"))
	h.waitState(func(st DeviceState) bool { return !st.Online })
	h.waitState(func(st DeviceState) bool { return st.Online })
	if f.Connects() <= before {
		t.Errorf("connects = %d, want a reconnect after %d", f.Connects(), before)
	}
	h.stop()
}

// Parts the device does not support are never requested, and the
// capabilities reach the sinks.
func TestUnsupportedPartsNotPolled(t *testing.T) {
	f := fake.New()
	f.SetUnsupported(routerkit.PartNodes)
	h := start(t, "", f)
	st := h.waitState(func(st DeviceState) bool { return st.Online && st.Polls >= 3 })
	h.stop()
	if n := f.Requested()[routerkit.PartNodes]; n != 0 {
		t.Errorf("nodes requested %d times", n)
	}
	if f.Requested()[routerkit.PartWAN] == 0 {
		t.Error("wan never requested")
	}
	if st.Snapshot.Nodes != nil || st.Supports(routerkit.PartNodes) || !st.Supports(routerkit.PartWAN) {
		t.Errorf("state %+v", st.Capabilities.Parts)
	}
	if _, ok := st.Capabilities.Action("led"); !ok {
		t.Error("actions missing from the state")
	}
}

// Capabilities.MinInterval spaces the polls of a device whatever the
// configured intervals.
func TestMinInterval(t *testing.T) {
	f := fake.New()
	f.SetSchedulingHints(60*time.Millisecond, true)
	h := start(t, "", f)
	h.waitState(func(st DeviceState) bool { return st.Online })
	time.Sleep(300 * time.Millisecond)
	h.stop()
	// 15ms intervals would give ~20 polls; 60ms spacing at most ~6
	if n := f.Polls(); n < 2 || n > 7 {
		t.Errorf("polls = %d, want 2..7 with a 60ms minimum interval", n)
	}
}
