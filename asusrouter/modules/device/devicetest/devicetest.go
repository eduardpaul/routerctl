// Package devicetest replays recorded sessions through the module
// fetchers and translators the way `AsusRouter` wires them, for the
// session-replay parity tests of the device identity closure.
//
// It reproduces the parts of asusrouter.py the tests need:
// `async_connect` (identity, username, AiMesh seed, clock sync),
// `_commit_data_state` (AiMesh topology and clock sync into the identity)
// and the multicaller batching of `_async_refresh_states`. It is a test
// helper, not the router port.
package devicetest

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	aimeshsource "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	devicesource "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"

	// Sources the device request depends on
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/support/source"
)

// ReplayUsername is the login the Python harness replays with.
const ReplayUsername = "admin"

// Router is a minimal replaying router: one session, one identity.
type Router struct {
	Session *replay.Session
	// Description is the identity (`AsusRouter.description`)
	Description *device.ARDeviceIdentity
}

// New starts a router on a session with an empty identity.
func New(sess *replay.Session) *Router {
	return &Router{Session: sess, Description: device.NewARDeviceIdentity()}
}

// Context builds the `callback` + `**kwargs` for a fetch.
func (r *Router) Context() *registry.Context {
	return &registry.Context{
		Read:     r.Session.Read(),
		FetchRaw: r.Session.FetchRaw(),
		FetchData: func(ctx context.Context, src source.DataSource, kw registry.Kwargs) (any, error) {
			out, err := r.FetchMulti(ctx, []source.DataSource{src})
			if err != nil {
				return nil, err
			}
			return out[source.IDOf(src)], nil
		},
		FetchDataMulti: func(ctx context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
			return r.FetchMulti(ctx, srcs)
		},
		Identity: r.Description,
		Kwargs:   registry.Kwargs{},
	}
}

func (r *Router) translateContext() *registry.TranslateContext {
	return &registry.TranslateContext{Identity: r.Description, Kwargs: registry.Kwargs{}}
}

// Fetch fetches and translates one source (`async_fetch_data(source,
// force=True)`), committing it like `_commit_data_state`.
func (r *Router) Fetch(ctx context.Context, src source.DataSource) (any, error) {
	out, err := r.FetchMulti(ctx, []source.DataSource{src})
	if err != nil {
		return nil, err
	}
	return out[source.IDOf(src)], nil
}

// FetchMulti fetches a collection: single sources one by one, multicaller
// sources batched per fetcher (sources sharing a fetch function share
// one request, like Python's call matrix).
func (r *Router) FetchMulti(ctx context.Context, srcs []source.DataSource) (map[source.ID]any, error) {
	result := map[source.ID]any{}

	type batch struct {
		entry *registry.SourceEntry
		srcs  []source.DataSource
	}
	var batches []*batch
	byFetcher := map[uintptr]*batch{}

	for _, src := range srcs {
		entry := registry.Source(src)
		if entry == nil {
			return nil, fmt.Errorf("no registry entry for %s", source.Repr(src))
		}
		if entry.Multi() {
			key := reflect.ValueOf(entry.FetchMulti).Pointer()
			b, ok := byFetcher[key]
			if !ok {
				b = &batch{entry: entry}
				byFetcher[key] = b
				batches = append(batches, b)
			}
			b.srcs = append(b.srcs, src)
			continue
		}
		batches = append(batches, &batch{entry: entry, srcs: []source.DataSource{src}})
	}

	for _, b := range batches {
		if b.entry.Multi() {
			data, err := b.entry.FetchMulti(ctx, r.Context(), b.srcs)
			if err != nil {
				return nil, err
			}
			translated := data
			if b.entry.TranslateMulti != nil {
				translated = b.entry.TranslateMulti(data, r.translateContext())
			}
			for _, src := range b.srcs {
				id := source.IDOf(src)
				if v, ok := translated[id]; ok {
					result[id] = v
					r.commit(src, v)
				}
			}
			continue
		}

		src := b.srcs[0]
		if b.entry.Fetch == nil {
			continue
		}
		raw, err := b.entry.Fetch(ctx, r.Context(), src)
		if err != nil {
			return nil, err
		}
		value := raw
		if b.entry.Translate != nil {
			value = b.entry.Translate(raw, r.translateContext())
		}
		result[source.IDOf(src)] = value
		r.commit(src, value)
	}

	return result, nil
}

