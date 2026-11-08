package source_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/wifi/source"
)

// TestReplayARWiFiSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARWiFiSource"]).
func TestReplayARWiFiSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARWiFiSourceUniversal)
}
