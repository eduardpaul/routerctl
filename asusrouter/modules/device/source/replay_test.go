package source_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
)

// TestReplayARDeviceSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARDeviceSource"]).
func TestReplayARDeviceSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARDeviceSourceUniversal)
}
