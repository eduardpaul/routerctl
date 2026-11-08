package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var wifiCapabilityFlags = []support.TableEntry[wifi.ARWiFiCapability]{
	{Key: string(support.ARSupportValueWifiMbo), Value: wifi.ARWiFiCapabilityMbo},
	{Key: string(support.ARSupportValueWifiMlo), Value: wifi.ARWiFiCapabilityMlo},
	{Key: string(support.ARSupportValueWifiMumimo), Value: wifi.ARWiFiCapabilityMumimo},
	{Key: string(support.ARSupportValueWifiOfdma), Value: wifi.ARWiFiCapabilityOfdma},
	{Key: string(support.ARSupportValueWifiOfdmaDl), Value: wifi.ARWiFiCapabilityOfdmaDl},
	{Key: string(support.ARSupportValueWifiPowerControl), Value: wifi.ARWiFiCapabilityPowerControl},
}

// First match wins
var wifiGeneration = support.MakeEnumTranslator([]support.TableEntry[wifi.ARWiFiGeneration]{
	{Key: string(support.ARSupportValueWifi7), Value: wifi.ARWiFiGenerationWifi7},
	{Key: string(support.ARSupportValueWifi6), Value: wifi.ARWiFiGenerationWifi6},
	{Key: string(support.ARSupportValueWifi5), Value: wifi.ARWiFiGenerationWifi5},
}, wifi.ARWiFiGenerationUnknown)

// First match wins
var wifiMultiband = support.MakeEnumTranslator([]support.TableEntry[wifi.ARWiFiMultiBand]{
	{Key: string(support.ARSupportValueWifiBandsQuad), Value: wifi.ARWiFiMultiBandQuadband},
	{Key: string(support.ARSupportValueWifiBandsTri), Value: wifi.ARWiFiMultiBandTriband},
	{Key: string(support.ARSupportValueWifiBandsDual), Value: wifi.ARWiFiMultiBandDualband},
}, wifi.ARWiFiMultiBandUnknown)

var wifiUnits = []support.TableEntry[int]{
	{Key: string(support.ARSupportValueWifiUnit0), Value: 0},
	{Key: string(support.ARSupportValueWifiUnit1), Value: 1},
	{Key: string(support.ARSupportValueWifiUnit2), Value: 2},
}

// smartConnect returns the band steering level: 2 for v2, 1 for v1, 0
// when unsupported.
func smartConnect(data map[string]any) int {
	if readers.IsTrueInDict(string(support.ARSupportValueWifiSmartConnectV2), data) {
		return 2
	}
	if readers.IsTrueInDict(string(support.ARSupportValueWifiSmartConnect), data) ||
		readers.IsTrueInDict(string(support.ARSupportValueWifiBandsteering), data) {
		return 1
	}
	return 0
}

// units returns the wireless unit indices; empty when the device reports
// no WiFi.
func units(data map[string]any) []int {
	out := []int{}
	if readers.IsTrueInDict(string(support.ARSupportValueWifiUnitNone), data) {
		return out
	}
	for _, entry := range wifiUnits {
		if readers.IsTrueInDict(entry.Key, data) {
			out = append(out, entry.Value)
		}
	}
	return out
}

// translateWifiCapabilities maps advertised WiFi capabilities; each key
// present only when known. Values are bool, int, ARWiFiGeneration,
// ARWiFiMultiBand or []int (UNITS).
func translateWifiCapabilities(data map[string]any) map[wifi.ARWiFiCapability]any {
	capabilities := map[wifi.ARWiFiCapability]any{}
	for _, entry := range wifiCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}

	if generation := wifiGeneration(data); generation != wifi.ARWiFiGenerationUnknown {
		capabilities[wifi.ARWiFiCapabilityGeneration] = generation
	}

	if multiband := wifiMultiband(data); multiband != wifi.ARWiFiMultiBandUnknown {
		capabilities[wifi.ARWiFiCapabilityMultiband] = multiband
	}

	if sc := smartConnect(data); sc != 0 {
		capabilities[wifi.ARWiFiCapabilitySmartConnect] = sc
	}

	if u := units(data); len(u) > 0 {
		capabilities[wifi.ARWiFiCapabilityUnits] = u
	}

	return capabilities
}

// translateWifi reports whether the device has WiFi - false when it opts
// out or has none.
func translateWifi(data map[string]any) bool {
	if readers.IsTrueInDict(string(support.ARSupportValueWifiUnitNone), data) {
		return false
	}
	return len(translateWifiCapabilities(data)) > 0
}
