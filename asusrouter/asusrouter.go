// Package asusrouter is the Go port of the Python `asusrouter` library
// (asusrouter/asusrouter.py). See asusrouter/PARITY.md for the mapping rules.
package asusrouter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

const (
	// Delay before retrying a request after an authorization error
	authRetryDelay = 1 * time.Second
	// Reboot recovery timings
	rebootRecoveryInitialDelay = 30 * time.Second
	rebootRecoveryInterval     = 10 * time.Second
	rebootRecoveryTimeout      = 300 * time.Second
)

// Options are the keyword arguments of `AsusRouter.__init__`.
type Options struct {
	Port             int
	UseSSL           bool
	Connection       ConnectionOptions
	Config           map[config.ARConfigKey]any
	ConnectionConfig map[config.ARConnectionConfigKey]any
	// Transport replaces the HTTP connection (session replay, tests)
	Transport Transport
}

// AsusRouter mirrors `AsusRouter`: the interface class.
type AsusRouter struct {
	config         *config.ARInstanceConfig
	username       string
	cacheThreshold time.Duration

	mu         sync.Mutex
	dataStates map[source.ID]*source.ARDataState
	// Endpoints that returned 404
	unavailable map[endpoint.AREndpoint]bool

	identityMu sync.Mutex
	identity   *device.ARDeviceIdentity

	// In-flight reboot recovery; holds all requests until it finishes
	recovery chan struct{}
	// Last commanded LED state, reasserted after a reboot (for Merlin)
	ledState *bool

	conn      *Connection
	transport Transport
}

// New mirrors `AsusRouter.__init__`.
func New(hostname, username, password string, opts Options) (*AsusRouter, error) {
	slog.Debug("Initializing a new interface", "host", identifiers.NewHostname(hostname))
	r := &AsusRouter{
		config:         config.NewARInstanceConfig(opts.Config),
		username:       username,
		cacheThreshold: consts.DefaultCacheTime,
		dataStates:     map[source.ID]*source.ARDataState{},
		unavailable:    map[endpoint.AREndpoint]bool{},
		identity:       device.NewARDeviceIdentity(),
	}
	if opts.Transport != nil {
		r.transport = opts.Transport
		return r, nil
	}
	copts := opts.Connection
	if opts.Port != 0 {
		copts.Port = opts.Port
	}
	copts.UseSSL = copts.UseSSL || opts.UseSSL
	if copts.Config == nil {
		copts.Config = opts.ConnectionConfig
	}
	conn, err := NewConnection(hostname, username, password, copts)
	if err != nil {
		return nil, err
	}
	r.conn = conn
	r.transport = conn
	return r, nil
}

// ---------------------------
// Properties -->
// ---------------------------

// Description mirrors `description`: the device identity, i.e. the
// content of the ARDeviceSource state (an empty identity before the
// first successful connect).
func (r *AsusRouter) Description() *device.ARDeviceIdentity {
	r.mu.Lock()
	st := r.dataStates[source.IDOf(deviceSourceUniversal())]
	r.mu.Unlock()
	if st != nil {
		if id, ok := st.Content().(*device.ARDeviceIdentity); ok && id != nil {
			return id
		}
	}
	r.identityMu.Lock()
	defer r.identityMu.Unlock()
	return r.identity
}

// Connected mirrors `connected`.
func (r *AsusRouter) Connected() bool { return r.transport.Connected() }

// Config mirrors `config`.
func (r *AsusRouter) Config() *config.ARInstanceConfig { return r.config }

// ConnectionConfig mirrors `connection_config` (nil for custom transports).
func (r *AsusRouter) ConnectionConfig() *config.ARConnectionConfig {
	if r.conn == nil {
		return config.NewARConnectionConfig()
	}
	return r.conn.Config()
}

// Webpanel mirrors `webpanel`.
func (r *AsusRouter) Webpanel() string {
	if r.conn == nil {
		return ""
	}
	return r.conn.Webpanel()
}

