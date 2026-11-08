// Shared helpers for the network module.

package network

import (
	"regexp"

	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/units"
)

// Matches a colon-separated MAC address anywhere in a string
var macRE = regexp.MustCompile(`[0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5}`)

// Raw filter mode value -> mode (both numeric and text are seen)
var macFilterModes = map[string]wifi.ARWiFiMacFilterMode{
	"0":        wifi.ARWiFiMacFilterModeDisabled,
	"1":        wifi.ARWiFiMacFilterModeAllow,
	"2":        wifi.ARWiFiMacFilterModeDeny,
	"disabled": wifi.ARWiFiMacFilterModeDisabled,
	"allow":    wifi.ARWiFiMacFilterModeAllow,
	"deny":     wifi.ARWiFiMacFilterModeDeny,
}

// ReadMacList mirrors `read_mac_list`: read a MAC filter list into MAC
// addresses.
func ReadMacList(raw any) []*identifiers.MacAddress {
	var macs []*identifiers.MacAddress
	for _, token := range macRE.FindAllString(readers.Decode(raw), -1) {
		if mac := identifiers.MacAddressFromValueSafe(token); mac != nil {
			macs = append(macs, mac)
		}
	}
	return macs
}

// MacFilterMode mirrors `mac_filter_mode`: map a raw filter mode value to
// a mode, or nil if unknown.
func MacFilterMode(raw any) *wifi.ARWiFiMacFilterMode {
	mode, ok := macFilterModes[converters.PyStr(raw)]
	if !ok {
		return nil
	}
	return &mode
}

// toBits mirrors `_to_bits`: convert a Kib/s rate to bits/s, or nil if
// unparsable.
func toBits(raw any) *int {
	value := converters.RawToInt(raw)
	if value == nil {
		return nil
	}
	// The unit is a known constant, so the conversion cannot fail
	base, _ := units.DataRateUnitConverter.ConvertToBase(
		float64(*value), units.UnitOfDataRateKibibitPerSecond,
	)
	bits := int(base)
	return &bits
}

// BandwidthLimit mirrors `bandwidth_limit`: build the bandwidth-limiter
// fields, empty when the limiter is off.
//
// Both backends store the per-direction rate in Kib/s; it is exposed in
// bits/s.
func BandwidthLimit(enabled, download, upload any) map[ARNetworkField]any {
	if b := converters.RawToBool(enabled); b == nil || !*b {
		return map[ARNetworkField]any{}
	}

	fields := map[ARNetworkField]any{}
	if down := toBits(download); down != nil {
		fields[ARNetworkFieldBandwidthLimitDownload] = *down
	}
	if up := toBits(upload); up != nil {
		fields[ARNetworkFieldBandwidthLimitUpload] = *up
	}
	return fields
}
