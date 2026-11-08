package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateSyslog(t *testing.T) {
	runCases(t, SyslogPatterns, []tcase{
		{name: "started", content: "syslogd started: BusyBox v1.25.1", want: ev{
			log.AREventKeyEventType: AREventSyslogStarted,
			log.AREventKeyRaw:       "syslogd started: BusyBox v1.25.1",
			log.AREventKeyVersion:   "1.25.1",
		}},
		{name: "unrecognized", content: "some future syslogd message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventSyslogUnknown,
			log.AREventKeyRaw:       unread("some future syslogd message we do not handle yet"),
		}},
	})
}