// SetCacheThreshold changes how long fetched data stays fresh.
func (r *AsusRouter) SetCacheThreshold(d time.Duration) { r.cacheThreshold = d }

// ---------------------------
// Connection-related methods -->
// ---------------------------

func (r *AsusRouter) ensureConnected(ctx context.Context) error {
	r.mu.Lock()
	recovering := r.recovery != nil
	r.mu.Unlock()
	if !recovering && !r.transport.Connected() {
		slog.Debug("Not connected yet; connecting before the request")
		_, err := r.Connect(ctx)
		return err
	}
	return nil
}

// Connect mirrors `async_connect`: connect and get the device identity.
func (r *AsusRouter) Connect(ctx context.Context) (bool, error) {
	ok, err := r.transport.Connect(ctx)
	if err != nil || !ok {
		return false, err
	}
	// Fresh connection — re-evaluate endpoint availability
	r.mu.Lock()
	r.unavailable = map[endpoint.AREndpoint]bool{}
	r.mu.Unlock()

	// Fetch the device description
	result, err := r.FetchData(ctx, deviceSourceUniversal(), true, nil)
	if err != nil {
		return false, err
	}
	if result != nil {
		// Inject username
		r.Description().UpdateUsername(identifiers.UsernameFromValueSafe(r.username))
		// Seed the live AiMesh topology before any user request
		if _, err := r.FetchData(ctx, aimeshSourceUniversal(), true, nil); err != nil {
			return false, err
		}
		// A seeded boot time anchors the first stabilization
		if seeded := seededBoottime(r.config); seeded != nil {
			r.Description().UpdateBoottime(seeded)
		}
		if _, err := r.FetchData(ctx, clockSourceUniversal(), true, nil); err != nil {
			return false, err
		}
	}
	return result != nil, nil
}

// Disconnect mirrors `async_disconnect`.
func (r *AsusRouter) Disconnect(ctx context.Context) (bool, error) {
	_, err := r.transport.Disconnect(ctx)
	return true, err
}

// Close mirrors `async_close`: disconnect and release the session.
func (r *AsusRouter) Close(ctx context.Context) error {
	if r.transport.Connected() {
		_, _ = r.Disconnect(ctx)
	}
	return r.transport.Close()
}

// Reconnect is an explicit reconnect (Go only): it re-allows logins the
// device refused before, drops the auth and connects again. Rate-limit
// calls - each one may be a login attempt.
func (r *AsusRouter) Reconnect(ctx context.Context) (bool, error) {
	if r.conn != nil {
		r.conn.ClearCredentialsRefused()
	}
	r.transport.ResetAuth()
	return r.Connect(ctx)
}

// SetCredentials mirrors `_async_set_credentials`.
func (r *AsusRouter) SetCredentials(ctx context.Context, username, password string) (bool, error) {
	if r.conn == nil {
		return false, errors.New("credentials cannot be changed on a custom transport")
	}
	return r.conn.SetCredentials(ctx, username, password)
}

func (r *AsusRouter) currentCredentials() (string, string) {
	if r.conn == nil {
		return r.username, ""
	}
	return r.conn.Username(), r.conn.Password()
}

// ---------------------------
// Request-related methods -->
// ---------------------------

