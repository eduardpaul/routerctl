package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateDaemon(t *testing.T) {
	lifecycle := []struct {
		content string
		event   AREventDaemon
	}{
		{"daemon is started", AREventDaemonStarted},
		{"daemon is stopped", AREventDaemonStopped},
		{"smb daemon is stopped", AREventDaemonStopped},
		// The firmware misspells it on some daemons
		{"daemon is stoped", AREventDaemonStopped},
	}
	var cases []tcase
	for _, l := range lifecycle {
		cases = append(cases, tcase{name: "lifecycle/" + l.content, content: l.content, want: ev{
			log.AREventKeyEventType: l.event,
			log.AREventKeyRaw:       l.content,
		}})
	}
	cases = append(cases, tcase{name: "unrecognized", content: "some future daemon message we do not handle yet", want: ev{
		log.AREventKeyEventType: AREventDaemonUnknown,
		log.AREventKeyRaw:       unread("some future daemon message we do not handle yet"),
	}})
	runCases(t, DaemonPatterns, cases)
}
