package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/ai"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateAi(t *testing.T) {
	check(t, "translateAi", translateAi, []boolCase{
		{d{k(support.ARSupportValueAi): 1}, true},
		{d{k(support.ARSupportValueAi): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateAiCapabilities(t *testing.T) {
	check(t, "translateAiCapabilities", translateAiCapabilities, []struct {
		data     d
		expected []ai.ARAICapability
	}{
		{d{k(support.ARSupportValueAiSlm): 1}, []ai.ARAICapability{ai.ARAICapabilitySlm}},
		{d{k(support.ARSupportValueAiUpgradeBeta): 1}, []ai.ARAICapability{ai.ARAICapabilityUpgradeBeta}},
		{d{k(support.ARSupportValueAiResetBeta): 1}, []ai.ARAICapability{ai.ARAICapabilityResetBeta}},
		{d{}, []ai.ARAICapability{}},
	})
}
