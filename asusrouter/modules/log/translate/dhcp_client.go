package translate

// DHCP client log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// DhcpClientPatterns mirrors `dhcp_client.PATTERNS`.
var DhcpClientPatterns = NewARLogPatternSet(
	AREventDhcpClientUnknown,
	MustARLogPattern(
		AREventDhcpClientBound,
		`bound (?P<local_ip>[\d.]+)/(?P<netmask>[\d.]+) `+
			`via (?P<gateway>[\d.]+) for (?P<lease_seconds>\d+) seconds`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyGateway:      FromValueSafe(identifiers.IpAddressFromValueSafe),
				log.AREventKeyLeaseSeconds: Int,
				log.AREventKeyLocalIp:      FromValueSafe(identifiers.IpAddressFromValueSafe),
				log.AREventKeyNetmask:      FromValueSafe(identifiers.IpAddressFromValueSafe),
			},
			Marker: "bound ",
		},
	),
	// The interface address was cleared
	MustARLogPattern(
		AREventDhcpClientDeconfig, `^deconfig$`, PatternOptions{Marker: "deconfig"},
	),
	MustARLogPattern(
		AREventDhcpClientLeaseFail, `^leasefail$`, PatternOptions{Marker: "leasefail"},
	),
)
