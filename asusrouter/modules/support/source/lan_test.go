package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/lan"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateLanCapabilities(t *testing.T) {
	check(t, "translateLanCapabilities", translateLanCapabilities, []struct {
		data     d
		expected []lan.ARLANCapability
	}{
		{d{k(support.ARSupportValueLanAggregation): 1}, []lan.ARLANCapability{lan.ARLANCapabilityAggregation}},
		{d{}, []lan.ARLANCapability{}},
	})
}
