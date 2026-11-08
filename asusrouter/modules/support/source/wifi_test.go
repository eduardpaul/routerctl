package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
)

// translate_wifi reports whether the device has WiFi.
func TestTranslateWifi(t *testing.T) {
	check(t, "translateWifi", translateWifi, []boolCase{
		// No capability at all -> no WiFi
		{d{}, false},
		// Explicit opt-out wins
		{d{k(support.ARSupportValueWifiUnitNone): 1}, false},
		{d{k(support.ARSupportValueWifiUnitNone): 1, k(support.ARSupportValueWifiUnit0): 1}, false},
		// Any real capability -> has WiFi
		{d{k(support.ARSupportValueWifiUnit0): 1}, true},
		{d{k(support.ARSupportValueWifi6): 1}, true},
	})
}

func TestTranslateWifiCapabilities(t *testing.T) {
	type caps = map[wifi.ARWiFiCapability]any
	check(t, "translateWifiCapabilities", translateWifiCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		// Generation (highest wins)
		{d{k(support.ARSupportValueWifi7): 1, k(support.ARSupportValueWifi6): 1}, caps{wifi.ARWiFiCapabilityGeneration: wifi.ARWiFiGenerationWifi7}},
		// Multiband (quad wins)
		{
			d{k(support.ARSupportValueWifiBandsQuad): 1, k(support.ARSupportValueWifiBandsTri): 1},
			caps{wifi.ARWiFiCapabilityMultiband: wifi.ARWiFiMultiBandQuadband},
		},
		// Units, in order
		{d{k(support.ARSupportValueWifiUnit0): 1, k(support.ARSupportValueWifiUnit2): 1}, caps{wifi.ARWiFiCapabilityUnits: []int{0, 2}}},
		// noWiFi drops the units
		{d{k(support.ARSupportValueWifiUnitNone): 1}, caps{}},
		// Bool radio capability
		{d{k(support.ARSupportValueWifiOfdmaDl): 1}, caps{wifi.ARWiFiCapabilityOfdmaDl: true}},
		// Smart Connect present only when supported
		{d{k(support.ARSupportValueWifiSmartConnectV2): 1}, caps{wifi.ARWiFiCapabilitySmartConnect: 2}},
		{d{k(support.ARSupportValueWifiBandsteering): 1}, caps{wifi.ARWiFiCapabilitySmartConnect: 1}},
		// A full device
		{
			d{
				k(support.ARSupportValueWifi7):              1,
				k(support.ARSupportValueWifiBandsTri):       1,
				k(support.ARSupportValueWifiUnit0):          1,
				k(support.ARSupportValueWifiUnit1):          1,
				k(support.ARSupportValueWifiUnit2):          1,
				k(support.ARSupportValueWifiMbo):            1,
				k(support.ARSupportValueWifiMlo):            1,
				k(support.ARSupportValueWifiPowerControl):   1,
				k(support.ARSupportValueWifiSmartConnectV2): 1,
			},
			caps{
				wifi.ARWiFiCapabilityMbo:          true,
				wifi.ARWiFiCapabilityMlo:          true,
				wifi.ARWiFiCapabilityPowerControl: true,
				wifi.ARWiFiCapabilityGeneration:   wifi.ARWiFiGenerationWifi7,
				wifi.ARWiFiCapabilityMultiband:    wifi.ARWiFiMultiBandTriband,
				wifi.ARWiFiCapabilitySmartConnect: 2,
				wifi.ARWiFiCapabilityUnits:        []int{0, 1, 2},
			},
		},
	})
}
