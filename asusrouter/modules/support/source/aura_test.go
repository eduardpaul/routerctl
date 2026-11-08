package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aura"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateAura(t *testing.T) {
	check(t, "translateAura", translateAura, []boolCase{
		{d{k(support.ARSupportValueAura): 1}, true},
		{d{k(support.ARSupportValueAura): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateAuraCapabilities(t *testing.T) {
	type caps = map[aura.ARAuraCapability]any
	check(t, "translateAuraCapabilities", translateAuraCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		{d{k(support.ARSupportValueAuraNightMode): 1}, caps{aura.ARAuraCapabilityNightMode: true}},
		// ZONE carries the zone count, present only when > 0
		{d{k(support.ARSupportValueAuraZone): 4}, caps{aura.ARAuraCapabilityZone: 4}},
		{d{k(support.ARSupportValueAuraZone): "0"}, caps{}},
		{
			d{k(support.ARSupportValueAuraNightMode): 1, k(support.ARSupportValueAuraZone): 3},
			caps{aura.ARAuraCapabilityNightMode: true, aura.ARAuraCapabilityZone: 3},
		},
	})
}
