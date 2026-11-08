package source_test

import (
	"testing"

	pkg "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device/devicetest"
)

// TestReplayARAiMeshSource replays the recorded sessions and compares the result with
// the Python golden (py.json["ARAiMeshSource"]).
func TestReplayARAiMeshSource(t *testing.T) {
	devicetest.ReplaySource(t, pkg.ARAiMeshSourceUniversal)
}
