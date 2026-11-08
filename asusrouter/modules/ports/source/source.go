// Package source mirrors asusrouter/modules/ports/source.py (split out of
// package ports to break the identity import cycle).
package source

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/ports"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Request fetching the whole AiMesh network in one call
const portStatusRequest = "node_mac=all"

// ARPortsSource is the AsusRouter ports data source.
type ARPortsSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARPortsSource) SourceName() string { return "ARPortsSource" }

// ARPortsSourceUniversal is the universal instance - preferred.
var ARPortsSourceUniversal = &ARPortsSource{}

// fetchState fetches raw port data, preferring the modern endpoint.
func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	data, err := c.Read(ctx, endpoint.AREndpointFetchPortStatus, portStatusRequest)
	if err != nil {
		return nil, err
	}
	if m, ok := data.(map[string]any); !ok || len(m) == 0 {
		data, err = c.Read(ctx, endpoint.AREndpointFetchPortsEthernet, "")
		if err != nil {
			return nil, err
		}
	}

	if m, ok := data.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// translateState translates raw port data to the unified per-MAC format.
func translateState(raw any, tc *registry.TranslateContext) any {
	data, ok := raw.(map[string]any)
	if !ok || len(data) == 0 {
		return map[identifiers.MacAddress]ports.ARPortsData{}
	}

	var identity ports.Identity
	if tc != nil && tc.Identity != nil {
		identity = tc.Identity
	}

	if _, ok := data["port_info"]; ok {
		return ports.TranslatePortStatus(data, identity)
	}
	if _, ok := data["portSpeed"]; ok {
		return ports.TranslateEthernetPorts(data, identity)
	}

	return map[identifiers.MacAddress]ports.ARPortsData{}
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARPortsSource",
		Default:   func() source.DataSource { return &ARPortsSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
