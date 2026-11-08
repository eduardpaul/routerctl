package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateModprobe(t *testing.T) {
	runCases(t, ModprobePatterns, []tcase{
		{name: "module_not_found", content: "module scsi_wait_scan not found in modules.dep", want: ev{
			log.AREventKeyEventType: AREventModprobeModuleNotFound,
			log.AREventKeyRaw:       "module scsi_wait_scan not found in modules.dep",
			log.AREventKeyModule:    "scsi_wait_scan",
		}},
		{name: "dashed_module", content: "module ledtrig-usbdev not found in modules.dep", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyModule, "ledtrig-usbdev")
		}},
		{name: "unrecognized", content: "some future modprobe message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventModprobeUnknown,
			log.AREventKeyRaw:       unread("some future modprobe message we do not handle yet"),
		}},
	})
}
