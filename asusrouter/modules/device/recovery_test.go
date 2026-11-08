package device

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
)

// TestWifiGenerationFromModel: the WiFi generation is read from the model
// name marker.
func TestWifiGenerationFromModel(t *testing.T) {
	cases := []struct {
		model    *string
		expected wifi.ARWiFiGeneration
	}{
		{strp("RT-AC66U"), wifi.ARWiFiGenerationWifi5},
		{strp("RT-AC5300"), wifi.ARWiFiGenerationWifi5},
		{strp("RT-AX88U"), wifi.ARWiFiGenerationWifi6},
		{strp("GT-AXE16000"), wifi.ARWiFiGenerationWifi6},
		{strp("GT-BE98"), wifi.ARWiFiGenerationWifi7},
		// No marker in the name
		{strp("ZenWiFi-XT8"), wifi.ARWiFiGenerationUnknown},
		{nil, wifi.ARWiFiGenerationUnknown},
		{strp(""), wifi.ARWiFiGenerationUnknown},
	}
	for _, tc := range cases {
		if got := wifiGenerationFromModel(tc.model); got != tc.expected {
			t.Errorf("%v: got %v, want %v", tc.model, got, tc.expected)
		}
	}
}

func recoveryIdentity(model *string, bands int, generation wifi.ARWiFiGeneration, multiband wifi.ARWiFiMultiBand) *ARDeviceIdentity {
	identity := NewARDeviceIdentity()
	identity.model = model
	// list(ARWiFiBand)[1 : bands + 1], unit = position
	for unit, band := range wifi.ARWiFiBandMembers[1 : bands+1] {
		identity.wifi.set(band, unit)
	}
	capabilities := map[wifi.ARWiFiCapability]any{}
	if generation != wifi.ARWiFiGenerationUnknown {
		capabilities[wifi.ARWiFiCapabilityGeneration] = generation
	}
	if multiband != wifi.ARWiFiMultiBandUnknown {
		capabilities[wifi.ARWiFiCapabilityMultiband] = multiband
	}
	identity.support = map[support.ARSupportType]any{support.ARSupportTypeWifiCapabilities: capabilities}
	return identity
}

func caps(identity *ARDeviceIdentity) map[wifi.ARWiFiCapability]any {
	return identity.Support()[support.ARSupportTypeWifiCapabilities].(map[wifi.ARWiFiCapability]any)
}

// TestRecoversBothFromLegacyData: a legacy device recovers generation from
// model and band count.
func TestRecoversBothFromLegacyData(t *testing.T) {
	identity := recoveryIdentity(strp("RT-AC66U"), 2, wifi.ARWiFiGenerationUnknown, wifi.ARWiFiMultiBandUnknown)
	recoverSupport(identity)
	c := caps(identity)
	if c[wifi.ARWiFiCapabilityGeneration] != wifi.ARWiFiGenerationWifi5 {
		t.Errorf("generation = %v", c[wifi.ARWiFiCapabilityGeneration])
	}
	if c[wifi.ARWiFiCapabilityMultiband] != wifi.ARWiFiMultiBandDualband {
		t.Errorf("multiband = %v", c[wifi.ARWiFiCapabilityMultiband])
	}
}

// TestKeepsFetchedValues: a real fetched value is never overwritten by
// recovery.
func TestKeepsFetchedValues(t *testing.T) {
	identity := recoveryIdentity(strp("RT-AC66U"), 2, wifi.ARWiFiGenerationWifi7, wifi.ARWiFiMultiBandTriband)
	recoverSupport(identity)
	c := caps(identity)
	if c[wifi.ARWiFiCapabilityGeneration] != wifi.ARWiFiGenerationWifi7 {
		t.Errorf("generation = %v", c[wifi.ARWiFiCapabilityGeneration])
	}
	if c[wifi.ARWiFiCapabilityMultiband] != wifi.ARWiFiMultiBandTriband {
		t.Errorf("multiband = %v", c[wifi.ARWiFiCapabilityMultiband])
	}
}

// TestUnrecoverableStaysUnknown: no model marker and no bands leaves both
// unknown.
func TestUnrecoverableStaysUnknown(t *testing.T) {
	identity := recoveryIdentity(strp("ZenWiFi-XT8"), 0, wifi.ARWiFiGenerationUnknown, wifi.ARWiFiMultiBandUnknown)
	recoverSupport(identity)
	c := caps(identity)
	if _, ok := c[wifi.ARWiFiCapabilityGeneration]; ok {
		t.Error("generation recovered")
	}
	if _, ok := c[wifi.ARWiFiCapabilityMultiband]; ok {
		t.Error("multiband recovered")
	}
}
