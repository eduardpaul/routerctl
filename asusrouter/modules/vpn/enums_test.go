package vpn_test

// Port of tests/modules/vpn/test_enums.py.

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
)

func TestVpnEnumsRoundTrip(t *testing.T) {
	// Every member resolves from its own stored value
	for _, m := range vpn.ARVpnProtocolMembers {
		if got := vpn.ARVpnProtocolFromValue(string(m)); got != m {
			t.Errorf("ARVpnProtocol %s -> %s", m, got)
		}
	}
	for _, m := range vpn.ARVpnRoleMembers {
		if got := vpn.ARVpnRoleFromValue(string(m)); got != m {
			t.Errorf("ARVpnRole %s -> %s", m, got)
		}
	}
	for _, m := range vpn.ARVpnStateMembers {
		if got := vpn.ARVpnStateFromValue(int(m)); got != m {
			t.Errorf("ARVpnState %d -> %d", m, got)
		}
	}
	for _, m := range vpn.ARVpnServerFieldMembers {
		if got := vpn.ARVpnServerFieldFromValue(string(m)); got != m {
			t.Errorf("ARVpnServerField %s -> %s", m, got)
		}
	}
	for _, m := range vpn.ARVpnPeerFieldMembers {
		if got := vpn.ARVpnPeerFieldFromValue(string(m)); got != m {
			t.Errorf("ARVpnPeerField %s -> %s", m, got)
		}
	}
	for _, m := range vpn.ARVpnClientFieldMembers {
		if got := vpn.ARVpnClientFieldFromValue(string(m)); got != m {
			t.Errorf("ARVpnClientField %s -> %s", m, got)
		}
	}
}

func TestVpnEnumsUnknownFallback(t *testing.T) {
	// An unrecognised value falls back to UNKNOWN
	const bad = "zzz-not-a-real-enum-value"
	if vpn.ARVpnProtocolFromValue(bad) != vpn.ARVpnProtocolUnknown ||
		vpn.ARVpnRoleFromValue(bad) != vpn.ARVpnRoleUnknown ||
		vpn.ARVpnStateFromValue(bad) != vpn.ARVpnStateUnknown ||
		vpn.ARVpnServerFieldFromValue(bad) != vpn.ARVpnServerFieldUnknown ||
		vpn.ARVpnPeerFieldFromValue(bad) != vpn.ARVpnPeerFieldUnknown ||
		vpn.ARVpnClientFieldFromValue(bad) != vpn.ARVpnClientFieldUnknown {
		t.Error("unknown fallback")
	}
}
