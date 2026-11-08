package device

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
)

// Recover identity values missing from the raw device data.

type modelGeneration struct {
	marker     string
	generation wifi.ARWiFiGeneration
}

// Model marker -> WiFi generation
var modelWifiGeneration = []modelGeneration{
	{"be", wifi.ARWiFiGenerationWifi7},
	{"axe", wifi.ARWiFiGenerationWifi6},
	{"ax", wifi.ARWiFiGenerationWifi6},
	{"ac", wifi.ARWiFiGenerationWifi5},
}

// wifiGenerationFromModel guesses the WiFi generation from the model name
// marker.
func wifiGenerationFromModel(model *string) wifi.ARWiFiGeneration {
	if model == nil || *model == "" {
		return wifi.ARWiFiGenerationUnknown
	}
	lowered := strings.ToLower(*model)
	for _, mg := range modelWifiGeneration {
		if strings.Contains(lowered, mg.marker) {
			return mg.generation
		}
	}
	return wifi.ARWiFiGenerationUnknown
}

// recoverSupport recovers unknown values from the complimentary data.
//
// It runs during the build, before the identity is shared, so it works on
// the fields directly.
func recoverSupport(identity *ARDeviceIdentity) {
	sup := identity.support
	capabilities, _ := sup[support.ARSupportTypeWifiCapabilities].(map[wifi.ARWiFiCapability]any)

	recovered := map[wifi.ARWiFiCapability]any{}

	if _, ok := capabilities[wifi.ARWiFiCapabilityGeneration]; !ok {
		if generation := wifiGenerationFromModel(identity.model); generation != wifi.ARWiFiGenerationUnknown {
			recovered[wifi.ARWiFiCapabilityGeneration] = generation
		}
	}

	if _, ok := capabilities[wifi.ARWiFiCapabilityMultiband]; !ok {
		if multiband := wifi.ARWiFiMultiBandFromValue(len(identity.wifi.units)); multiband != wifi.ARWiFiMultiBandUnknown {
			recovered[wifi.ARWiFiCapabilityMultiband] = multiband
		}
	}

	if len(recovered) > 0 {
		// Copy before updating: the capabilities map may be shared with
		// the support source state
		merged := make(map[wifi.ARWiFiCapability]any, len(capabilities)+len(recovered))
		for k, v := range capabilities {
			merged[k] = v
		}
		for k, v := range recovered {
			merged[k] = v
		}
		if sup == nil {
			sup = map[support.ARSupportType]any{}
			identity.support = sup
		}
		sup[support.ARSupportTypeWifiCapabilities] = merged
	}
}
