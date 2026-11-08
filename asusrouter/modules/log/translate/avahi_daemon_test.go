package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateAvahiDaemon(t *testing.T) {
	nss := "WARNING: No NSS support for mDNS detected, consider installing nss-mdns!"
	runCases(t, AvahiDaemonPatterns, []tcase{
		{name: "alias_established", content: `Alias name "RT-FAKE1" successfully established.`, want: ev{
			log.AREventKeyEventType: AREventAvahiDaemonAliasEstablished,
			log.AREventKeyRaw:       `Alias name "RT-FAKE1" successfully established.`,
			log.AREventKeyAlias:     "RT-FAKE1",
		}},
		{name: "nss_unsupported", content: nss, want: ev{
			log.AREventKeyEventType: AREventAvahiDaemonNssUnsupported,
			log.AREventKeyRaw:       nss,
		}},
		{name: "unrecognized", content: "some future avahi message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventAvahiDaemonUnknown,
			log.AREventKeyRaw:       unread("some future avahi message we do not handle yet"),
		}},
	})
}
