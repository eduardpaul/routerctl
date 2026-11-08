package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateNtp(t *testing.T) {
	runCases(t, NtpPatterns, []tcase{
		{name: "start_update", content: "start NTP update", want: ev{
			log.AREventKeyEventType: AREventNtpStartUpdate,
			log.AREventKeyRaw:       "start NTP update",
		}},
		{name: "unrecognized", content: "some future ntp message", want: ev{
			log.AREventKeyEventType: AREventNtpUnknown,
			log.AREventKeyRaw:       unread("some future ntp message"),
		}},
	})
}
