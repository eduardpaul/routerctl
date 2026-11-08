package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// translateDeviceMode lists the operation modes the device can switch to.
func translateDeviceMode(data map[string]any) []common.AROperationMode {
	modes := []common.AROperationMode{}

	if !readers.IsTrueInDict(string(support.ARSupportValueModeNoRouter), data) {
		modes = append(modes, common.AROperationModeRouter)
	}

	if readers.IsTrueInDict(string(support.ARSupportValueModeRepeater), data) {
		modes = append(modes, common.AROperationModeRepeater)
	}

	if !readers.IsTrueInDict(string(support.ARSupportValueModeNoAccessPoint), data) {
		modes = append(modes, common.AROperationModeAccessPoint)
	}

	if readers.IsTrueInDict(string(support.ARSupportValueModeMediaBridge), data) ||
		readers.IsTrueInDict(string(support.ARSupportValueModeMediaBridgeProxysta), data) {
		modes = append(modes, common.AROperationModeMediaBridge)
	}

	if readers.IsTrueInDict(string(support.ARSupportValueAimeshNode), data) {
		modes = append(modes, common.AROperationModeAimeshNode)
	}

	return modes
}
