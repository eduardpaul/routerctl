package devicetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
)

// TestReplayIdentity replays `async_connect` on every recorded session
// and compares the identity with the Python golden (py.json["_identity"]).
func TestReplayIdentity(t *testing.T) {
	devices, err := Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) == 0 {
		t.Skip("no recorded sessions in parity/local or parity/fixtures")
	}
	for _, d := range devices {
		t.Run(filepath.Base(d.Dir), func(t *testing.T) {
			want, ok := d.Golden["_identity"]
			if !ok {
				t.Skip("no _identity golden")
			}
			r, err := d.Connected(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, diff := range replay.Diff(want, r.Description) {
				t.Error(diff)
			}
		})
	}
}
