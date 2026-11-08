package source

import (
	parentalcontrol "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

var translateParentalControl = support.MakeBoolTranslator(string(support.ARSupportValueParentalControl), false)

var parentalControlCapabilityInts = []support.TableEntry[parentalcontrol.ARParentalControlCapability]{
	{Key: string(support.ARSupportValueParentalControlMaxEntries), Value: parentalcontrol.ARParentalControlCapabilityMaxEntries},
	{Key: string(support.ARSupportValueParentalControlMaxRules), Value: parentalcontrol.ARParentalControlCapabilityMaxRules},
	{Key: string(support.ARSupportValueParentalControlSchedVersion), Value: parentalcontrol.ARParentalControlCapabilitySchedVersion},
}

// translateParentalControlCapabilities maps parental control limits and
// version, present only when reported.
func translateParentalControlCapabilities(data map[string]any) map[parentalcontrol.ARParentalControlCapability]int {
	capabilities := map[parentalcontrol.ARParentalControlCapability]int{}

	for _, entry := range parentalControlCapabilityInts {
		if value := converters.RawToInt(data[entry.Key]); value != nil && *value != 0 {
			capabilities[entry.Value] = *value
		}
	}

	return capabilities
}
