package translate

// DHCP server log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// DnsmasqDhcpPatterns mirrors `dnsmasq_dhcp.PATTERNS`.
var DnsmasqDhcpPatterns = NewARLogPatternSet(
	AREventDnsmasqDhcpUnknown,
	// `DHCP<VERB>(<iface>) <ip> <mac> [hostname]`; only ACK is named
	MustARLogPattern(
		map[string]StrEnum{
			"DHCPACK":     AREventDnsmasqDhcpAck,
			"DHCPREQUEST": AREventDnsmasqDhcpRequest,
		},
		`(?P<event_type>DHCP[A-Z]+)\((?P<interface>[^)]+)\)\s+`+
			`(?P<client_ip>[\d.]+)\s+`+
			`(?P<client_mac>`+MacPattern+`)(?:\s+(?P<hostname>\S+))?`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientIp:  FromValueSafe(identifiers.IpAddressFromValueSafe),
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
				// The name the client asked for - a plain str here would
				// leave it out of the raw message's classification
				log.AREventKeyHostname: FromValueSafe(identifiers.HostnameFromValueSafe),
			},
			Marker: "DHCP",
		},
	),
)
