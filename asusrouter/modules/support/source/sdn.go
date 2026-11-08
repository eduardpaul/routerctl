package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/network"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

var translateSdn = support.MakeBoolTranslator(string(support.ARSupportValueSdn), false)

var sdnCapabilityInts = []support.TableEntry[network.ARSDNCapability]{
	{Key: string(support.ARSupportValueSdnAwv), Value: network.ARSDNCapabilityAwv},
	{Key: string(support.ARSupportValueSdnMainfh), Value: network.ARSDNCapabilityMainFronthaul},
	{Key: string(support.ARSupportValueSdnMaxRules), Value: network.ARSDNCapabilityMaxRules},
	{Key: string(support.ARSupportValueSdnMwl), Value: network.ARSDNCapabilityMwl},
	{Key: string(support.ARSupportValueSdnPriority), Value: network.ARSDNCapabilityPriority},
}

// translateSdnCapabilities maps SDN capabilities, each present only when
// reported.
func translateSdnCapabilities(data map[string]any) map[network.ARSDNCapability]int {
	capabilities := map[network.ARSDNCapability]int{}
	for _, entry := range sdnCapabilityInts {
		if value := converters.RawToInt(data[entry.Key]); value != nil && *value != 0 {
			capabilities[entry.Value] = *value
		}
	}
	return capabilities
}
