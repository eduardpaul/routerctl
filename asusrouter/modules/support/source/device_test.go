package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateDeviceMode(t *testing.T) {
	type modes = []common.AROperationMode
	check(t, "translateDeviceMode", translateDeviceMode, []struct {
		data     d
		expected modes
	}{
		// Empty: router and access point are available by default
		{d{}, modes{common.AROperationModeRouter, common.AROperationModeAccessPoint}},
		// Negative flags remove the defaults
		{d{k(support.ARSupportValueModeNoRouter): 1, k(support.ARSupportValueModeNoAccessPoint): 1}, modes{}},
		{
			d{k(support.ARSupportValueModeRepeater): 1},
			modes{common.AROperationModeRouter, common.AROperationModeRepeater, common.AROperationModeAccessPoint},
		},
		// Media bridge from either token, deduped
		{
			d{
				k(support.ARSupportValueModeNoRouter):            1,
				k(support.ARSupportValueModeNoAccessPoint):       1,
				k(support.ARSupportValueModeMediaBridge):         1,
				k(support.ARSupportValueModeMediaBridgeProxysta): 1,
			},
			modes{common.AROperationModeMediaBridge},
		},
		// AiMesh node from the shared amasNode flag
		{
			d{
				k(support.ARSupportValueModeNoRouter):      1,
				k(support.ARSupportValueModeNoAccessPoint): 1,
				k(support.ARSupportValueAimeshNode):        1,
			},
			modes{common.AROperationModeAimeshNode},
		},
		// All five, ordered by mode value
		{
			d{
				k(support.ARSupportValueModeRepeater):    1,
				k(support.ARSupportValueModeMediaBridge): 1,
				k(support.ARSupportValueAimeshNode):      1,
			},
			modes{
				common.AROperationModeRouter,
				common.AROperationModeRepeater,
				common.AROperationModeAccessPoint,
				common.AROperationModeMediaBridge,
				common.AROperationModeAimeshNode,
			},
		},
	})
}
