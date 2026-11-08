package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateAhs(t *testing.T) {
	runCases(t, AhsPatterns, []tcase{
		{name: "json_update", content: "[read_json]Update ahs JSON file.", want: ev{
			log.AREventKeyEventType: AREventAhsJsonUpdate,
			log.AREventKeyRaw:       "[read_json]Update ahs JSON file.",
		}},
		{name: "terminate", content: "===Terminate ahs daemon===", want: ev{
			log.AREventKeyEventType: AREventAhsTerminate,
			log.AREventKeyRaw:       "===Terminate ahs daemon===",
		}},
		{name: "unrecognized", content: "some future ahs message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventAhsUnknown,
			log.AREventKeyRaw:       unread("some future ahs message we do not handle yet"),
		}},
	})
}
