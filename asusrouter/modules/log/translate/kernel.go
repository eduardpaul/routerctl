package translate

// Kernel log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Common ethernet switch-port prefix shared by the link up/down messages
const kernelEthPort = `(?P<interface>eth\d+)\s+\(Int switch port:\s*(?P<switch_port>\d+)\)\s+` +
	`\(Logical Port:\s*(?P<logical_port>\d+)\)\s+` +
	`\(phyId:\s*(?P<phy_id>[0-9A-Fa-f]+)\)`

var kernelEthPortFields = map[log.AREventKey]Converter{
	log.AREventKeyLogicalPort: Int,
	log.AREventKeySwitchPort:  Int,
}

// kernelEntered reports whether the interface entered rather than left
// the mode.
func kernelEntered(value string) any { return value == "entered" }

// kernelOn reports whether the driver reports the feature as on.
func kernelOn(value string) any { return value == "On" }

// kernelOne reports whether the driver flag is set.
func kernelOne(value string) any { return value == "1" }

// KernelPatterns mirrors `kernel.PATTERNS`.
var KernelPatterns = NewARLogPatternSet(
	AREventKernelUnknown,
	MustARLogPattern(
		AREventKernelBridgePortState,
		`(?P<bridge>br\d+): port (?P<port>\d+)`+
			`\((?P<interface>[a-z0-9.]+)\) entering (?P<state>\w+) state`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Marker:  "entering",
		},
	),
	MustARLogPattern(
		AREventKernelBridgeTopologyChange,
		`(?P<bridge>br\d+): topology change detected`,
		PatternOptions{Marker: "topology change detected"},
	),
	MustARLogPattern(
		AREventKernelEthernetLinkDown,
		kernelEthPort+`\s+Link (?:DOWN|Down)`,
		PatternOptions{Convert: kernelEthPortFields, Marker: "Link"},
	),
	MustARLogPattern(
		AREventKernelEthernetLinkUp,
		kernelEthPort+`\s+Link (?:UP|Up) at (?P<speed>\d+) mbps `+
			`(?P<duplex>\w+) duplex(?:\s+AN:\s*(?P<autoneg>\w+))?`,
		PatternOptions{
			Convert: merge(kernelEthPortFields, map[log.AREventKey]Converter{
				log.AREventKeyAutoneg: Plain(kernelOn),
				log.AREventKeySpeed:   Int,
			}),
			Marker: "Link",
		},
	),
	MustARLogPattern(
		AREventKernelKlogdStart,
		`klogd started: BusyBox v(?P<version>[\d.]+)`,
		PatternOptions{Marker: "klogd started"},
	),
	MustARLogPattern(
		AREventKernelPhyCalInit,
		`wlc_phy_cal_init_acphy: NOT Implemented`,
		PatternOptions{Marker: "wlc_phy_cal_init_acphy"},
	),
	MustARLogPattern(
		AREventKernelPromiscuousMode,
		`device (?P<interface>[a-z0-9.]+) `+
			`(?P<enabled>entered|left) promiscuous mode`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyEnabled: Plain(kernelEntered)},
			Marker:  "promiscuous mode",
		},
	),
	// SBF unknown; only the client MAC is interpreted
	MustARLogPattern(
		AREventKernelSbfInit,
		`SBF:\s*\w+:\s*INIT\s*\[(?P<client_mac>`+MacPattern+`)\]`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
			},
			Marker: "SBF:",
		},
	),
	// Both driver ioctls fail with -30 (BCME_NOTFOUND) once the
	// station is gone
	MustARLogPattern(
		AREventKernelScbDeauthError,
		`(?P<command>WLC_SCB_[A-Z_]+)\s+`+
			`(?:error\s*\(|err\s+)(?P<error>-?\d+)\)?`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyError: Int},
			Marker:  "WLC_SCB_",
		},
	),
	// `<iface>: dev_set_promiscuity(master, 1)` on the bridge
	MustARLogPattern(
		AREventKernelPromiscuousMode,
		`(?P<interface>[a-z0-9.]+): `+
			`dev_set_promiscuity\(master, (?P<enabled>[01])\)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyEnabled: Plain(kernelOne)},
			Marker:  "dev_set_promiscuity",
		},
	),
	MustARLogPattern(
		AREventKernelTaskletKillAttempt,
		`Attempt to kill tasklet from interrupt`,
		PatternOptions{Marker: "Attempt to kill tasklet"},
	),
	MustARLogPattern(
		AREventKernelWirelessController,
		`(?P<interface>eth\d+): Broadcom (?P<chip>BCM\d+) `+
			`802\.11 Wireless Controller (?P<version>[\d.]+) `+
			`\(r(?P<revision>\d+)\)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyRevision: Int},
			Marker:  "Wireless Controller",
		},
	),
	MustARLogPattern(
		AREventKernelWlModuleInit,
		`wl_module_init: (?P<parameter>\w+) set to `+
			`(?P<value>0x[0-9A-Fa-f]+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyValue: IntBase16},
			Marker:  "wl_module_init:",
		},
	),
)
