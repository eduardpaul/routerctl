package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const (
	raNode      = "AA:BB:CC:00:00:01"
	raClient    = "AA:BB:CC:00:00:02"
	raCandidate = "determine candidate node [" + raNode + "](rssi: -46dbm) " +
		"for client [" + raClient + "](rssi: -57dbm from client)" +
		"(rssi: -71dbm from ap) to roam"
	raRoam    = "Roam a client [" + raClient + "], status [0]"
	raWeak    = "wl0.2: disconnect weak signal strength station [" + raClient + "]"
	raRemove  = "wl0.2: remove client [" + raClient + "] from monitor list"
	raAlready = "[EXAP]Deauth old sta in wl0.2: " + raClient + ", " +
		"because sta is already connected to " + raNode + "."
)

func TestCandidateSearch(t *testing.T) {
	short := "determine candidate node [" + raNode + "](rssi: -10dbm) " +
		"for client [" + raClient + "](rssi: -32dbm) to roam"
	runCases(t, RoamingAssistantPatterns, []tcase{
		{name: "parses_fields", content: raCandidate, want: ev{
			log.AREventKeyEventType:  AREventRoamingAssistantCandidateSearch,
			log.AREventKeyRaw:        classified(raCandidate, IDENTIFIER),
			log.AREventKeyNodeMac:    mac(t, raNode),
			log.AREventKeyNodeRssi:   -46,
			log.AREventKeyClientMac:  mac(t, raClient),
			log.AREventKeyClientRssi: -57,
			log.AREventKeyApRssi:     -71,
		}},
		{name: "short_variant", content: short, want: ev{
			log.AREventKeyEventType:  AREventRoamingAssistantCandidateSearch,
			log.AREventKeyRaw:        classified(short, IDENTIFIER),
			log.AREventKeyNodeMac:    mac(t, raNode),
			log.AREventKeyNodeRssi:   -10,
			log.AREventKeyClientMac:  mac(t, raClient),
			log.AREventKeyClientRssi: -32,
		}},
	})
}

func TestClientRoaming(t *testing.T) {
	runCases(t, RoamingAssistantPatterns, []tcase{
		{name: "parses_fields", content: raRoam, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantClientRoaming,
			log.AREventKeyRaw:       classified(raRoam, IDENTIFIER),
			log.AREventKeyClientMac: mac(t, raClient),
			log.AREventKeyStatus:    0,
		}},
	})
}

func TestDeauthOldSta(t *testing.T) {
	node := "[EXAP]Deauth old sta in 0 2: " + raClient
	runCases(t, RoamingAssistantPatterns, []tcase{
		{name: "router_with_reason", content: raAlready, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantDeauthOldSta,
			log.AREventKeyRaw:       classified(raAlready, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 2),
			log.AREventKeyClientMac: mac(t, raClient),
			log.AREventKeyNodeMac:   mac(t, raNode),
		}},
		{name: "node_without_reason", content: node, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantDeauthOldSta,
			log.AREventKeyRaw:       classified(node, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 2),
			log.AREventKeyClientMac: mac(t, raClient),
		}},
	})
}

func TestMonitoredClient(t *testing.T) {
	bind := "wl0.2: sta-ap-band-bind deauth [" + raClient + "]"
	runCases(t, RoamingAssistantPatterns, []tcase{
		{name: "disconnect_weak_signal", content: raWeak, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantDisconnectWeakSignal,
			log.AREventKeyRaw:       classified(raWeak, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 2),
			log.AREventKeyClientMac: mac(t, raClient),
		}},
		{name: "remove_client", content: raRemove, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantRemoveClient,
			log.AREventKeyRaw:       classified(raRemove, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 2),
			log.AREventKeyClientMac: mac(t, raClient),
		}},
		{name: "sta_ap_band_bind_deauth", content: bind, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantStaApBandBindDeauth,
			log.AREventKeyRaw:       classified(bind, IDENTIFIER),
			log.AREventKeyWlId:      wl(0, 2),
			log.AREventKeyClientMac: mac(t, raClient),
		}},
	})
}

func TestTranslateRoamingAssistant(t *testing.T) {
	unknown := "some future roamast message we do not handle yet"
	runCases(t, RoamingAssistantPatterns, []tcase{
		{name: "roaming_start", content: "ROAMING Start", want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantRoamingStart,
			log.AREventKeyRaw:       "ROAMING Start",
		}},
		{name: "unrecognized", content: unknown, want: ev{
			log.AREventKeyEventType: AREventRoamingAssistantUnknown,
			log.AREventKeyRaw:       unread(unknown),
		}},
	})
}
