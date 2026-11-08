package source

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

func TestTranslateFirmware(t *testing.T) {
	check(t, "translateFirmware", translateFirmware, []boolCase{
		// Manual upload is available by default
		{d{}, true},
		{d{k(support.ARSupportValueFirmwareLiveUpdate): 1}, true},
		// Opting out of manual upload with no other capability -> none
		{d{k(support.ARSupportValueFirmwareNoManual): 1}, false},
	})
}

func TestTranslateFirmwareCapabilities(t *testing.T) {
	type caps = map[firmware.ARFirmwareCapability]bool
	check(t, "translateFirmwareCapabilities", translateFirmwareCapabilities, []struct {
		data     d
		expected caps
	}{
		// Manual upload present by default
		{d{}, caps{firmware.ARFirmwareCapabilityManualUpload: true}},
		// noFwManual removes it
		{d{k(support.ARSupportValueFirmwareNoManual): 1}, caps{}},
		// Live update, plus the default manual upload
		{
			d{k(support.ARSupportValueFirmwareLiveUpdate): 2},
			caps{firmware.ARFirmwareCapabilityLiveUpdate: true, firmware.ARFirmwareCapabilityManualUpload: true},
		},
		// noupdate disables live update but not manual upload
		{
			d{k(support.ARSupportValueFirmwareLiveUpdate): 1, k(support.ARSupportValueFirmwareNoUpdate): 1},
			caps{firmware.ARFirmwareCapabilityManualUpload: true},
		},
		// Auto upgrade, beta and revert
		{
			d{
				k(support.ARSupportValueFirmwareAutoUpgrade): 1,
				k(support.ARSupportValueFirmwareBeta):        1,
				k(support.ARSupportValueFirmwareRevert):      1,
				k(support.ARSupportValueFirmwareNoManual):    1,
			},
			caps{
				firmware.ARFirmwareCapabilityAutoUpgrade: true,
				firmware.ARFirmwareCapabilityBeta:        true,
				firmware.ARFirmwareCapabilityRevert:      true,
			},
		},
	})
}
