package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Firmware default active connections when Fusion reports 0
const vpnFusionDefaultConnections = 2

var vpnCapabilityFlags = []support.TableEntry[vpn.ARVpnCapability]{
	{Key: string(support.ARSupportValueVpnClient), Value: vpn.ARVpnCapabilityClient},
	{Key: string(support.ARSupportValueVpnFusion), Value: vpn.ARVpnCapabilityFusion},
	{Key: string(support.ARSupportValueVpnIpsec), Value: vpn.ARVpnCapabilityIpsec},
	{Key: string(support.ARSupportValueVpnOpenvpn), Value: vpn.ARVpnCapabilityOpenvpn},
	{Key: string(support.ARSupportValueVpnPptp), Value: vpn.ARVpnCapabilityPptp},
	{Key: string(support.ARSupportValueVpnWireguard), Value: vpn.ARVpnCapabilityWireguard},
}

// fusionConnections returns the max Fusion connections active at once; 0
// reports the default.
func fusionConnections(data map[string]any) int {
	if value := converters.RawToInt(data[string(support.ARSupportValueVpnFusionMaxConnections)]); value != nil && *value != 0 {
		return *value
	}
	return vpnFusionDefaultConnections
}

// translateVpnCapabilities maps advertised VPN capabilities;
// FUSION_CONNECTIONS carries an int.
func translateVpnCapabilities(data map[string]any) map[vpn.ARVpnCapability]any {
	capabilities := map[vpn.ARVpnCapability]any{}
	for _, entry := range vpnCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}
	if _, ok := capabilities[vpn.ARVpnCapabilityFusion]; ok {
		capabilities[vpn.ARVpnCapabilityFusionConnections] = fusionConnections(data)
	}
	return capabilities
}

// translateVpn reports whether the device advertises any VPN capability.
func translateVpn(data map[string]any) bool {
	return len(translateVpnCapabilities(data)) > 0
}
