package dsl_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"

	// Register ARDeviceSource / ARAiMeshSource and the nvram multicaller;
	// device/source imports this package through support/source, hence
	// the external test package
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
)

// replayPipeline is a minimal `AsusRouter` data pipeline over a recorded
// session: every fetch runs the registered entry (single or multicaller)
// and its translator, with the identity built like `async_connect` does.
type replayPipeline struct {
	session  *replay.Session
	identity *device.ARDeviceIdentity
}

func (p *replayPipeline) context() *registry.Context {
	return &registry.Context{
		Read:           p.session.Read(),
		FetchRaw:       p.session.FetchRaw(),
		FetchData:      p.fetchData,
		FetchDataMulti: p.fetchDataMulti,
		Identity:       p.identity,
		Kwargs:         registry.Kwargs{"force": true},
	}
}

func (p *replayPipeline) fetchData(ctx context.Context, src source.DataSource, kw registry.Kwargs) (any, error) {
	values, err := p.fetchDataMulti(ctx, []source.DataSource{src}, kw)
	if err != nil {
		return nil, err
	}
	return values[source.IDOf(src)], nil
}

// fetchDataMulti groups multicallers by their fetch callable, like
// Python `_get_call_matrix` (ARNvramType and ARNvramIndexSource share one
// `fetch_state`, so they go out as one batched request).
func (p *replayPipeline) fetchDataMulti(ctx context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
	result := map[source.ID]any{}
	groups := map[uintptr][]source.DataSource{}
	entries := map[uintptr]*registry.SourceEntry{}
	var order []uintptr
	for _, src := range srcs {
		entry := registry.Source(src)
		if entry == nil {
			continue
		}
		if !entry.Multi() {
			raw, err := entry.Fetch(ctx, p.context(), src)
			if err != nil {
				return nil, err
			}
			value := raw
			if entry.Translate != nil {
				value = entry.Translate(raw, &registry.TranslateContext{Identity: p.identity})
			}
			result[source.IDOf(src)] = value
			continue
		}
		caller := reflect.ValueOf(entry.FetchMulti).Pointer()
		if _, ok := groups[caller]; !ok {
			order = append(order, caller)
			entries[caller] = entry
		}
		groups[caller] = append(groups[caller], src)
	}
	for _, caller := range order {
		entry := entries[caller]
		data, err := entry.FetchMulti(ctx, p.context(), groups[caller])
		if err != nil {
			return nil, err
		}
		tc := &registry.TranslateContext{Identity: p.identity}
		switch {
		case entry.TranslateMulti != nil:
			data = entry.TranslateMulti(data, tc)
		case entry.Translate != nil:
			for id, v := range data {
				data[id] = entry.Translate(v, tc)
			}
		}
		for id, v := range data {
			result[id] = v
		}
	}
	return result, nil
}

// fetchDefault fetches a registered source's default instance by name.
func (p *replayPipeline) fetchDefault(t *testing.T, name string) any {
	t.Helper()
	entry := registry.SourceByName(name)
	if entry == nil || entry.Default == nil {
		t.Fatalf("source %s is not registered", name)
	}
	value, err := p.fetchData(context.Background(), entry.Default(), nil)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return value
}

// connect mirrors `async_connect`: the identity, then the AiMesh topology
// seeded into it (the clock seeding is not needed here).
func (p *replayPipeline) connect(t *testing.T) {
	t.Helper()
	p.identity = device.NewARDeviceIdentity()
	identity, ok := p.fetchDefault(t, "ARDeviceSource").(*device.ARDeviceIdentity)
	if !ok || identity == nil {
		t.Fatal("ARDeviceSource did not build an identity")
	}
	p.identity = identity
	if topology, ok := p.fetchDefault(t, "ARAiMeshSource").(*aimesh.ARAiMeshTopology); ok && topology != nil {
		p.identity.UpdateAimesh(topology)
	}
}

// runReplay replays every recorded session through fetch + translate of
// a fresh source from newSource (sources may keep state, like the clients
// history) and compares with the Python golden for name.
func runReplay(t *testing.T, name string, newSource func() source.DataSource) {
	devices := replay.LocalDevices()
	if len(devices) == 0 {
		t.Skip("no recorded sessions")
	}
	for _, dir := range devices {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			session, err := replay.Load(filepath.Join(dir, "session.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			golden, err := replay.LoadGolden(filepath.Join(dir, "py.json"))
			if err != nil {
				t.Fatal(err)
			}
			want, ok := golden[name]
			if !ok {
				t.Skipf("no %s golden", name)
			}

			p := &replayPipeline{session: session}
			p.connect(t)

			src := newSource()
			entry := registry.Source(src)
			raw, err := entry.Fetch(context.Background(), p.context(), src)
			if err != nil {
				t.Fatal(err)
			}
			got := raw
			if entry.Translate != nil {
				got = entry.Translate(raw, &registry.TranslateContext{Identity: p.identity})
			}
			for _, d := range replay.Diff(want, got) {
				t.Error(d)
			}
			for _, miss := range session.Misses {
				t.Logf("unmatched query: %s", miss)
			}
		})
	}
}
