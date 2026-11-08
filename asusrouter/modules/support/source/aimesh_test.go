package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateAimesh(t *testing.T) {
	check(t, "translateAimesh", translateAimesh, []boolCase{
		{d{k(support.ARSupportValueAimesh): 1}, true},
		{d{k(support.ARSupportValueAimesh): "0"}, false},
		{d{}, false},
	})
}

func TestTranslateAimeshCapabilities(t *testing.T) {
	type caps = map[aimesh.ARAiMeshCapability]any
	check(t, "translateAimeshCapabilities", translateAimeshCapabilities, []struct {
		data     d
		expected caps
	}{
		{d{}, caps{}},
		{d{k(support.ARSupportValueAimeshNewOnboarding): 1}, caps{aimesh.ARAiMeshCapabilityNewOnboarding: true}},
		{d{k(support.ARSupportValueAimeshNode): 1}, caps{aimesh.ARAiMeshCapabilityNode: true}},
		// GENERATION carries the amas version, present only when > 0
		{d{k(support.ARSupportValueAimesh): 2}, caps{aimesh.ARAiMeshCapabilityGeneration: 2}},
		{d{k(support.ARSupportValueAimesh): "0"}, caps{}},
		{
			d{
				k(support.ARSupportValueAimesh):              3,
				k(support.ARSupportValueAimeshNode):          1,
				k(support.ARSupportValueAimeshRouter):        1,
				k(support.ARSupportValueAimeshNewOnboarding): 1,
			},
			caps{
				aimesh.ARAiMeshCapabilityNewOnboarding: true,
				aimesh.ARAiMeshCapabilityNode:          true,
				aimesh.ARAiMeshCapabilityRouter:        true,
				aimesh.ARAiMeshCapabilityGeneration:    3,
			},
		},
	})
}
