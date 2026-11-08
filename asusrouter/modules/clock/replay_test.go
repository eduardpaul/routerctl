package clock_test

import (
	"testing"

	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
)

// TestReplayARClockSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARClockSource"]).
func TestReplayARClockSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARClockSourceUniversal)
}
