package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/connection"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateConnection(t *testing.T) {
	check(t, "translateConnection", translateConnection, []struct {
		data     d
		expected []connection.ARConnection
	}{
		{d{k(support.ARSupportValueConnectionHttps): 1}, []connection.ARConnection{connection.ARConnectionHttps}},
		{d{k(support.ARSupportValueConnectionSsh): 1}, []connection.ARConnection{connection.ARConnectionSsh}},
		{d{}, []connection.ARConnection{}},
	})
}
