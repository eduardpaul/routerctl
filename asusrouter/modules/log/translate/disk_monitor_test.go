package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateDiskMonitor(t *testing.T) {
	runCases(t, DiskMonitorPatterns, []tcase{
		{name: "finish", content: "Finish", want: ev{
			log.AREventKeyEventType: AREventDiskMonitorFinish,
			log.AREventKeyRaw:       "Finish",
		}},
		{name: "sigalrm", content: "Got SIGALRM...", want: ev{
			log.AREventKeyEventType: AREventDiskMonitorSigalrm,
			log.AREventKeyRaw:       "Got SIGALRM...",
		}},
		{name: "unrecognized", content: "some future disk_monitor message", want: ev{
			log.AREventKeyEventType: AREventDiskMonitorUnknown,
			log.AREventKeyRaw:       unread("some future disk_monitor message"),
		}},
	})
}
