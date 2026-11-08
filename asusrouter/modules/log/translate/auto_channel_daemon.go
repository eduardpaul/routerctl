package translate

// Auto channel selection daemon log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// The chanspec the daemon settled on, as raw hex then `<channel>[/<width>]`
// TODO: a 40 MHz spec names its sideband instead of a width (`8l`, `6u`
// against `36/160`), so those events report no bandwidth - add it
const autoChannelChanspec = `channel spec: \S+ \((?P<channel>\d+)(?:/(?P<bandwidth>\d+))?`

// autoChannelBandwidth reads a channel width, dropping one the enum does
// not know.
func autoChannelBandwidth(value string) any {
	bandwidth := wifi.ARWiFiBandwidthFromValue(value)

	if bandwidth == wifi.ARWiFiBandwidthUnknown {
		return nil
	}
	return bandwidth
}

// autoChannelFrequency reads a frequency band, dropping one the enum
// does not know.
func autoChannelFrequency(value string) any {
	frequency := wifi.ARWiFiFrequencyFromValue(value)

	if frequency == wifi.ARWiFiFrequencyUnknown {
		return nil
	}
	return frequency
}

var autoChannelConvert = map[log.AREventKey]Converter{
	log.AREventKeyBandwidth: Plain(autoChannelBandwidth),
	log.AREventKeyChannel:   Int,
	log.AREventKeyWlId:      FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
}

// AutoChannelDaemonPatterns mirrors `auto_channel_daemon.PATTERNS`.
var AutoChannelDaemonPatterns = NewARLogPatternSet(
	AREventAutoChannelDaemonUnknown,
	MustARLogPattern(
		AREventAutoChannelDaemonChannelAdjusted,
		RadioPattern+`: Adjusted `+autoChannelChanspec,
		PatternOptions{Convert: autoChannelConvert, Marker: "Adjusted channel spec"},
	),
	MustARLogPattern(
		AREventAutoChannelDaemonChannelSelected,
		RadioPattern+`: selected `+autoChannelChanspec,
		PatternOptions{Convert: autoChannelConvert, Marker: "selected channel spec"},
	),
	MustARLogPattern(
		AREventAutoChannelDaemonChannelSwitched,
		RadioPattern+`: NONACSD channel switching to `+autoChannelChanspec,
		PatternOptions{Convert: autoChannelConvert, Marker: "channel switching to"},
	),
	MustARLogPattern(
		AREventAutoChannelDaemonPolicySelected,
		RadioPattern+`: Selecting (?P<frequency>\d+g\d*) band`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyFrequency: Plain(autoChannelFrequency),
				log.AREventKeyWlId:      FromValueSafe(identifiers.WiFiInterfaceFromValueSafe),
			},
			Marker: "band ACS policy",
		},
	),
)
