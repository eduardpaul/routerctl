package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
)

func TestTranslateVpn(t *testing.T) {
	check(t, "translateVpn", translateVpn, []boolCase{
		{d{k(support.ARSupportValueVpnOpenvpn): 1}, true},
		{d{k(support.ARSupportValueVpnIpsec): 5}, true},
		{d{k(support.ARSupportValueVpnOpenvpn): 0}, false},
		{d{}, false},
	})
}

func TestTranslateVpnCapabilities(t *testing.T) {
	type caps = map[vpn.ARVpnCapability]any
	check(t, "translateVpnCapabilities", translateVpnCapabilities, []struct {
		data     d
		expected caps
	}{
		// Non-Fusion capabilities map to True, no connection detail
		{d{k(support.ARSupportValueVpnClient): 1}, caps{vpn.ARVpnCapabilityClient: true}},
		{d{k(support.ARSupportValueVpnIpsec): 5}, caps{vpn.ARVpnCapabilityIpsec: true}},
		// Fusion pulls in FUSION_CONNECTIONS with its int value
		{
			d{k(support.ARSupportValueVpnFusion): 1, k(support.ARSupportValueVpnFusionMaxConnections): 4},
			caps{vpn.ARVpnCapabilityFusion: true, vpn.ARVpnCapabilityFusionConnections: 4},
		},
		// Fusion with 0 -> firmware default 2
		{d{k(support.ARSupportValueVpnFusion): 1}, caps{vpn.ARVpnCapabilityFusion: true, vpn.ARVpnCapabilityFusionConnections: 2}},
		// No Fusion -> the connection flag is ignored
		{d{k(support.ARSupportValueVpnFusionMaxConnections): 4}, caps{}},
		{d{}, caps{}},
	})
}
