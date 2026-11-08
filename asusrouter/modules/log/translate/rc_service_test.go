package translate

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const (
	rcWaiting = `waitting "stop_httpd" via udhcpc_lan ...`
	// Newer firmware appends the in-flight action right after the quote
	rcWaitingLastRc = `waitting "restart_wan_if 0"(last_rc:stop_samba) via wanduck ...`
)

func TestNotify(t *testing.T) {
	single := "service 2011:notify_rc restart_firewall"
	runCases(t, RcServicePatterns, []tcase{
		{name: "single", content: single, want: ev{
			log.AREventKeyEventType: AREventRcServiceNotify,
			log.AREventKeyRaw:       single,
			log.AREventKeyCaller:    "service",
			log.AREventKeyPid:       2011,
			log.AREventKeyServices:  []common.ARService{common.ARServiceFirewallRestart},
			log.AREventKeyActions:   []string{"restart_firewall"},
		}},
		{name: "multiple",
			content: "cfg_client 2621:notify_rc restart_wireless;chpass;restart_time;restart_qos;restart_firewall",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyServices, []common.ARService{
					common.ARServiceWirelessRestart,
					common.ARServiceChpass,
					common.ARServiceTimeRestart,
					common.ARServiceQosRestart,
					common.ARServiceFirewallRestart,
				})
			}},
		// A real RT-AC66U line
		{name: "trailing_separator", content: "httpd 1234:notify_rc restart_time;restart_upnp;",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyActions, []string{"restart_time", "restart_upnp"})
				eq(t, e, log.AREventKeyServices, []common.ARService{
					common.ARServiceTimeRestart,
					common.ARServiceUpnpRestart,
				})
			}},
		{name: "action_argument", content: "cfg_client 4875:notify_rc stop_upgrade;start_webs_upgrade 1",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyServices, []common.ARService{
					common.ARServiceFirmwareUpgradeStop,
					common.ARServiceFirmwareWebUpgradeStart,
				})
				eq(t, e, log.AREventKeyActions, []string{"stop_upgrade", "start_webs_upgrade 1"})
			}},
		{name: "unknown_service", content: "rc 5715:notify_rc restart_wireless;start_madeup_service",
			check: func(t *testing.T, e ev) {
				eq(t, e, log.AREventKeyServices, []common.ARService{
					common.ARServiceWirelessRestart,
					common.ARServiceUnknown,
				})
				eq(t, e, log.AREventKeyActions, []string{"restart_wireless", "start_madeup_service"})
			}},
	})
}

func TestWaiting(t *testing.T) {
	runCases(t, RcServicePatterns, []tcase{
		{name: "match", content: rcWaiting, want: ev{
			log.AREventKeyEventType: AREventRcServiceWaiting,
			log.AREventKeyRaw:       rcWaiting,
			log.AREventKeyCaller:    "udhcpc_lan",
			log.AREventKeyService:   common.ARServiceWebuiStop,
			log.AREventKeyAction:    "stop_httpd",
		}},
		{name: "last_rc", content: rcWaitingLastRc, want: ev{
			log.AREventKeyEventType:   AREventRcServiceWaiting,
			log.AREventKeyRaw:         rcWaitingLastRc,
			log.AREventKeyCaller:      "wanduck",
			log.AREventKeyService:     common.ARServiceWanInterfaceRestart,
			log.AREventKeyAction:      "restart_wan_if 0",
			log.AREventKeyLastAction:  "stop_samba",
			log.AREventKeyLastService: common.ARServiceSambaStop,
		}},
		{name: "empty_last_rc_dropped", content: `waitting "stop_httpd"(last_rc:) via wanduck ...`,
			check: func(t *testing.T, e ev) {
				absent(t, e, log.AREventKeyLastAction)
				absent(t, e, log.AREventKeyLastService)
			}},
	})
}

func TestTranslateRcService(t *testing.T) {
	content := "some future rc_service message we do not handle yet"
	runCases(t, RcServicePatterns, []tcase{
		{name: "unrecognized", content: content, want: ev{
			log.AREventKeyEventType: AREventRcServiceUnknown,
			log.AREventKeyRaw:       unread(content),
		}},
	})
}
