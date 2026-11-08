package common

import "testing"

// The request body keys match the device fields.
func TestKeys(t *testing.T) {
	if ActionModeKey != "action_mode" {
		t.Errorf("ActionModeKey = %q", ActionModeKey)
	}
	if RcServiceKey != "rc_service" {
		t.Errorf("RcServiceKey = %q", RcServiceKey)
	}
}

func TestARActionModeValues(t *testing.T) {
	// APPLY maps to its string value
	if ARActionModeApply != "apply" {
		t.Errorf("APPLY = %q", ARActionModeApply)
	}
	// AIMESH_REBOOT keeps the raw Asus string
	if ARActionModeAimeshReboot != "device_reboot" {
		t.Errorf("AIMESH_REBOOT = %q", ARActionModeAimeshReboot)
	}
}

// from_value maps known strings, falls back to UNKNOWN.
func TestARActionModeFromValue(t *testing.T) {
	cases := []struct {
		value    any
		expected ARActionMode
	}{
		{"apply", ARActionModeApply},
		{"re_reconnect", ARActionModeAimeshRebuild},
		{"nonsense", ARActionModeUnknown},
		{nil, ARActionModeUnknown},
	}
	for _, c := range cases {
		if got := ARActionModeFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestARServiceValues(t *testing.T) {
	// WIRELESS_RESTART maps to the raw Asus string
	if ARServiceWirelessRestart != "restart_wireless" {
		t.Errorf("WIRELESS_RESTART = %q", ARServiceWirelessRestart)
	}
	// A feature-renamed member keeps the Asus daemon string
	if ARServiceAuraRestart != "restart_ledg" || ARServiceWebuiRestart != "restart_httpd" {
		t.Errorf("renamed members lost their raw values")
	}
}

// from_value maps known strings, falls back to UNKNOWN.
func TestARServiceFromValue(t *testing.T) {
	cases := []struct {
		value    any
		expected ARService
	}{
		{"restart_wireless", ARServiceWirelessRestart},
		{"restart_firewall", ARServiceFirewallRestart},
		{"restart_sdn", ARServiceSdnRestart},
		{"restart_unknown", ARServiceUnknown},
		{nil, ARServiceUnknown},
	}
	for _, c := range cases {
		if got := ARServiceFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

// Stringifying a member yields the raw service string.
func TestARServiceStrIsRawValue(t *testing.T) {
	if got := string(ARServiceSdnRestart); got != "restart_sdn" {
		t.Errorf("str = %q", got)
	}
}
