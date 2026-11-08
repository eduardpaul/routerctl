package source_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

// TestReplayARSupportSource replays the recorded sessions and compares the
// result with the Python golden (py.json["ARSupportSource"]).
func TestReplayARSupportSource(t *testing.T) {
	devicetest.ReplaySource(t, support.ARSupportSourceUniversal)
}
