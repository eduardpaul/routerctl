package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateCron(t *testing.T) {
	runCases(t, CronPatterns, []tcase{
		{name: "time_disparity", content: "time disparity of 1191748 minutes detected", want: ev{
			log.AREventKeyEventType: AREventCronTimeDisparity,
			log.AREventKeyRaw:       "time disparity of 1191748 minutes detected",
			log.AREventKeyMinutes:   1191748,
		}},
		{name: "negative_disparity", content: "time disparity of -42 minutes detected", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyMinutes, -42)
		}},
		{name: "unrecognized", content: "some future crond message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventCronUnknown,
			log.AREventKeyRaw:       unread("some future crond message we do not handle yet"),
		}},
	})
}
