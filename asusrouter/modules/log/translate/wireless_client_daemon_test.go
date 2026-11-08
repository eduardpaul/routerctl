package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const (
	wlcClient = "AA:BB:CC:00:00:01"
	wlcPrefix = "wlceventd_proc_event(722):"
	wlcAuth   = wlcPrefix + " wl0.1: Auth " + wlcClient + ", status: Successful (0), rssi:0"
	wlcDeauth = wlcPrefix + " wl0.1: Deauth_ind " + wlcClient + ", status: 0, " +
		"reason: Deauthenticated because sending station is leaving " +
		"(or has left) IBSS or ESS (3), rssi:-55"
)

func TestConnect(t *testing.T) {
	assoc := wlcPrefix + " wl2.1: Assoc " + wlcClient + ", status: Successful (0), rssi:-42"
	reassoc := wlcPrefix + " wl0.1: ReAssoc " + wlcClient + ", status: Successful (0), rssi:-47"
	runCases(t, WirelessClientDaemonPatterns, []tcase{
		{name: "assoc", content: assoc, want: ev{
			log.AREventKeyEventType:  AREventWirelessClientDaemonAssoc,
			log.AREventKeyRaw:        classified(assoc, IDENTIFIER),
			log.AREventKeyWlId:       wl(2, 1),
			log.AREventKeyClientMac:  mac(t, wlcClient),
			log.AREventKeyStatus:     0,
			log.AREventKeyStatusText: "Successful",
			log.AREventKeyRssi:       -42,
		}},
		{name: "auth", content: wlcAuth, want: ev{
			log.AREventKeyEventType:  AREventWirelessClientDaemonAuth,
			log.AREventKeyRaw:        classified(wlcAuth, IDENTIFIER),
			log.AREventKeyWlId:       wl(0, 1),
			log.AREventKeyClientMac:  mac(t, wlcClient),
			log.AREventKeyStatus:     0,
			log.AREventKeyStatusText: "Successful",
			log.AREventKeyRssi:       0,
		}},
		{name: "reassoc", content: reassoc, check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyEventType, AREventWirelessClientDaemonReassoc)
		}},
	})
}

func TestDisconnect(t *testing.T) {
	disassoc := wlcPrefix + " wl2.1: Disassoc " + wlcClient + ", status: 0, " +
		"reason: Unspecified reason (1), rssi:0"
	runCases(t, WirelessClientDaemonPatterns, []tcase{
		{name: "deauth", content: wlcDeauth, want: ev{
			log.AREventKeyEventType: AREventWirelessClientDaemonDeauth,
			log.AREventKeyRaw:       classified(wlcDeauth, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 1),
			log.AREventKeyClientMac: mac(t, wlcClient),
			log.AREventKeyStatus:    0,
			log.AREventKeyReason:    3,
			log.AREventKeyReasonText: "Deauthenticated because sending station is leaving " +
				"(or has left) IBSS or ESS",
			log.AREventKeyRssi: -55,
		}},
		{name: "disassoc", content: disassoc, want: ev{
			log.AREventKeyEventType:  AREventWirelessClientDaemonDisassoc,
			log.AREventKeyRaw:        classified(disassoc, IDENTIFIER),
			log.AREventKeyWlId:       wl(2, 1),
			log.AREventKeyClientMac:  mac(t, wlcClient),
			log.AREventKeyStatus:     0,
			log.AREventKeyReason:     1,
			log.AREventKeyReasonText: "Unspecified reason",
			log.AREventKeyRssi:       0,
		}},
		{name: "reason_hex_code",
			content: wlcPrefix + " wl0.1: Deauth_ind " + wlcClient + ", status: 0, reason: 4-way handshake timeout (f), rssi:-46",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyReason, 15)
				eq(t, e, log.AREventKeyReasonText, "4-way handshake timeout")
			}},
		{name: "reason_unmapped",
			content: wlcPrefix + " wl0.1: Deauth_ind " + wlcClient + ", status: 0, reason: UNKNOWN (7b7c), rssi:-68",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyReason, 0x7B7C)
				eq(t, e, log.AREventKeyReasonText, "UNKNOWN")
			}},
	})
}

func TestTranslateWirelessClientDaemon(t *testing.T) {
	unknown := "some future wlceventd message we do not handle yet"
	runCases(t, WirelessClientDaemonPatterns, []tcase{
		{name: "start", content: "main(1239): wlceventd Start...", want: ev{
			log.AREventKeyEventType: AREventWirelessClientDaemonStart,
			log.AREventKeyRaw:       "main(1239): wlceventd Start...",
		}},
		{name: "unrecognized", content: unknown, want: ev{
			log.AREventKeyEventType: AREventWirelessClientDaemonUnknown,
			log.AREventKeyRaw:       unread(unknown),
		}},
	})
}
