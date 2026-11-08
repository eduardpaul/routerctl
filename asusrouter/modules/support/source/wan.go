package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wan"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var translateWan = support.MakeBoolTranslator(string(support.ARSupportValueWanNowan), true)

var wanCapabilityFlags = []support.TableEntry[wan.ARWANCapability]{
	{Key: string(support.ARSupportValueWanAggregation), Value: wan.ARWANCapabilityAggregation},
	{Key: string(support.ARSupportValueWanDualwan), Value: wan.ARWANCapabilityDualwan},
	{Key: string(support.ARSupportValueWanRealIp), Value: wan.ARWANCapabilityRealIp},
}

// translateWanCapabilities maps advertised WAN capabilities; LIMIT
// carries the port count.
func translateWanCapabilities(data map[string]any) map[wan.ARWANCapability]any {
	capabilities := map[wan.ARWANCapability]any{}
	for _, entry := range wanCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}

	if limit := converters.RawToInt(data[string(support.ARSupportValueWanLimit)]); limit != nil && *limit != 0 {
		capabilities[wan.ARWANCapabilityLimit] = *limit
	}

	return capabilities
}
