package translate

// System init log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// InitPatterns mirrors `init.PATTERNS`.
var InitPatterns = NewARLogPatternSet(
	AREventInitUnknown,
	MustARLogPattern(
		AREventInitFirmwareBanner,
		`fwver: (?P<version>\S+)`+
			`(?: \(sn:(?P<device_serial>\S+)`+
			` /ha:(?P<device_mac>[0-9A-Fa-f:]+)\s*\))?`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyDeviceMac:    FromValueSafe(identifiers.MacAddressFromValueSafe),
				log.AREventKeyDeviceSerial: FromValueSafe(identifiers.SerialFromValueSafe),
			},
			Marker: "fwver:",
		},
	),
)
