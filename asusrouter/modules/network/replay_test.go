package network_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/network"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"

	// Registers ARDeviceSource (and its nvram/support dependencies); it
	// imports this package through support/source, hence the external
	// test package
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
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

func (p *replayPipeline) fetchDataMulti(ctx context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
	result := map[source.ID]any{}
	groups := map[string][]source.DataSource{}
	var order []string
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
		if _, ok := groups[entry.Name]; !ok {
			order = append(order, entry.Name)
		}
		groups[entry.Name] = append(groups[entry.Name], src)
	}
	for _, name := range order {
		entry := registry.SourceByName(name)
		data, err := entry.FetchMulti(ctx, p.context(), groups[name])
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

// connect mirrors `async_connect` as far as the network source needs it:
// the identity (the network source reads only its support and wifi maps,
// so the AiMesh topology and clock seeding are skipped).
func (p *replayPipeline) connect(t *testing.T) {
	t.Helper()
	p.identity = device.NewARDeviceIdentity()
	identity, ok := p.fetchDefault(t, "ARDeviceSource").(*device.ARDeviceIdentity)
	if !ok || identity == nil {
		t.Fatal("ARDeviceSource did not build an identity")
	}
	p.identity = identity
}

func TestReplayParity(t *testing.T) {
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
			want, ok := golden["ARNetworkSource"]
			if !ok {
				t.Skip("no ARNetworkSource golden")
			}

			p := &replayPipeline{session: session}
			p.connect(t)

			entry := registry.Source(network.ARNetworkSourceUniversal)
			raw, err := entry.Fetch(context.Background(), p.context(), network.ARNetworkSourceUniversal)
			if err != nil {
				t.Fatal(err)
			}
			got := entry.Translate(raw, &registry.TranslateContext{Identity: p.identity})
			for _, d := range replay.Diff(want, got) {
				t.Error(d)
			}
		})
	}
}
