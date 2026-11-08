package clients

import (
	"context"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// ARClientsSource mirrors `ARClientsSource`: clients data source for the
// connected router. It is router-global (empty `_key`) and remembers the
// clients it has seen.
type ARClientsSource struct {
	source.Base

	mu   sync.Mutex
	prev map[identifiers.MacAddress]ARClient
}

// SourceName returns the Python class name.
func (*ARClientsSource) SourceName() string { return "ARClientsSource" }

// MergeHistory mirrors `merge_history`: keep previously seen clients that
// vanished, marked offline.
func (s *ARClientsSource) MergeHistory(current map[identifiers.MacAddress]ARClient) map[identifiers.MacAddress]ARClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[identifiers.MacAddress]ARClient, len(current))
	for mac, client := range current {
		result[mac] = client
	}
	for mac, client := range s.prev {
		if _, ok := result[mac]; !ok {
			client.Online = false
			client.Connection = nil
			result[mac] = client
		}
	}
	s.prev = result
	return result
}

// ARClientsSourceUniversal is the universal instance - preferred
var ARClientsSourceUniversal = &ARClientsSource{}

// fetchState mirrors `fetch_state`: fetch and build the client list,
// retaining vanished clients.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	raw, err := c.Read(ctx, endpoint.AREndpointFetchData,
		endpoint.HookRequest(endpoint.ARHookClientlist, endpoint.ARHookClientlistDatabase))
	if err != nil {
		return nil, err
	}
	s, ok := src.(*ARClientsSource)
	if !ok || s == nil {
		s = ARClientsSourceUniversal
	}
	return s.MergeHistory(BuildClients(raw, c.Identity)), nil
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:    "ARClientsSource",
		Default: func() source.DataSource { return &ARClientsSource{} },
		Fetch:   fetchState,
	})
}
