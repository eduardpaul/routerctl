package client

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// TestReplayParity replays every recorded session through fetch +
// translate and compares with the Python golden (py.json).
//
// The VPN client fetch and translate do not read the identity, so none is
// built here.
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
			want, ok := golden["ARVpnClientSource"]
			if !ok {
				t.Skip("no golden for ARVpnClientSource")
			}

			c := &registry.Context{Read: session.Read(), FetchRaw: session.FetchRaw()}
			raw, err := fetchState(context.Background(), c, ARVpnClientSourceUniversal)
			if err != nil {
				t.Fatal(err)
			}
			got := translateState(raw, &registry.TranslateContext{})
			for _, m := range session.Misses {
				t.Errorf("unmatched query: %s", m)
			}
			for _, d := range replay.Diff(want, got) {
				t.Error(d)
			}
		})
	}
}
