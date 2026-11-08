package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var translateAimesh = support.MakeBoolTranslator(string(support.ARSupportValueAimesh), false)

var aimeshCapabilityFlags = []support.TableEntry[aimesh.ARAiMeshCapability]{
	{Key: string(support.ARSupportValueAimeshNewOnboarding), Value: aimesh.ARAiMeshCapabilityNewOnboarding},
	{Key: string(support.ARSupportValueAimeshNode), Value: aimesh.ARAiMeshCapabilityNode},
	{Key: string(support.ARSupportValueAimeshRouter), Value: aimesh.ARAiMeshCapabilityRouter},
}

// translateAimeshCapabilities maps advertised AiMesh capabilities;
// GENERATION carries the version.
func translateAimeshCapabilities(data map[string]any) map[aimesh.ARAiMeshCapability]any {
	capabilities := map[aimesh.ARAiMeshCapability]any{}
	for _, entry := range aimeshCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}

	if generation := converters.RawToInt(data[string(support.ARSupportValueAimesh)]); generation != nil && *generation != 0 {
		capabilities[aimesh.ARAiMeshCapabilityGeneration] = *generation
	}

	return capabilities
}
