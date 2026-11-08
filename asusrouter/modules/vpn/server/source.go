// Package server mirrors asusrouter/modules/vpn/server/: the VPN server
// data source and action. The protocol backends (openvpn.py,
// wireguard.py) are the subpackages `openvpn` and `wireguard`.
package server

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/server/openvpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/server/wireguard"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// backend is one protocol backend module (each exposes `nvram_items` and
// `translate`).
type backend struct {
	protocol   vpn.ARVpnProtocol
	nvramItems func() []nvram.ARNvramItem
	translate  func(map[string]any) map[int]map[vpn.ARVpnServerField]any
}

// Protocol -> its backend module (each exposes `nvram_items` and `translate`)
var backends = []backend{
	{vpn.ARVpnProtocolWireguard, wireguard.NvramItems, wireguard.Translate},
	{vpn.ARVpnProtocolOpenvpn, openvpn.NvramItems, openvpn.Translate},
}

// ARVpnServerSource mirrors `ARVpnServerSource`: AsusRouter VPN server
// data source (router-global, all protocols).
type ARVpnServerSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARVpnServerSource) SourceName() string { return "ARVpnServerSource" }

// ARVpnServerSourceUniversal is the universal instance - preferred
var ARVpnServerSourceUniversal = &ARVpnServerSource{}

// fetchState mirrors `fetch_state`: fetch the status hook and nvram config
// for every VPN server.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	hooks := []endpoint.ARHookItem{wireguard.Hook}
	for _, b := range backends {
		for _, item := range b.nvramItems() {
			hooks = append(hooks, item)
		}
	}

	request := endpoint.HookRequest(hooks...)
	data, err := c.Read(ctx, endpoint.AREndpointFetchData, request)
	if err != nil {
		return nil, err
	}

	// OpenVPN connected clients live in a separate endpoint; only worth
	// fetching when the server is enabled
	if m, ok := data.(map[string]any); ok && openvpn.ServerEnabled(m) {
		status, err := c.Read(ctx, endpoint.AREndpointFetchVpnOpenvpnStatus, "")
		if err != nil {
			return nil, err
		}
		m[openvpn.ClientStatusKey] = openvpn.ReadClientStatus(status)
	}

	return data, nil
}

// translateState mirrors `translate_state`: raw data into VPN server
// profiles grouped by protocol.
func translateState(raw any, _ *registry.TranslateContext) any {
	data, ok := raw.(map[string]any)
	if !ok {
		return map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnServerField]any{}
	}

	result := map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnServerField]any{}
	for _, b := range backends {
		if servers := b.translate(data); len(servers) > 0 {
			result[b.protocol] = servers
		}
	}

	return result
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARVpnServerSource",
		Default:   func() source.DataSource { return &ARVpnServerSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
