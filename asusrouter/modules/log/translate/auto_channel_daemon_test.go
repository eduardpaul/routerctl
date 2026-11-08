package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
)

func TestChannelSpec(t *testing.T) {
	verbs := []struct {
		content string
		event   AREventAutoChannelDaemon
	}{
		{"wl1.1: Adjusted channel spec: 0xe832 (36/160)", AREventAutoChannelDaemonChannelAdjusted},
		{"wl1.1: selected channel spec: 0xe832 (36/160)", AREventAutoChannelDaemonChannelSelected},
		{"wl1.1: NONACSD channel switching to channel spec: 0xe832 (36/160)", AREventAutoChannelDaemonChannelSwitched},
	}
	var cases []tcase
	for _, v := range verbs {
		cases = append(cases, tcase{name: "verbs/" + string(v.event), content: v.content, want: ev{
			log.AREventKeyEventType: v.event,
			log.AREventKeyRaw:       v.content,
			log.AREventKeyWlId:      wl(1, 1),
			log.AREventKeyChannel:   36,
			log.AREventKeyBandwidth: wifi.ARWiFiBandwidthWidth160,
		}})
	}
	eth := "eth7: selected channel spec: 0xe06a (100/80)"
	cases = append(cases, tcase{name: "ethernet_named_radio", content: eth, want: ev{
		log.AREventKeyEventType: AREventAutoChannelDaemonChannelSelected,
		log.AREventKeyRaw:       eth,
		log.AREventKeyInterface: "eth7",
		log.AREventKeyChannel:   100,
		log.AREventKeyBandwidth: wifi.ARWiFiBandwidthWidth80,
	}})
	for _, spec := range []string{"0x1007 (7)", "0x180a (8l)", "0x1904 (6u)"} {
		cases = append(cases, tcase{name: "width_left_out/" + spec, content: "wl0.1: selected channel spec: " + spec,
			check: func(t *testing.T, e ev) {
				present(t, e, log.AREventKeyChannel)
				absent(t, e, log.AREventKeyBandwidth)
			}})
	}
	cases = append(cases, tcase{name: "unknown_width_dropped", content: "wl0.1: selected channel spec: 0x1007 (7/33)",
		check: func(t *testing.T, e ev) { absent(t, e, log.AREventKeyBandwidth) }})
	runCases(t, AutoChannelDaemonPatterns, cases)
}

func TestPolicy(t *testing.T) {
	runCases(t, AutoChannelDaemonPatterns, []tcase{
		{name: "policy_selected", content: "wl0.1: Selecting 2g band ACS policy", want: ev{
			log.AREventKeyEventType: AREventAutoChannelDaemonPolicySelected,
			log.AREventKeyRaw:       "wl0.1: Selecting 2g band ACS policy",
			log.AREventKeyWlId:      wl(0, 1),
			log.AREventKeyFrequency: wifi.ARWiFiFrequencyFreq2g,
		}},
		{name: "five_gigahertz", content: "eth7: Selecting 5g band ACS policy", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyFrequency, wifi.ARWiFiFrequencyFreq5g)
			eq(t, e, log.AREventKeyInterface, "eth7")
		}},
		{name: "unknown_frequency_dropped", content: "wl0.1: Selecting 9g band ACS policy", check: func(t *testing.T, e ev) {
			absent(t, e, log.AREventKeyFrequency)
		}},
	})
}

func TestUnhandled(t *testing.T) {
	var cases []tcase
	for _, content := range []string{
		// Numeric driver codes with no confirmed meaning
		"acs_update_driver(441): acs update failed ret code: -22",
		"acs_set_chspec: 0xe832 (36/160) for reason ACS_INIT",
		"acs_init_run(1000): wl0.1: update driver failed",
	} {
		cases = append(cases, tcase{name: "internal_traces/" + content, content: content, want: ev{
			log.AREventKeyEventType: AREventAutoChannelDaemonUnknown,
			log.AREventKeyRaw:       unread(content),
		}})
	}
	runCases(t, AutoChannelDaemonPatterns, cases)
}
