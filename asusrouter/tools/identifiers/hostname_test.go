package identifiers

import (
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testHost = "router.example.com"

func TestHostnameIsSensitive(t *testing.T) {
	var host security.ARSensitive = NewHostname(testHost)
	if host.RevealLevel() != security.ARSecurityLevelReasonable || !host.Maskable() {
		t.Fatal("bad classification")
	}
}

func TestHostnameValueAndStr(t *testing.T) {
	host := NewHostname(testHost)
	if host.Value() != testHost || host.String() != testHost || host.GoString() != "Hostname('router.example.com')" {
		t.Fatalf("got %q %q %q", host.Value(), host.String(), host.GoString())
	}
}

func TestHostnameFromValue(t *testing.T) {
	host := NewHostname(testHost)
	if HostnameFromValue(host) != host || HostnameFromValue(testHost) != host {
		t.Fatal("from_value")
	}
	if got := HostnameFromValueSafe(host); got == nil || *got != host {
		t.Fatal("safe instance")
	}
	if got := HostnameFromValueSafe(testHost); got == nil || *got != host {
		t.Fatal("safe string")
	}
	if HostnameFromValueSafe(nil) != nil || HostnameFromValueSafe("") != nil {
		t.Fatal("safe empty")
	}
}

func TestHostnameMask(t *testing.T) {
	security.ConfigureKey([]byte(strings.Repeat("A", 32)))
	host := NewHostname(testHost)
	masked := host.Mask().(Hostname)
	if masked == host || !strings.HasPrefix(masked.Value(), "host-") || masked != host.Mask() {
		t.Fatalf("masked = %q", masked)
	}
	ipHost := NewHostname("192.168.1.1")
	m1 := ipHost.Mask()
	security.ConfigureKey([]byte(strings.Repeat("B", 32)))
	if ipHost.Mask() == m1 {
		t.Fatal("mask does not depend on the key")
	}
}

func TestHostnameEquality(t *testing.T) {
	host := NewHostname(testHost)
	if host != NewHostname(testHost) || !host.Equal(testHost) || host.Equal(42) {
		t.Fatal("equality")
	}
}
