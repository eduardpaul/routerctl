package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateHostapd(t *testing.T) {
	const client = "aa:bb:cc:3f:8d:15"
	addFailed := "wl0.1: STA " + client + " IEEE 802.11: Could not add STA to kernel driver"
	other := "wl0.1: STA aa:bb:cc:3f:8d:15 IEEE 802.11: authenticated"
	runCases(t, HostapdPatterns, []tcase{
		{name: "sta_add_failed", content: addFailed, want: ev{
			log.AREventKeyEventType: AREventHostapdStaAddFailed,
			log.AREventKeyRaw:       classified(addFailed, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 1),
			log.AREventKeyClientMac: mac(t, client),
		}},
		{name: "unrecognized", content: other, want: ev{
			log.AREventKeyEventType: AREventHostapdUnknown,
			log.AREventKeyRaw:       unread(other),
		}},
	})
}
