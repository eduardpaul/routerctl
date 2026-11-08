package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestTranslateDnsmasqDhcp(t *testing.T) {
	const addr = "192.168.1.11"
	const client = "aa:bb:cc:00:00:24"
	request := "DHCPREQUEST(br0) " + addr + " " + client
	ack := "DHCPACK(br0) " + addr + " " + client + " my-device"
	rangeLine := "DHCP range 192.168.1.2 -- 192.168.1.254"
	runCases(t, DnsmasqDhcpPatterns, []tcase{
		{name: "request", content: request, want: ev{
			log.AREventKeyEventType: AREventDnsmasqDhcpRequest,
			log.AREventKeyRaw:       classified(request, IDENTIFIER),
			log.AREventKeyInterface: "br0",
			log.AREventKeyClientIp:  ip(t, addr),
			log.AREventKeyClientMac: mac(t, client),
		}},
		// Python compares the Hostname with its str; Go with the Hostname
		{name: "ack_with_hostname", content: ack, want: ev{
			log.AREventKeyEventType: AREventDnsmasqDhcpAck,
			log.AREventKeyRaw:       classified(ack, IDENTIFIER),
			log.AREventKeyInterface: "br0",
			log.AREventKeyClientIp:  ip(t, addr),
			log.AREventKeyClientMac: mac(t, client),
			log.AREventKeyHostname:  identifiers.NewHostname("my-device"),
		}},
		{name: "unknown_verb_keeps_data", content: "DHCPRELEASE(br0) " + addr + " " + client, check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyEventType, AREventDnsmasqDhcpUnknown)
			eq(t, e, log.AREventKeyClientIp, ip(t, addr))
			absent(t, e, log.AREventKeyHostname)
		}},
		{name: "unrecognized", content: rangeLine, want: ev{
			log.AREventKeyEventType: AREventDnsmasqDhcpUnknown,
			log.AREventKeyRaw:       unread(rangeLine),
		}},
	})
}
