package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wan"
)

func TestTranslateWan(t *testing.T) {
	check(t, "translateWan", translateWan, []boolCase{
		{d{k(support.ARSupportValueWanNowan): 1}, false},
		{d{k(support.ARSupportValueWanNowan): "0"}, true},
		{d{}, true},
	})
}

func TestTranslateWanCapabilities(t *testing.T) {
	type caps = map[wan.ARWANCapability]any
	check(t, "translateWanCapabilities", translateWanCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		{d{k(support.ARSupportValueWanAggregation): 1}, caps{wan.ARWANCapabilityAggregation: true}},
		{d{k(support.ARSupportValueWanDualwan): 1}, caps{wan.ARWANCapabilityDualwan: true}},
		{d{k(support.ARSupportValueWanRealIp): 1}, caps{wan.ARWANCapabilityRealIp: true}},
		// LIMIT carries the port count, present only when reported
		{d{k(support.ARSupportValueWanLimit): 2}, caps{wan.ARWANCapabilityLimit: 2}},
		{d{k(support.ARSupportValueWanLimit): "0"}, caps{}},
		{
			d{k(support.ARSupportValueWanAggregation): 1, k(support.ARSupportValueWanDualwan): 1, k(support.ARSupportValueWanLimit): 2},
			caps{wan.ARWANCapabilityAggregation: true, wan.ARWANCapabilityDualwan: true, wan.ARWANCapabilityLimit: 2},
		},
	})
}
