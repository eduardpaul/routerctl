package wan_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"

	"github.com/eduardpaul/routerctl/asusrouter/modules/wan"
)

func TestReplayParity(t *testing.T) {
	runReplay(t, "ARWanSource", func() source.DataSource { return &wan.ARWanSource{} })
}
