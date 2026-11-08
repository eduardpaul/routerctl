package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/aura"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var translateAura = support.MakeBoolTranslator(string(support.ARSupportValueAura), false)

// translateAuraCapabilities maps advertised Aura capabilities; ZONE
// carries the zone count.
func translateAuraCapabilities(data map[string]any) map[aura.ARAuraCapability]any {
	capabilities := map[aura.ARAuraCapability]any{}

	if readers.IsTrueInDict(string(support.ARSupportValueAuraNightMode), data) {
		capabilities[aura.ARAuraCapabilityNightMode] = true
	}

	if zones := converters.RawToInt(data[string(support.ARSupportValueAuraZone)]); zones != nil && *zones != 0 {
		capabilities[aura.ARAuraCapabilityZone] = *zones
	}

	return capabilities
}