// commit mirrors the identity side of `_commit_data_state`.
func (r *Router) commit(src source.DataSource, value any) {
	switch v := value.(type) {
	case *device.ARDeviceIdentity:
		// The device state content is the description
		if _, ok := src.(*devicesource.ARDeviceSource); ok {
			r.Description = v
		}
	case *aimesh.ARAiMeshTopology:
		// Keep the identity's live AiMesh topology in sync
		r.Description.UpdateAimesh(v)
	case map[clock.ARClockField]any:
		// Keep the identity's live clock in sync
		if _, ok := src.(*clock.ARClockSource); ok {
			SyncClock(r.Description, v)
		}
	}
}

// SyncClock mirrors `AsusRouter._sync_clock`: sync the identity with the
// values a clock read reported.
func SyncClock(identity *device.ARDeviceIdentity, value map[clock.ARClockField]any) {
	if boottime, ok := value[clock.ARClockFieldBoottime].(clock.ARBoottime); ok {
		t := boottime.Time
		identity.UpdateBoottime(&t)
	}

	// `bool` is an `int` in Python; only a real count says anything here
	if uptime, ok := value[clock.ARClockFieldUptime].(int); ok {
		identity.UpdateUptime(&uptime)
	}

	if deviceTime, ok := value[clock.ARClockFieldDeviceTime].(time.Time); ok {
		identity.UpdateDeviceTime(&deviceTime)
	}
}

// Connect mirrors `async_connect`: fetch the device description, inject
// the username, seed the AiMesh topology and sync the clock.
func (r *Router) Connect(ctx context.Context) error {
	if _, err := r.Fetch(ctx, devicesource.ARDeviceSourceUniversal); err != nil {
		return err
	}
	username := identifiers.UsernameFromValue(ReplayUsername)
	r.Description.UpdateUsername(&username)
	if _, err := r.Fetch(ctx, aimeshsource.ARAiMeshSourceUniversal); err != nil {
		return err
	}
	_, err := r.Fetch(ctx, clock.ARClockSourceUniversal)
	return err
}

// Device is one recorded session with its Python golden output.
type Device struct {
	Dir     string
	Session string
	Golden  map[string]any
}

// Devices lists the recorded sessions (parity/local and parity/fixtures).
func Devices() ([]Device, error) {
	var out []Device
	for _, dir := range replay.LocalDevices() {
		golden, err := replay.LoadGolden(dir + "/py.json")
		if err != nil {
			return nil, err
		}
		out = append(out, Device{Dir: dir, Session: dir + "/session.jsonl", Golden: golden})
	}
	return out, nil
}

// Connected loads a device session and connects a router on it.
func (d Device) Connected(ctx context.Context) (*Router, error) {
	sess, err := replay.Load(d.Session)
	if err != nil {
		return nil, err
	}
	r := New(sess)
	if err := r.Connect(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// ReplaySource runs a source through every recorded session (after a
// replayed connect) and compares it with `py.json[<SourceName>]`. It
// skips when no session is recorded.
func ReplaySource(t *testing.T, src source.DataSource) {
	t.Helper()
	devices, err := Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("no recorded sessions in parity/local or parity/fixtures")
	}
	for _, d := range devices {
		t.Run(filepath.Base(d.Dir), func(t *testing.T) {
			want, ok := d.Golden[src.SourceName()]
			if !ok {
				t.Skipf("no golden for %s", src.SourceName())
			}
			ctx := context.Background()
			r, err := d.Connected(ctx)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Fetch(ctx, src)
			if err != nil {
				t.Fatal(err)
			}
			for _, diff := range replay.Diff(want, got) {
				t.Error(diff)
			}
		})
	}
}
