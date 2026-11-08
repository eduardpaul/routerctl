// Package client mirrors asusrouter/modules/vpn/client/: the VPN client
// data source and action (the router acting as a VPN client). The
// backends (classic.py, fusion.py) are the subpackages `classic` and
// `fusion`.
package client

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client/classic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client/fusion"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ARVpnClientSource mirrors `ARVpnClientSource`: AsusRouter VPN client
// data source (router acting as a VPN client).
type ARVpnClientSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARVpnClientSource) SourceName() string { return "ARVpnClientSource" }

// ARVpnClientSourceUniversal is the universal instance - preferred
var ARVpnClientSourceUniversal = &ARVpnClientSource{}

// isFusion mirrors `_is_fusion`: whether the device exposes the VPN Fusion
// `vpnc_clientlist`.
func isFusion(data map[string]any) bool {
	return converters.RawToStr(data[string(nvram.ARNvramTypeVpncClientlist)]) != nil
}

// fetchState mirrors `fetch_state`: fetch client status and config,
// falling back to the classic path.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	// dict.fromkeys: drop repeated items, keep the first occurrence
	seen := map[source.ID]bool{}
	hooks := []endpoint.ARHookItem{fusion.StatusHook, fusion.NondefWanHook}
	for _, item := range append(fusion.NvramItems(), classic.NvramItems()...) {
		id := source.IDOf(item)
		if seen[id] {
			continue
		}
		seen[id] = true
		hooks = append(hooks, item)
	}
	request := endpoint.HookRequest(hooks...)
	data, err := c.Read(ctx, endpoint.AREndpointFetchData, request)
	if err != nil {
		return nil, err
	}

	// Non-Fusion devices report OpenVPN clients through vpn.cgi instead
	if m, ok := data.(map[string]any); ok && !isFusion(m) {
		status, err := c.Read(ctx, endpoint.AREndpointFetchVpnStatus, "")
		if err != nil {
			return nil, err
		}
		m[classic.StatusKey] = status
	}

	return data, nil
}

// translateState mirrors `translate_state`: raw data into VPN client
// profiles grouped by protocol.
func translateState(raw any, _ *registry.TranslateContext) any {
	data, ok := raw.(map[string]any)
	if !ok {
		return map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any{}
	}

	if isFusion(data) {
		return fusion.Translate(data)
	}
	return classic.Translate(data)
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARVpnClientSource",
		Default:   func() source.DataSource { return &ARVpnClientSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
