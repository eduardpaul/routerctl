package translate

// Roaming assistant log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// A client seen on a monitored radio
var roamingAssistantClient = map[log.AREventKey]Converter{
	log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
	log.AREventKeyWlId:      FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
}

// RoamingAssistantPatterns mirrors `roaming_assistant.PATTERNS`.
var RoamingAssistantPatterns = NewARLogPatternSet(
	AREventRoamingAssistantUnknown,
	MustARLogPattern(
		AREventRoamingAssistantCandidateSearch,
		`determine candidate node \[(?P<node_mac>`+MacPattern+`)\]`+
			`\s*\(rssi:\s*(?P<node_rssi>-?\d+)\s*dbm\)`+
			`\s*for client \[(?P<client_mac>`+MacPattern+`)\]`+
			`\s*\(rssi:\s*(?P<client_rssi>-?\d+)\s*dbm`+
			`(?:\s*from client\)\s*\(rssi:\s*(?P<ap_rssi>-?\d+)\s*dbm`+
			`\s*from ap)?\)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyApRssi:     Int,
				log.AREventKeyClientMac:  FromValueSafe(identifiers.MacAddressFromValueSafe),
				log.AREventKeyClientRssi: Int,
				log.AREventKeyNodeMac:    FromValueSafe(identifiers.MacAddressFromValueSafe),
				log.AREventKeyNodeRssi:   Int,
			},
			Marker: "determine candidate node",
		},
	),
	MustARLogPattern(
		AREventRoamingAssistantClientRoaming,
		`Roam a client \[(?P<client_mac>`+MacPattern+`)\],`+
			`\s*status \[(?P<status>-?\d+)\]`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
				log.AREventKeyStatus:    Int,
			},
			Marker: "Roam a client",
		},
	),
	// Older builds print the radio as a bare `<unit> <subunit>` pair
	MustARLogPattern(
		AREventRoamingAssistantDeauthOldSta,
		`Deauth old sta in (?P<wl_id>wl[\d.]+|\d+ \d+):`+
			`\s*(?P<client_mac>`+MacPattern+`)`+
			`(?:,\s*because sta is already connected to `+
			`(?P<node_mac>`+MacPattern+`))?`,
		PatternOptions{
			Convert: merge(roamingAssistantClient, map[log.AREventKey]Converter{
				log.AREventKeyNodeMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
			}),
			Marker: "Deauth old sta in",
		},
	),
	MustARLogPattern(
		AREventRoamingAssistantDisconnectWeakSignal,
		WlPattern+`:\s*disconnect weak signal strength station `+
			`\[(?P<client_mac>`+MacPattern+`)\]`,
		PatternOptions{Convert: roamingAssistantClient, Marker: "disconnect weak signal strength station"},
	),
	MustARLogPattern(
		AREventRoamingAssistantRemoveClient,
		WlPattern+`:\s*remove client `+
			`\[(?P<client_mac>`+MacPattern+`)\] from monitor list`,
		PatternOptions{Convert: roamingAssistantClient, Marker: "from monitor list"},
	),
	MustARLogPattern(
		AREventRoamingAssistantRoamingStart,
		`ROAMING Start`,
		PatternOptions{Marker: "ROAMING Start"},
	),
	MustARLogPattern(
		AREventRoamingAssistantStaApBandBindDeauth,
		WlPattern+`:\s*sta-ap-band-bind deauth `+
			`\[(?P<client_mac>`+MacPattern+`)\]`,
		PatternOptions{Convert: roamingAssistantClient, Marker: "sta-ap-band-bind deauth"},
	),
)
