package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateMiniupnp(t *testing.T) {
	peer := "HTTP peer 10.1.0.2:46167 is not from a LAN, closing the connection"
	subscriber := "upnpevents_processfds: 0x210fe8, remove subscriber " +
		"uuid:3ddcd1d3-2380-45f5-b069-cc28aaf405c3 after an ERROR cb: " +
		"http://192.168.0.2:2869/upnp/eventing/fyumvcmqvt"
	notify := "upnp_event_process_notify: connect(192.168.0.2:2869): Connection timed out"
	legacy := "Cannot get IP address for ext interface . Network is down"
	advised := "it is advised to use network interface name instead of 192.168.1.1/255.255.255.0"
	public := "SendNATPMPPublicAddressChangeNotification: cannot get public IP address, stopping"
	unknown := "some future miniupnpd message we do not handle yet"
	runCases(t, MiniupnpPatterns, []tcase{
		{name: "chain_not_found", content: "chain VUPNP not found", want: ev{
			log.AREventKeyEventType: AREventMiniupnpChainNotFound,
			log.AREventKeyRaw:       "chain VUPNP not found",
			log.AREventKeyChain:     "VUPNP",
		}},
		{name: "event_recv_timeout", content: "upnp_event_recv: recv(): Connection timed out", want: ev{
			log.AREventKeyEventType: AREventMiniupnpEventRecvTimeout,
			log.AREventKeyRaw:       "upnp_event_recv: recv(): Connection timed out",
		}},
		{name: "interface_ip_failed", content: "Failed to get ip address for interface", want: ev{
			log.AREventKeyEventType: AREventMiniupnpInterfaceIpFailed,
			log.AREventKeyRaw:       "Failed to get ip address for interface",
		}},
		{name: "peer_not_lan", content: peer, want: ev{
			log.AREventKeyEventType: AREventMiniupnpPeerNotLan,
			log.AREventKeyRaw:       classified(peer, IDENTIFIER),
			log.AREventKeyClientIp:  ip(t, "10.1.0.2"),
			log.AREventKeyPort:      46167,
		}},
		{name: "subscriber_removed", content: subscriber, want: ev{
			log.AREventKeyEventType: AREventMiniupnpSubscriberRemoved,
			log.AREventKeyRaw:       classified(subscriber, IDENTIFIER),
			log.AREventKeyUuid:      "3ddcd1d3-2380-45f5-b069-cc28aaf405c3",
			log.AREventKeyClientIp:  ip(t, "192.168.0.2"),
			log.AREventKeyPort:      2869,
			log.AREventKeyPath:      "/upnp/eventing/fyumvcmqvt",
		}},
		{name: "event_notify_failed", content: notify, want: ev{
			log.AREventKeyEventType: AREventMiniupnpEventNotifyFailed,
			log.AREventKeyRaw:       classified(notify, IDENTIFIER),
			log.AREventKeyClientIp:  ip(t, "192.168.0.2"),
			log.AREventKeyPort:      2869,
		}},
		{name: "http_listening", content: "HTTP listening on port 39733", want: ev{
			log.AREventKeyEventType: AREventMiniupnpHttpListening,
			log.AREventKeyRaw:       "HTTP listening on port 39733",
			log.AREventKeyPort:      39733,
		}},
		{name: "nat_pmp_listening", content: "Listening for NAT-PMP/PCP traffic on port 5351", want: ev{
			log.AREventKeyEventType: AREventMiniupnpNatPmpListening,
			log.AREventKeyRaw:       "Listening for NAT-PMP/PCP traffic on port 5351",
			log.AREventKeyPort:      5351,
		}},
		{name: "interface_ip_failed_legacy_wording", content: legacy, want: ev{
			log.AREventKeyEventType: AREventMiniupnpInterfaceIpFailed,
			log.AREventKeyRaw:       legacy,
		}},
		{name: "interface_name_advised", content: advised, want: ev{
			log.AREventKeyEventType: AREventMiniupnpInterfaceNameAdvised,
			log.AREventKeyRaw:       classified(advised, IDENTIFIER),
			log.AREventKeyLocalIp:   ip(t, "192.168.1.1"),
			log.AREventKeyNetmask:   ip(t, "255.255.255.0"),
		}},
		{name: "public_address_failed", content: public, want: ev{
			log.AREventKeyEventType: AREventMiniupnpPublicAddressFailed,
			log.AREventKeyRaw:       public,
		}},
		{name: "shutdown", content: "shutting down MiniUPnPd", want: ev{
			log.AREventKeyEventType: AREventMiniupnpShutdown,
			log.AREventKeyRaw:       "shutting down MiniUPnPd",
		}},
		{name: "unrecognized", content: unknown, want: ev{
			log.AREventKeyEventType: AREventMiniupnpUnknown,
			log.AREventKeyRaw:       unread(unknown),
		}},
	})
}
