package count_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/clients/count"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
)

func TestReplayParity(t *testing.T) {
	runReplay(t, "ARClientsCountSource", func() source.DataSource { return &count.ARClientsCountSource{} })
}
