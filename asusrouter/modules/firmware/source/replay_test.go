package source_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/firmware/source"
)

// TestReplayARFirmwareSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARFirmwareSource"]).
func TestReplayARFirmwareSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARFirmwareSourceUniversal)
}
