package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestTranslateInit(t *testing.T) {
	banner := "fwver: 3.0.0.6_102_34735 (sn:R2FAKE000000ABC /ha:C8:7F:54:00:00:01 )"
	runCases(t, InitPatterns, []tcase{
		// The banner names this device, so both values are typed
		{name: "firmware_banner", content: banner, want: ev{
			log.AREventKeyEventType:    AREventInitFirmwareBanner,
			log.AREventKeyRaw:          classified(banner, IDENTIFIER),
			log.AREventKeyVersion:      "3.0.0.6_102_34735",
			log.AREventKeyDeviceSerial: identifiers.NewSerial("R2FAKE000000ABC"),
			log.AREventKeyDeviceMac:    mac(t, "C8:7F:54:00:00:01"),
		}},
		{name: "banner_without_hardware", content: "fwver: 3.0.0.4_388_24333-ga030aaf", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyVersion, "3.0.0.4_388_24333-ga030aaf")
			absent(t, e, log.AREventKeyDeviceSerial)
			absent(t, e, log.AREventKeyDeviceMac)
		}},
		{name: "unrecognized", content: "some future init message we do not handle yet", want: ev{
			log.AREventKeyEventType: AREventInitUnknown,
			log.AREventKeyRaw:       unread("some future init message we do not handle yet"),
		}},
	})
}
