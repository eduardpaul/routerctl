package translate

// Band steering daemon log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var bandSteeringClient = map[log.AREventKey]Converter{
	log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
}
var bandSteeringRadio = map[log.AREventKey]Converter{
	log.AREventKeyWlId: FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
}

// merge mirrors `{**a, **b, key: value}` for converter maps.
func merge(maps ...map[log.AREventKey]Converter) map[log.AREventKey]Converter {
	out := map[log.AREventKey]Converter{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// BandSteeringDaemonPatterns mirrors `band_steering_daemon.PATTERNS`.
var BandSteeringDaemonPatterns = NewARLogPatternSet(
	AREventBandSteeringDaemonUnknown,
	// The steering request itself; the trailing `ssid <mac>` is left
	// in the raw, the firmware label does not match its value
	MustARLogPattern(
		AREventBandSteeringDaemonActFrameSent,
		`(?:`+WlPattern+`\s+)?Sending act Frame to `+
			`(?P<client_mac>`+MacPattern+`) `+
			`with transition target (?P<target>wl[\d.]+)`,
		PatternOptions{
			Convert: merge(
				bandSteeringClient,
				bandSteeringRadio,
				map[log.AREventKey]Converter{
					log.AREventKeyTarget: FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
				},
			),
			Marker: "Sending act Frame",
		},
	),
	MustARLogPattern(
		AREventBandSteeringDaemonStaNoResponse,
		`STA:(?P<client_mac>`+MacPattern+`) no response`,
		PatternOptions{Convert: bandSteeringClient, Marker: "no response"},
	),
	MustARLogPattern(
		AREventBandSteeringDaemonStaSkipped,
		`Skip STA:(?P<client_mac>`+MacPattern+`)`,
		PatternOptions{Convert: bandSteeringClient, Marker: "Skip STA:"},
	),
	// `event` and `token` carry no confirmed meaning, so the detailed
	// form keeps only the fields that do
	MustARLogPattern(
		AREventBandSteeringDaemonTransitResponse,
		`BSS Transit Response: ifname=`+WlPattern+`.*?`+
			`status=(?P<status>\d+), `+
			`mac=(?P<client_mac>`+MacPattern+`)`,
		PatternOptions{
			Convert: merge(
				bandSteeringClient,
				bandSteeringRadio,
				map[log.AREventKey]Converter{log.AREventKeyStatus: Int},
			),
			Marker: "BSS Transit Response",
		},
	),
	MustARLogPattern(
		AREventBandSteeringDaemonTransitResponse,
		`BSS Transit Response: STA (?P<status_text>\w+)`,
		PatternOptions{Marker: "BSS Transit Response"},
	),
	// The token carries no confirmed meaning, so it stays in the raw
	MustARLogPattern(
		AREventBandSteeringDaemonTransitResponse,
		`BSS Transit Response: not for token`,
		PatternOptions{Marker: "BSS Transit Response"},
	),
	MustARLogPattern(
		AREventBandSteeringDaemonTransitResponse,
		`BSS Transit Response: not for interface `+WlPattern,
		PatternOptions{Convert: bandSteeringRadio, Marker: "BSS Transit Response"},
	),
)
