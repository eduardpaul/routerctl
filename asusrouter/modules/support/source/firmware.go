package source

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

var firmwareCapabilityFlags = []support.TableEntry[firmware.ARFirmwareCapability]{
	{Key: string(support.ARSupportValueFirmwareAutoUpgrade), Value: firmware.ARFirmwareCapabilityAutoUpgrade},
	{Key: string(support.ARSupportValueFirmwareBeta), Value: firmware.ARFirmwareCapabilityBeta},
	{Key: string(support.ARSupportValueFirmwareRevert), Value: firmware.ARFirmwareCapabilityRevert},
}

// translateFirmwareCapabilities maps advertised firmware-management
// capabilities.
func translateFirmwareCapabilities(data map[string]any) map[firmware.ARFirmwareCapability]bool {
	capabilities := map[firmware.ARFirmwareCapability]bool{}
	for _, entry := range firmwareCapabilityFlags {
		if readers.IsTrueInDict(entry.Key, data) {
			capabilities[entry.Value] = true
		}
	}

	if readers.IsTrueInDict(string(support.ARSupportValueFirmwareLiveUpdate), data) &&
		!readers.IsTrueInDict(string(support.ARSupportValueFirmwareNoUpdate), data) {
		capabilities[firmware.ARFirmwareCapabilityLiveUpdate] = true
	}

	// Manual upload is available unless the device opts out
	if !readers.IsTrueInDict(string(support.ARSupportValueFirmwareNoManual), data) {
		capabilities[firmware.ARFirmwareCapabilityManualUpload] = true
	}
	return capabilities
}

// translateFirmware reports whether the device advertises any firmware
// capability.
func translateFirmware(data map[string]any) bool {
	return len(translateFirmwareCapabilities(data)) > 0
}
