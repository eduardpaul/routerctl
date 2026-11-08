package translate

// MiniUPnP daemon log events for AsusRouter.

import (
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Every message naming a peer reads its address the same way
var miniupnpPeer = map[log.AREventKey]Converter{
	log.AREventKeyClientIp: FromValueSafe(identifiers.IpAddressFromValueSafe),
	log.AREventKeyPort:     Int,
}

// MiniupnpPatterns mirrors `miniupnp.PATTERNS`.
var MiniupnpPatterns = NewARLogPatternSet(
	AREventMiniupnpUnknown,
	// A missing iptables chain
	MustARLogPattern(
		AREventMiniupnpChainNotFound,
		`chain (?P<chain>\S+) not found`,
		PatternOptions{Marker: "not found"},
	),
	// The failure reason trails the peer and is left in the raw
	MustARLogPattern(
		AREventMiniupnpEventNotifyFailed,
		`upnp_event_process_notify: `+
			`connect\((?P<client_ip>[\d.]+):(?P<port>\d+)\)`,
		PatternOptions{Convert: miniupnpPeer, Marker: "upnp_event_process_notify"},
	),
	MustARLogPattern(
		AREventMiniupnpEventRecvTimeout,
		`upnp_event_recv: recv\(\): Connection timed out`,
		PatternOptions{Marker: "upnp_event_recv"},
	),
	MustARLogPattern(
		AREventMiniupnpHttpListening,
		`HTTP listening on port (?P<port>\d+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Marker:  "HTTP listening on port",
		},
	),
	MustARLogPattern(
		AREventMiniupnpInterfaceIpFailed,
		`Failed to get ip address for interface`,
		PatternOptions{Marker: "Failed to get ip address"},
	),
	// The daemon was configured with an address where it wants a name
	MustARLogPattern(
		AREventMiniupnpInterfaceNameAdvised,
		`use network interface name instead of `+
			`(?P<local_ip>[\d.]+)/(?P<netmask>[\d.]+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyLocalIp: FromValueSafe(identifiers.IpAddressFromValueSafe),
				log.AREventKeyNetmask: FromValueSafe(identifiers.IpAddressFromValueSafe),
			},
			Marker: "use network interface name instead",
		},
	),
	// Same failure, worded differently on older firmware
	MustARLogPattern(
		AREventMiniupnpInterfaceIpFailed,
		`Cannot get IP address for ext interface`,
		PatternOptions{Marker: "Cannot get IP address"},
	),
	MustARLogPattern(
		AREventMiniupnpNatPmpListening,
		`Listening for NAT-PMP/PCP traffic on port (?P<port>\d+)`,
		PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Marker:  "NAT-PMP/PCP traffic",
		},
	),
	MustARLogPattern(
		AREventMiniupnpPeerNotLan,
		`HTTP peer (?P<client_ip>[\d.]+):(?P<port>\d+) is not from a LAN`,
		PatternOptions{Convert: miniupnpPeer, Marker: "is not from a LAN"},
	),
	MustARLogPattern(
		AREventMiniupnpPublicAddressFailed,
		`SendNATPMPPublicAddressChangeNotification: `+
			`cannot get public IP address`,
		PatternOptions{Marker: "cannot get public IP address"},
	),
	MustARLogPattern(
		AREventMiniupnpShutdown,
		`shutting down MiniUPnPd`,
		PatternOptions{Marker: "shutting down MiniUPnPd"},
	),
	// The callback carries the LAN client `http://<ip>:<port><path>`
	MustARLogPattern(
		AREventMiniupnpSubscriberRemoved,
		`remove subscriber uuid:(?P<uuid>[0-9A-Fa-f-]+) `+
			`after an ERROR cb: `+
			`\w+://(?P<client_ip>[\d.]+):(?P<port>\d+)(?P<path>\S+)`,
		PatternOptions{Convert: miniupnpPeer, Marker: "remove subscriber"},
	),
)
