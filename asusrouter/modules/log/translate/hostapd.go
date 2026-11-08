package translate

// Host AP daemon log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// HostapdPatterns mirrors `hostapd.PATTERNS`.
var HostapdPatterns = NewARLogPatternSet(
	AREventHostapdUnknown,
	MustARLogPattern(
		AREventHostapdStaAddFailed,
		WlPattern+`: STA (?P<client_mac>`+MacPattern+`) `+
			`IEEE 802\.11: Could not add STA to kernel driver`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
				log.AREventKeyWlId:      FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
			},
			Marker: "Could not add STA to kernel driver",
		},
	),
)
