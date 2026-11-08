package aura

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

// AuraSupported mirrors `aura_supported`: whether the device exposes Aura.
func AuraSupported(identity *device.ARDeviceIdentity) bool {
	return identity != nil && truthy(
		support.SupportValue(identity.Support(), support.ARSupportTypeAura),
	)
}

// NightModeSupported mirrors `night_mode_supported`: whether the device
// exposes Aura night mode.
func NightModeSupported(identity *device.ARDeviceIdentity) bool {
	return identity != nil && support.SupportAvailableIn(
		identity.Support(),
		support.ARSupportTypeAuraCapabilities,
		ARAuraCapabilityNightMode,
	)
}

// truthy mirrors Python `bool(value)` for support values.
func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}
