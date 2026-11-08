package source

import (
	"testing"

	parentalcontrol "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateParentalControl(t *testing.T) {
	check(t, "translateParentalControl", translateParentalControl, []boolCase{
		{d{k(support.ARSupportValueParentalControl): 1}, true},
		{d{k(support.ARSupportValueParentalControl): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateParentalControlCapabilities(t *testing.T) {
	type caps = map[parentalcontrol.ARParentalControlCapability]int
	check(t, "translateParentalControlCapabilities", translateParentalControlCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		{d{k(support.ARSupportValueParentalControlMaxRules): "16"}, caps{parentalcontrol.ARParentalControlCapabilityMaxRules: 16}},
		// A zero-valued limit is dropped
		{d{k(support.ARSupportValueParentalControlMaxRules): "0"}, caps{}},
		{
			d{
				k(support.ARSupportValueParentalControlMaxRules):     "64",
				k(support.ARSupportValueParentalControlMaxEntries):   "256",
				k(support.ARSupportValueParentalControlSchedVersion): "3",
			},
			caps{
				parentalcontrol.ARParentalControlCapabilityMaxEntries:   256,
				parentalcontrol.ARParentalControlCapabilityMaxRules:     64,
				parentalcontrol.ARParentalControlCapabilitySchedVersion: 3,
			},
		},
	})
}
