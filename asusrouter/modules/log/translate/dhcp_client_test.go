package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateDhcpClient(t *testing.T) {
	bound := "bound 192.168.0.2/255.255.255.0 via 192.168.0.1 for 86400 seconds."
	runCases(t, DhcpClientPatterns, []tcase{
		{name: "bound", content: bound, want: ev{
			log.AREventKeyEventType:    AREventDhcpClientBound,
			log.AREventKeyRaw:          classified(bound, IDENTIFIER),
			log.AREventKeyLocalIp:      ip(t, "192.168.0.2"),
			log.AREventKeyNetmask:      ip(t, "255.255.255.0"),
			log.AREventKeyGateway:      ip(t, "192.168.0.1"),
			log.AREventKeyLeaseSeconds: 86400,
		}},
		{name: "deconfig", content: "deconfig", want: ev{
			log.AREventKeyEventType: AREventDhcpClientDeconfig,
			log.AREventKeyRaw:       "deconfig",
		}},
		{name: "lease_fail", content: "leasefail", want: ev{
			log.AREventKeyEventType: AREventDhcpClientLeaseFail,
			log.AREventKeyRaw:       "leasefail",
		}},
		{name: "unrecognized", content: "renew", want: ev{
			log.AREventKeyEventType: AREventDhcpClientUnknown,
			log.AREventKeyRaw:       unread("renew"),
		}},
	})
}
