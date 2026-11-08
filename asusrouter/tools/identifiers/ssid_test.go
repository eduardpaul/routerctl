package identifiers

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testSsid = "my-network"

func TestSsidIsSensitive(t *testing.T) {
	var s security.ARSensitive = NewSsid(testSsid)
	if s.RevealLevel() != security.ARSecurityLevelDefault || s.Maskable() {
		t.Fatal("classification")
	}
}

func TestSsidValueAndStr(t *testing.T) {
	s := NewSsid(testSsid)
	if s.Value() != testSsid || s.String() != testSsid || s.GoString() != "Ssid('my-network')" {
		t.Fatal("value")
	}
	// Logged raw at the default (SANITIZED) log level
	if s.LogValue().String() != testSsid {
		t.Fatal("log value")
	}
}

func TestSsidFromValue(t *testing.T) {
	s := NewSsid(testSsid)
	if SsidFromValue(s) != s || SsidFromValue(testSsid) != s {
		t.Fatal("from_value")
	}
	if got := SsidFromValueSafe(s); got == nil || *got != s {
		t.Fatal("safe instance")
	}
	if got := SsidFromValueSafe(testSsid); got == nil || *got != s {
		t.Fatal("safe string")
	}
	if SsidFromValueSafe(nil) != nil || SsidFromValueSafe("") != nil {
		t.Fatal("safe empty")
	}
}

func TestSsidDefaultMask(t *testing.T) {
	if NewSsid(testSsid).Mask() != security.Redacted {
		t.Fatal("mask")
	}
}

func TestSsidEquality(t *testing.T) {
	s := NewSsid(testSsid)
	if s != NewSsid(testSsid) || !s.Equal(testSsid) || s.Equal(42) {
		t.Fatal("equality")
	}
}
