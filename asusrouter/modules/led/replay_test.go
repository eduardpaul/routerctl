package led_test

// Session-replay parity for ARLedSource: replays every recorded session
// through fetch + translate and compares with the Python golden output
// (parity/local/<device>/py.json).

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"

	// The sources `async_connect` reads, and the source under test
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/led"
)

// The parity harness replays with `username="admin"`
const replayUsername = "admin"

// replayPipeline is a minimal `AsusRouter` data pipeline over a recorded
// session: every fetch runs the registered entry (single or multicaller)
// and its translator (`force=True`, so nothing is served from a cache),
// and commits keep the identity in sync like `_commit_data_state`.
type replayPipeline struct {
	session  *replay.Session
	identity *device.ARDeviceIdentity
	previous map[source.ID]any
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
				value = entry.Translate(raw, &registry.TranslateContext{
					Identity: p.identity,
					Previous: p.previous[source.IDOf(src)],
				})
			}
			p.commit(src, value)
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

// commit mirrors `_commit_data_state`: keep the previous content and the
// identity's live AiMesh topology and clock in sync.
func (p *replayPipeline) commit(src source.DataSource, value any) {
	p.previous[source.IDOf(src)] = value
	if topology, ok := value.(*aimesh.ARAiMeshTopology); ok && p.identity != nil {
		p.identity.UpdateAimesh(topology)
	}
	state, ok := value.(map[clock.ARClockField]any)
	if !ok || src.SourceName() != "ARClockSource" || p.identity == nil {
		return
	}
	if boottime, ok := state[clock.ARClockFieldBoottime].(clock.ARBoottime); ok {
		p.identity.UpdateBoottime(&boottime.Time)
	}
	if uptime, ok := state[clock.ARClockFieldUptime].(int); ok {
		p.identity.UpdateUptime(&uptime)
	}
	if deviceTime, ok := state[clock.ARClockFieldDeviceTime].(time.Time); ok {
		p.identity.UpdateDeviceTime(&deviceTime)
	}
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

// connect mirrors `async_connect`: the identity, the injected username,
// the seeded AiMesh topology and the device clock.
func (p *replayPipeline) connect(t *testing.T) {
	t.Helper()
	p.identity = device.NewARDeviceIdentity()
	identity, ok := p.fetchDefault(t, "ARDeviceSource").(*device.ARDeviceIdentity)
	if !ok || identity == nil {
		t.Fatal("ARDeviceSource did not build an identity")
	}
	p.identity = identity
	username := identifiers.NewUsername(replayUsername)
	p.identity.UpdateUsername(&username)
	p.fetchDefault(t, "ARAiMeshSource")
	p.fetchDefault(t, "ARClockSource")
}

func TestReplayParity(t *testing.T) {
	devices := replay.LocalDevices()
	if len(devices) == 0 {
		t.Skip("no recorded sessions under parity/local or parity/fixtures")
	}
	for _, dir := range devices {
		golden, err := replay.LoadGolden(filepath.Join(dir, "py.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"ARLedSource"} {
			t.Run(filepath.Base(dir)+"/"+name, func(t *testing.T) {
				want, ok := golden[name]
				if !ok {
					t.Skipf("%s has no %s golden", dir, name)
				}
				session, err := replay.Load(filepath.Join(dir, "session.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				p := &replayPipeline{session: session, previous: map[source.ID]any{}}
				p.connect(t)
				got := p.fetchDefault(t, name)
				if diffs := replay.Diff(want, got); len(diffs) > 0 {
					for _, d := range diffs {
						t.Error(d)
					}
				}
			})
		}
	}
}
