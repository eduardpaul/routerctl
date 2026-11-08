package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/network"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateSdn(t *testing.T) {
	check(t, "translateSdn", translateSdn, []boolCase{
		{d{k(support.ARSupportValueSdn): 6}, true},
		{d{k(support.ARSupportValueSdn): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateSdnCapabilities(t *testing.T) {
	type caps = map[network.ARSDNCapability]int
	check(t, "translateSdnCapabilities", translateSdnCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		{d{k(support.ARSupportValueSdnMaxRules): "19"}, caps{network.ARSDNCapabilityMaxRules: 19}},
		// A zero-valued flag is dropped
		{d{k(support.ARSupportValueSdnMwl): "0"}, caps{}},
		{
			d{
				k(support.ARSupportValueSdnAwv):      1,
				k(support.ARSupportValueSdnMainfh):   1,
				k(support.ARSupportValueSdnMaxRules): 19,
				k(support.ARSupportValueSdnMwl):      6,
				k(support.ARSupportValueSdnPriority): 1,
			},
			caps{
				network.ARSDNCapabilityAwv:           1,
				network.ARSDNCapabilityMainFronthaul: 1,
				network.ARSDNCapabilityMaxRules:      19,
				network.ARSDNCapabilityMwl:           6,
				network.ARSDNCapabilityPriority:      1,
			},
		},
	})
}
