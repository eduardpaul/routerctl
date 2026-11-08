package source_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/ports/source"
)

// TestReplayARPortsSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARPortsSource"]).
func TestReplayARPortsSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARPortsSourceUniversal)
}
