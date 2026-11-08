package schedule_test

// Session-replay parity: run the recorded sessions through the Go router
// (connect, then the sources in name order, like parity/py_harness.py)
// and compare with the Python goldens.

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	asusrouter "github.com/eduardpaul/routerctl/asusrouter"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control/block_all"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control/schedule"
	_ "github.com/eduardpaul/routerctl/asusrouter/modules/support/source"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func TestReplayParity(t *testing.T) {
	devices := replay.LocalDevices()
	if len(devices) == 0 {
		t.Skip("no recorded sessions")
	}
	names := []string{"ARBlockAllSource", "ARParentalControlSource"}
	sort.Strings(names)
	for _, dir := range devices {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			ctx := context.Background()
			golden, err := replay.LoadGolden(filepath.Join(dir, "py.json"))
			if err != nil {
				t.Fatal(err)
			}
			transport, err := asusrouter.NewReplayTransport(filepath.Join(dir, "session.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			router, err := asusrouter.New("replay.invalid", "admin", "replay", asusrouter.Options{Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := router.Connect(ctx); err != nil {
				t.Fatal(err)
			}
			for _, name := range names {
				want, ok := golden[name]
				if !ok {
					continue
				}
				entry := registry.SourceByName(name)
				if entry == nil || entry.Default == nil {
					t.Errorf("%s: not registered", name)
					continue
				}
				got, err := router.FetchData(ctx, entry.Default(), true, nil)
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				for _, d := range replay.Diff(want, got) {
					t.Errorf("%s: %s", name, d)
				}
			}
		})
	}
}
