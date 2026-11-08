package portforwarding

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// replayContext answers from a recorded session like the router: reads
// go to the session, and FetchDataMulti batches multicaller sources that
// share a fetch function (Python groups by callable) and translates them.
func replayContext(s *replay.Session, identity *device.ARDeviceIdentity) *registry.Context {
	c := &registry.Context{Read: s.Read(), FetchRaw: s.FetchRaw(), Identity: identity}
	c.FetchDataMulti = func(ctx context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		type group struct {
			entry *registry.SourceEntry
			srcs  []source.DataSource
		}
		var order []uintptr
		groups := map[uintptr]*group{}
		for _, src := range srcs {
			e := registry.Source(src)
			if e == nil || e.FetchMulti == nil {
				continue
			}
			key := reflect.ValueOf(e.FetchMulti).Pointer()
			if groups[key] == nil {
				groups[key] = &group{entry: e}
				order = append(order, key)
			}
			groups[key].srcs = append(groups[key].srcs, src)
		}
		out := map[source.ID]any{}
		for _, key := range order {
			g := groups[key]
			raw, err := g.entry.FetchMulti(ctx, c, g.srcs)
			if err != nil {
				return nil, err
			}
			translated := raw
			if g.entry.TranslateMulti != nil {
				translated = g.entry.TranslateMulti(raw, &registry.TranslateContext{Identity: identity})
			}
			for id, v := range translated {
				out[id] = v
			}
		}
		return out, nil
	}
	return c
}

// TestReplayParity replays every recorded session through fetch +
// translate and compares with the Python golden.
func TestReplayParity(t *testing.T) {
	const name = "ARPortForwardingSource"
	devices := replay.LocalDevices()
	if len(devices) == 0 {
		t.Skip("no recorded sessions")
	}
	for _, dir := range devices {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			golden, err := replay.LoadGolden(filepath.Join(dir, "py.json"))
			if err != nil {
				t.Fatal(err)
			}
			want, ok := golden[name]
			if !ok {
				t.Skipf("no %s in golden", name)
			}
			s, err := replay.Load(filepath.Join(dir, "session.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			c := replayContext(s, nil)
			raw, err := fetchState(context.Background(), c, ARPortForwardingSourceUniversal)
			if err != nil {
				t.Fatal(err)
			}
			got := translateState(raw, &registry.TranslateContext{})
			if diffs := replay.Diff(want, got); len(diffs) > 0 {
				t.Errorf("%s differs from Python (misses: %v):\n%v", name, s.Misses, diffs)
			}
		})
	}
}