// Fetch mirrors `async_fetch`: the raw content of an endpoint, nil when
// the endpoint is unavailable on this firmware.
func (r *AsusRouter) Fetch(ctx context.Context, ep endpoint.AREndpoint, request string) (*string, error) {
	// Hold every request while recovering from a reboot
	r.mu.Lock()
	recovery := r.recovery
	unavailable := r.unavailable[ep]
	r.mu.Unlock()
	if recovery != nil {
		select {
		case <-recovery:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Skip endpoints already known absent on this firmware
	if unavailable {
		slog.Debug("Endpoint is known unavailable, skipping", "endpoint", string(ep))
		return nil, nil
	}

	var payload *string
	if request != "" {
		payload = &request
	}
	requestType := endpoint.GetEndpointRequestType(ep)
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := r.transport.Query(ctx, ep, payload, requestType)
		if err == nil {
			// Legacy can return a 200 with redirect instead of 404
			if readers.IsRedirectPage(resp.Content) {
				r.markUnavailable(ep)
				return nil, nil
			}
			recordDump(ctx, ep, requestType, payload, resp.Content)
			content := resp.Content
			return &content, nil
		}
		if errors.Is(err, arerror.Err404) {
			r.markUnavailable(ep)
			return nil, nil
		}
		code, isAccess := arerror.AccessCode(err)
		if !isAccess || code != int(endpoint.ARAccessErrorAuthorization) || attempt > 0 {
			return nil, err
		}
		r.transport.ResetAuth()
		if err := sleepCtx(ctx, authRetryDelay); err != nil {
			return nil, err
		}
	}
	// Unreachable: the retry loop always returns
	return nil, nil
}

func (r *AsusRouter) markUnavailable(ep endpoint.AREndpoint) {
	slog.Debug("Endpoint not available, marking unavailable", "endpoint", string(ep))
	r.mu.Lock()
	r.unavailable[ep] = true
	r.mu.Unlock()
}

// Read mirrors `async_read`: fetch and parse content.
func (r *AsusRouter) Read(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
	content, err := r.Fetch(ctx, ep, request)
	if err != nil || content == nil {
		return map[string]any{}, err
	}
	normalized := converters.RawToStr(*content)
	if normalized == nil || *normalized == "" {
		return map[string]any{}, nil
	}
	return endpoint.GetEndpointReader(ep)(*normalized), nil
}

// ---------------------------
// Data pipeline -->
// ---------------------------

func (r *AsusRouter) state(src source.DataSource) *source.ARDataState {
	id := source.IDOf(src)
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.dataStates[id]
	if !ok {
		st = source.NewARDataState(src)
		r.dataStates[id] = st
	}
	return st
}

func (r *AsusRouter) newContext(kw registry.Kwargs, force bool) *registry.Context {
	return &registry.Context{
		Read:     r.Read,
		FetchRaw: r.Fetch,
		// Sub-fetches inherit this call's force
		FetchData: func(ctx context.Context, src source.DataSource, sub registry.Kwargs) (any, error) {
			return r.FetchData(ctx, src, force, sub)
		},
		FetchDataMulti: func(ctx context.Context, srcs []source.DataSource, sub registry.Kwargs) (map[source.ID]any, error) {
			return r.FetchDataMany(ctx, srcs, force, sub)
		},
		RunAction: func(ctx context.Context, a action.ARAction) (any, error) {
			return r.RunAction(ctx, a, nil)
		},
		Identity:         r.Description(),
		ConnectionConfig: r.ConnectionConfig(),
		Kwargs:           kw,
	}
}

// FetchData mirrors `async_fetch_data` for a single source: fresh (or
// cached, unless force) content of the source; nil when there is none.
func (r *AsusRouter) FetchData(ctx context.Context, src source.DataSource, force bool, kw registry.Kwargs) (any, error) {
	res, err := r.FetchDataMany(ctx, []source.DataSource{src}, force, kw)
	if err != nil || res == nil {
		return nil, err
	}
	return res[source.IDOf(src)], nil
}

// FetchDataMany mirrors `async_fetch_data` for a collection: fresh
// content keyed by source ID (only states fresh after the refresh).
func (r *AsusRouter) FetchDataMany(ctx context.Context, srcs []source.DataSource, force bool, kw registry.Kwargs) (map[source.ID]any, error) {
	if err := r.ensureConnected(ctx); err != nil {
		return nil, err
	}
	if len(srcs) == 0 {
		return nil, nil
	}

	states := make([]*source.ARDataState, 0, len(srcs))
	for _, s := range srcs {
		states = append(states, r.state(s))
	}
	if err := r.refreshStates(ctx, states, force, kw); err != nil {
		return nil, err
	}

	// Check for a reboot
	if r.Description().Rebooted() {
		r.handleReboot(ctx)
	}

	result := map[source.ID]any{}
	for _, st := range states {
		// Only fresh states, like Python (even when forced): a forced
		// refresh that committed nothing must not hand back stale content
		if st.IsFresh(r.cacheThreshold) {
			result[source.IDOf(st.Source())] = st.Content()
		}
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// refreshStates mirrors `_async_refresh_data_state` +
// `_async_refresh_states`: states refreshed here vs. awaited from a
// concurrent caller, fanned out per caller group.
func (r *AsusRouter) refreshStates(ctx context.Context, states []*source.ARDataState, force bool, kw registry.Kwargs) error {
	var mine, pending []*source.ARDataState
	for _, st := range states {
		if st.Refreshing() {
			pending = append(pending, st)
			continue
		}
		if force || !st.IsFresh(r.cacheThreshold) {
			if st.BeginRefresh() {
				mine = append(mine, st)
			} else {
				pending = append(pending, st)
			}
		}
	}

	if len(mine) > 0 {
		c := r.newContext(kw, force)
		// Group multicallers by their fetch function (like Python's call
		// matrix: sources sharing one fetch go out as one batched
		// request), single callers run alone
		multi := map[uintptr][]*source.ARDataState{}
		entries := map[uintptr]*registry.SourceEntry{}
		var order []uintptr
		var singles []*source.ARDataState
		for _, st := range mine {
			e := registry.Source(st.Source())
			switch {
			case e == nil:
				st.EndRefresh()
			case e.Multi():
				key := reflect.ValueOf(e.FetchMulti).Pointer()
				if _, ok := multi[key]; !ok {
					order = append(order, key)
					entries[key] = e
				}
				multi[key] = append(multi[key], st)
			default:
				singles = append(singles, st)
			}
		}
		var wg sync.WaitGroup
		errs := make(chan error, len(order)+len(singles))
		for _, key := range order {
			group := multi[key]
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- r.refreshMulti(ctx, c, entries[key], group)
			}()
		}
		for _, st := range singles {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- r.refreshSingle(ctx, c, registry.Source(st.Source()), st)
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return err
			}
		}
	}

	// Wait for refreshes started by concurrent callers
	for _, st := range pending {
		select {
		case <-st.WaitRefresh():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *AsusRouter) refreshSingle(ctx context.Context, c *registry.Context, e *registry.SourceEntry, st *source.ARDataState) error {
	defer st.EndRefresh()
	if e.Fetch == nil {
		return nil
	}
	raw, err := e.Fetch(ctx, c, st.Source())
	if err != nil {
		return err
	}
	value := raw
	if e.Translate != nil {
		value = e.Translate(raw, &registry.TranslateContext{
			Identity: r.Description(),
			Previous: st.Content(),
			Kwargs:   c.Kwargs,
		})
	}
	r.commit(st, value)
	return nil
}

func (r *AsusRouter) refreshMulti(ctx context.Context, c *registry.Context, e *registry.SourceEntry, states []*source.ARDataState) error {
	defer func() {
		for _, st := range states {
			st.EndRefresh()
		}
	}()
	srcs := make([]source.DataSource, len(states))
	for i, st := range states {
		srcs[i] = st.Source()
	}
	data, err := e.FetchMulti(ctx, c, srcs)
	if err != nil {
		return err
	}
	tc := &registry.TranslateContext{Identity: r.Description(), Kwargs: c.Kwargs}
	switch {
	case e.TranslateMulti != nil:
		data = e.TranslateMulti(data, tc)
	case e.Translate != nil:
		for id, v := range data {
			data[id] = e.Translate(v, tc)
		}
	}
	for _, st := range states {
		if v, ok := data[source.IDOf(st.Source())]; ok {
			r.commit(st, v)
		}
	}
	return nil
}

// commit mirrors `_commit_data_state`: update a state and keep the
// identity's live topology and clock in sync.
func (r *AsusRouter) commit(st *source.ARDataState, value any) {
	st.Update(value)
	syncIdentity(r.Description(), st.Source(), value)
}

// Expire mirrors `_async_expire_data`.
func (r *AsusRouter) Expire(_ context.Context, src source.DataSource) {
	r.mu.Lock()
	st := r.dataStates[source.IDOf(src)]
	r.mu.Unlock()
	if st != nil {
		st.Expire()
	}
}

// DefaultSources mirrors `_default_sources`.
func (r *AsusRouter) DefaultSources() []source.DataSource { return registry.DefaultSources() }

// ---------------------------
// Action pipeline -->
// ---------------------------

// RunAction mirrors `async_run_action`: run an action or push data to
// the device. It returns the (translated) action result.
func (r *AsusRouter) RunAction(ctx context.Context, a action.ARAction, kw registry.Kwargs) (any, error) {
	e := registry.Action(a)
	if e == nil || e.Run == nil {
		return nil, fmt.Errorf("no action registered for %s", a.ActionName())
	}
	if err := r.ensureConnected(ctx); err != nil {
		return nil, err
	}
	c := r.newContext(kw, true)
	c.Expire = r.Expire
	c.CredentialsGet = r.currentCredentials
	c.CredentialsSet = r.SetCredentials

	raw, err := e.Run(ctx, c, a)
	if err != nil {
		return nil, err
	}

	// Remember the last commanded LED state
	if state, ok := ledActionState(a); ok && truthyResult(raw) {
		r.mu.Lock()
		r.ledState = &state
		r.mu.Unlock()
	}

	// An action may have triggered a reboot
	if r.Description().Rebooted() {
		r.transport.ResetAuth()
		r.startRecovery()
	}

	if e.Translate == nil {
		return raw, nil
	}
	return e.Translate(raw, &registry.TranslateContext{Identity: r.Description(), Kwargs: kw}), nil
}

func truthyResult(v any) bool {
	if b, ok := v.(interface{ Bool() bool }); ok {
		return b.Bool()
	}
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case *bool:
		return t != nil && *t
	}
	return true
}

// handleReboot mirrors `_async_handle_reboot`.
func (r *AsusRouter) handleReboot(ctx context.Context) {
	r.Description().ClearRebooted()
	r.mu.Lock()
	led := r.ledState
	r.mu.Unlock()
	// Recover LED off for Merlin
	recoverLedState(ctx, r, led)
}

func (r *AsusRouter) startRecovery() {
	r.mu.Lock()
	if r.recovery != nil {
		r.mu.Unlock()
		return
	}
	done := make(chan struct{})
	r.recovery = done
	r.mu.Unlock()
	go r.recoverAfterReboot(done)
}

// recoverAfterReboot mirrors `_async_recover_after_reboot`: hold
// requests while the device reboots, until it is back online.
func (r *AsusRouter) recoverAfterReboot(done chan struct{}) {
	slog.Warn("Device is rebooting; data fetching is on hold until it is back")
	if r.conn != nil {
		r.conn.SetErrorSuppression(true)
		defer r.conn.SetErrorSuppression(false)
	}
	defer func() {
		r.mu.Lock()
		r.recovery = nil
		r.mu.Unlock()
		close(done)
	}()
	ctx := context.Background()
	time.Sleep(rebootRecoveryInitialDelay)
	deadline := time.Now().Add(rebootRecoveryTimeout)
	for time.Now().Before(deadline) {
		if ok, _ := r.transport.Connect(ctx); ok {
			slog.Debug("Device is back online after the reboot")
			r.Description().ClearRebooted()
			return
		}
		time.Sleep(rebootRecoveryInterval)
	}
	slog.Error("Device did not come back after the reboot", "timeout", rebootRecoveryTimeout)
}
