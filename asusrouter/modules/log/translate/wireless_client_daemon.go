package translate

// Wireless client event daemon log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Shared by both station messages
var wirelessClientDaemonStation = map[log.AREventKey]Converter{
	log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
	log.AREventKeyRssi:      Int,
	log.AREventKeyWlId:      FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
}

// WirelessClientDaemonPatterns mirrors `wireless_client_daemon.PATTERNS`.
var WirelessClientDaemonPatterns = NewARLogPatternSet(
	AREventWirelessClientDaemonUnknown,
	MustARLogPattern(
		map[string]StrEnum{
			"Assoc":   AREventWirelessClientDaemonAssoc,
			"Auth":    AREventWirelessClientDaemonAuth,
			"ReAssoc": AREventWirelessClientDaemonReassoc,
		},
		WlPattern+`:\s*(?P<event_type>Assoc|Auth|ReAssoc)\s+`+
			`(?P<client_mac>`+MacPattern+`),`+
			`\s*status:\s*(?P<status_text>.+?)\s*\((?P<status>-?\d+)\),`+
			`\s*rssi:\s*(?P<rssi>-?\d+)`,
		PatternOptions{
			Convert: merge(wirelessClientDaemonStation, map[log.AREventKey]Converter{log.AREventKeyStatus: Int}),
			Marker:  "status:",
		},
	),
	MustARLogPattern(
		map[string]StrEnum{
			"Deauth_ind": AREventWirelessClientDaemonDeauth,
			"Disassoc":   AREventWirelessClientDaemonDisassoc,
		},
		WlPattern+`:\s*(?P<event_type>Deauth_ind|Disassoc)\s+`+
			`(?P<client_mac>`+MacPattern+`),`+
			`\s*status:\s*(?P<status>-?\d+),`+
			`\s*reason:\s*(?P<reason_text>.+?)\s*\((?P<reason>[0-9A-Fa-f]+)\),`+
			`\s*rssi:\s*(?P<rssi>-?\d+)`,
		// The driver prints the reason code in hex
		PatternOptions{
			Convert: merge(wirelessClientDaemonStation, map[log.AREventKey]Converter{
				log.AREventKeyReason: IntBase16,
				log.AREventKeyStatus: Int,
			}),
			Marker: "reason:",
		},
	),
	MustARLogPattern(
		AREventWirelessClientDaemonStart,
		`wlceventd Start`,
		PatternOptions{Marker: "wlceventd Start"},
	),
)
