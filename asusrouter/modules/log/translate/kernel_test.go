package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const (
	kernelClient = "aa:bb:cc:00:00:01"
	kernelPort   = "eth3 (Int switch port: 10) (Logical Port: 10) (phyId: 1b)"
	kernelDown   = kernelPort + " Link DOWN."
	kernelUp     = kernelPort + " Link Up at 100 mbps full duplex AN: On"
)

func TestBridge(t *testing.T) {
	portState := "br0: port 4(wl0.1) entering disabled state"
	topology := "br0: topology change detected, propagating"
	cases := []tcase{
		{name: "port_state", content: portState, want: ev{
			log.AREventKeyEventType: AREventKernelBridgePortState,
			log.AREventKeyRaw:       portState,
			log.AREventKeyBridge:    "br0",
			log.AREventKeyPort:      4,
			log.AREventKeyInterface: "wl0.1",
			log.AREventKeyState:     "disabled",
		}},
	}
	for _, state := range []string{"listening", "learning", "forwarding"} {
		state := state
		cases = append(cases, tcase{name: "state_variants/" + state,
			content: "br0: port 1(vlan1) entering " + state + " state",
			check:   func(t *testing.T, e ev) { eq(t, e, log.AREventKeyState, state) }})
	}
	cases = append(cases, tcase{name: "topology_change", content: topology, want: ev{
		log.AREventKeyEventType: AREventKernelBridgeTopologyChange,
		log.AREventKeyRaw:       topology,
		log.AREventKeyBridge:    "br0",
	}})
	runCases(t, KernelPatterns, cases)
}

func TestEthernetLink(t *testing.T) {
	runCases(t, KernelPatterns, []tcase{
		{name: "down", content: kernelDown, want: ev{
			log.AREventKeyEventType:   AREventKernelEthernetLinkDown,
			log.AREventKeyRaw:         kernelDown,
			log.AREventKeyInterface:   "eth3",
			log.AREventKeySwitchPort:  10,
			log.AREventKeyLogicalPort: 10,
			log.AREventKeyPhyId:       "1b",
		}},
		{name: "up_with_autoneg", content: kernelUp, want: ev{
			log.AREventKeyEventType:   AREventKernelEthernetLinkUp,
			log.AREventKeyRaw:         kernelUp,
			log.AREventKeyInterface:   "eth3",
			log.AREventKeySwitchPort:  10,
			log.AREventKeyLogicalPort: 10,
			log.AREventKeyPhyId:       "1b",
			log.AREventKeySpeed:       100,
			log.AREventKeyDuplex:      "full",
			log.AREventKeyAutoneg:     true,
		}},
		{name: "up_without_autoneg",
			content: "eth0 (Int switch port: 3) (Logical Port: 3) (phyId: c) Link UP at 1000 mbps full duplex",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeySpeed, 1000)
				absent(t, e, log.AREventKeyAutoneg)
			}},
	})
}

func TestDriver(t *testing.T) {
	klogd := "klogd started: BusyBox v1.25.1 (2018-04-08 14:04:33 EDT)"
	controller := "eth1: Broadcom BCM4331 802.11 Wireless Controller 6.30.163.2002 (r382208)"
	runCases(t, KernelPatterns, []tcase{
		{name: "klogd_start", content: klogd, want: ev{
			log.AREventKeyEventType: AREventKernelKlogdStart,
			log.AREventKeyRaw:       klogd,
			log.AREventKeyVersion:   "1.25.1",
		}},
		{name: "phy_cal_init", content: "wlc_phy_cal_init_acphy: NOT Implemented", want: ev{
			log.AREventKeyEventType: AREventKernelPhyCalInit,
			log.AREventKeyRaw:       "wlc_phy_cal_init_acphy: NOT Implemented",
		}},
		{name: "tasklet_kill_attempt", content: "Attempt to kill tasklet from interrupt", want: ev{
			log.AREventKeyEventType: AREventKernelTaskletKillAttempt,
			log.AREventKeyRaw:       "Attempt to kill tasklet from interrupt",
		}},
		{name: "wireless_controller", content: controller, want: ev{
			log.AREventKeyEventType: AREventKernelWirelessController,
			log.AREventKeyRaw:       controller,
			log.AREventKeyInterface: "eth1",
			log.AREventKeyChip:      "BCM4331",
			log.AREventKeyVersion:   "6.30.163.2002",
			log.AREventKeyRevision:  382208,
		}},
		{name: "wl_module_init", content: "wl_module_init: passivemode set to 0x1", want: ev{
			log.AREventKeyEventType: AREventKernelWlModuleInit,
			log.AREventKeyRaw:       "wl_module_init: passivemode set to 0x1",
			log.AREventKeyParameter: "passivemode",
			log.AREventKeyValue:     1,
		}},
	})
}

