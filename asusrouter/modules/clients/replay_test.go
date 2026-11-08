package clients_test

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"

	"github.com/eduardpaul/routerctl/asusrouter/modules/clients"
)

func TestReplayParity(t *testing.T) {
	runReplay(t, "ARClientsSource", func() source.DataSource { return &clients.ARClientsSource{} })
}
