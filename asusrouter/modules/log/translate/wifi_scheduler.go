package translate

// WiFi scheduler log events for AsusRouter.

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// The scheduler names a radio by its two indices instead of its `wl`
// token, and they are the same pair
const wifiSchedulerSeparator = ", subunit="

// wifiSchedulerRadio reads `0, subunit=2` as the interface those indices
// name.
func wifiSchedulerRadio(value string) any {
	unit, subunit, _ := strings.Cut(value, wifiSchedulerSeparator)

	if iface := identifiers.WiFiInterfaceFromValueSafe(unit + "." + subunit); iface != nil {
		return iface
	}
	return nil
}

// WifiSchedulerPatterns mirrors `wifi_scheduler.PATTERNS`.
var WifiSchedulerPatterns = NewARLogPatternSet(
	AREventWifiSchedulerUnknown,
	MustARLogPattern(
		map[string]StrEnum{
			"off": AREventWifiSchedulerRadioOff,
			"on":  AREventWifiSchedulerRadioOn,
		},
		`Turn radio \[band_index=(?P<wl_id>\d+`+wifiSchedulerSeparator+`\d+)\] `+
			`(?P<event_type>on|off)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyWlId: Plain(wifiSchedulerRadio)},
			Marker:  "Turn radio",
		},
	),
)
