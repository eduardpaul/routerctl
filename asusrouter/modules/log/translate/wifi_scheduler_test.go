package translate

import (
	"fmt"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

func TestTranslateWifiScheduler(t *testing.T) {
	on := "Turn radio [band_index=0, subunit=2] on."
	off := "Turn radio [band_index=1, subunit=4] off."
	unknown := "some future scheduler message we do not handle yet"
	cases := []tcase{
		{name: "radio_on", content: on, want: ev{
			log.AREventKeyEventType: AREventWifiSchedulerRadioOn,
			log.AREventKeyRaw:       on,
			log.AREventKeyWlId:      wl(0, 2),
		}},
		{name: "radio_off", content: off, want: ev{
			log.AREventKeyEventType: AREventWifiSchedulerRadioOff,
			log.AREventKeyRaw:       off,
			log.AREventKeyWlId:      wl(1, 4),
		}},
	}
	for _, r := range [][2]int{{0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 3}, {1, 4}} {
		r := r
		cases = append(cases, tcase{
			name:    fmt.Sprintf("every_observed_radio/%d-%d", r[0], r[1]),
			content: fmt.Sprintf("Turn radio [band_index=%d, subunit=%d] on.", r[0], r[1]),
			check:   func(t *testing.T, e ev) { eq(t, e, log.AREventKeyWlId, wl(r[0], r[1])) },
		})
	}
	cases = append(cases, tcase{name: "unrecognized", content: unknown, want: ev{
		log.AREventKeyEventType: AREventWifiSchedulerUnknown,
		log.AREventKeyRaw:       unread(unknown),
	}})
	runCases(t, WifiSchedulerPatterns, cases)
}