func TestPromiscuousMode(t *testing.T) {
	runCases(t, KernelPatterns, []tcase{
		{name: "entered", content: "device wl0.1 entered promiscuous mode", want: ev{
			log.AREventKeyEventType: AREventKernelPromiscuousMode,
			log.AREventKeyRaw:       "device wl0.1 entered promiscuous mode",
			log.AREventKeyInterface: "wl0.1",
			log.AREventKeyEnabled:   true,
		}},
		{name: "left", content: "device eth1 left promiscuous mode", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyEnabled, false)
		}},
		{name: "set_promiscuity_on", content: "vlan1: dev_set_promiscuity(master, 1)", want: ev{
			log.AREventKeyEventType: AREventKernelPromiscuousMode,
			log.AREventKeyRaw:       "vlan1: dev_set_promiscuity(master, 1)",
			log.AREventKeyInterface: "vlan1",
			log.AREventKeyEnabled:   true,
		}},
		{name: "set_promiscuity_off", content: "vlan1: dev_set_promiscuity(master, 0)", check: func(t *testing.T, e ev) {
			eq(t, e, log.AREventKeyEnabled, false)
		}},
	})
}

func TestKernelStation(t *testing.T) {
	sbf := "SBF: dhd0: INIT [" + kernelClient + "] ID 65535 BFW 65535 THRSH 2048"
	runCases(t, KernelPatterns, []tcase{
		{name: "sbf_init", content: sbf, want: ev{
			log.AREventKeyEventType: AREventKernelSbfInit,
			log.AREventKeyRaw:       classified(sbf, IDENTIFIER),
			log.AREventKeyClientMac: mac(t, kernelClient),
		}},
		{name: "scb_deauthorize", content: "WLC_SCB_DEAUTHORIZE error (-30)", want: ev{
			log.AREventKeyEventType: AREventKernelScbDeauthError,
			log.AREventKeyRaw:       "WLC_SCB_DEAUTHORIZE error (-30)",
			log.AREventKeyCommand:   "WLC_SCB_DEAUTHORIZE",
			log.AREventKeyError:     -30,
		}},
		{name: "scb_deauthenticate", content: "WLC_SCB_DEAUTHENTICATE_FOR_REASON err -30", want: ev{
			log.AREventKeyEventType: AREventKernelScbDeauthError,
			log.AREventKeyRaw:       "WLC_SCB_DEAUTHENTICATE_FOR_REASON err -30",
			log.AREventKeyCommand:   "WLC_SCB_DEAUTHENTICATE_FOR_REASON",
			log.AREventKeyError:     -30,
		}},
	})
}

func TestTranslateKernel(t *testing.T) {
	content := "some future kernel message we do not handle yet"
	runCases(t, KernelPatterns, []tcase{
		{name: "unrecognized", content: content, want: ev{
			log.AREventKeyEventType: AREventKernelUnknown,
			log.AREventKeyRaw:       unread(content),
		}},
	})
}
