package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const (
	bsdClient = "aa:bb:cc:00:00:01"
	bsdBssid  = "aa:bb:cc:00:00:02"
)

func TestActFrameSent(t *testing.T) {
	full := "bsd: wl0.1 Sending act Frame to " + bsdClient + " with transition target wl1.1 ssid " + bsdBssid
	noSource := "bsd: Sending act Frame to " + bsdClient + " with transition target wl1.1 ssid " + bsdBssid
	runCases(t, BandSteeringDaemonPatterns, []tcase{
		{name: "parses_radios_and_client", content: full, want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonActFrameSent,
			log.AREventKeyRaw:       classified(full, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 1),
			log.AREventKeyClientMac: mac(t, bsdClient),
			log.AREventKeyTarget:    wl(1, 1),
		}},
		{name: "source_radio_optional", content: noSource, check: func(t *testing.T, e ev) {
			absent(t, e, log.AREventKeyWlId)
			eq(t, e, log.AREventKeyTarget, wl(1, 1))
		}},
	})
}

func TestTransitResponse(t *testing.T) {
	detailed := "bsd: BSS Transit Response: ifname=wl1.1, event=156, token=e0, status=7, mac=" + bsdClient
	runCases(t, BandSteeringDaemonPatterns, []tcase{
		{name: "status_word", content: "bsd: BSS Transit Response: STA reject", want: ev{
			log.AREventKeyEventType:  AREventBandSteeringDaemonTransitResponse,
			log.AREventKeyRaw:        "bsd: BSS Transit Response: STA reject",
			log.AREventKeyStatusText: "reject",
		}},
		{name: "other_interface", content: "bsd: BSS Transit Response: not for interface wl0.1", want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonTransitResponse,
			log.AREventKeyRaw:       "bsd: BSS Transit Response: not for interface wl0.1",
			log.AREventKeyWlId:      wl(0, 1),
		}},
		{name: "detailed_form", content: detailed, want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonTransitResponse,
			log.AREventKeyRaw:       classified(detailed, IDENTIFIER),
			log.AREventKeyWlId:      wl(1, 1),
			log.AREventKeyStatus:    7,
			log.AREventKeyClientMac: mac(t, bsdClient),
		}},
		{name: "token_only_form", content: "bsd: BSS Transit Response: not for token e4", want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonTransitResponse,
			log.AREventKeyRaw:       "bsd: BSS Transit Response: not for token e4",
		}},
	})
}

func TestBandSteeringStation(t *testing.T) {
	noResponse := "bsd: STA:" + bsdClient + " no response"
	skipped := "bsd: Skip STA:" + bsdClient + " reject BSSID"
	runCases(t, BandSteeringDaemonPatterns, []tcase{
		{name: "no_response", content: noResponse, want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonStaNoResponse,
			log.AREventKeyRaw:       classified(noResponse, IDENTIFIER),
			log.AREventKeyClientMac: mac(t, bsdClient),
		}},
		{name: "skipped", content: skipped, want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonStaSkipped,
			log.AREventKeyRaw:       classified(skipped, IDENTIFIER),
			log.AREventKeyClientMac: mac(t, bsdClient),
		}},
	})
}

func TestTranslateBandSteeringDaemon(t *testing.T) {
	content := "bsd: some future bsd message we do not handle yet"
	runCases(t, BandSteeringDaemonPatterns, []tcase{
		{name: "unrecognized", content: content, want: ev{
			log.AREventKeyEventType: AREventBandSteeringDaemonUnknown,
			log.AREventKeyRaw:       unread(content),
		}},
	})
}
