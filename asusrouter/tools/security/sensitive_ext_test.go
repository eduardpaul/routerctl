package security_test

import (
	"fmt"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const (
	extMac      = "aa:bb:cc:11:22:33"
	extIP       = "192.168.1.10"
	extSsid     = "my-network"
	extPassword = "s3cr3t"
)

func TestRedactedStrAndRepr(t *testing.T) {
	if fmt.Sprint(security.Redacted) != security.RedactedStr || security.RedactedStr != "-=REDACTED=-" {
		t.Fatal("str")
	}
	if fmt.Sprintf("%#v", security.Redacted) != "<redacted>" {
		t.Fatal("repr")
	}
}

func TestSensitiveWrapper(t *testing.T) {
	// The default reveal level is UNSAFE
	wrapped := security.NewSensitive("payload")
	if wrapped.RevealLevel() != security.ARSecurityLevelUnsafe || wrapped.Maskable() || wrapped.Mask() != security.Redacted {
		t.Fatal("defaults")
	}

	// Value is stored and str returns the raw form
	wrapped = security.Sensitive{Value: "payload", Level: security.ARSecurityLevelDefault}
	if wrapped.Value != "payload" || wrapped.RevealLevel() != security.ARSecurityLevelDefault || wrapped.String() != "payload" {
		t.Fatal("value")
	}
	if got := fmt.Sprintf("%#v", wrapped); got != "Sensitive('payload', <ARSecurityLevel.DEFAULT: 1>)" {
		t.Fatalf("repr = %q", got)
	}
	if wrapped.CanonicalValue() != "payload" {
		t.Fatal("canonical")
	}
}

func TestSensitiveRender(t *testing.T) {
	cases := []struct {
		reveal, level security.ARSecurityLevel
		raw           bool
	}{
		{security.ARSecurityLevelDefault, security.ARSecurityLevelSanitized, true},
		{security.ARSecurityLevelUnsafe, security.ARSecurityLevelSanitized, false},
		{security.ARSecurityLevelUnsafe, security.ARSecurityLevelUnsafe, true},
	}
	for _, c := range cases {
		wrapped := security.Sensitive{Value: "payload", Level: c.reveal}
		result := security.Render(wrapped, c.level)
		if c.raw && result != any(wrapped) || !c.raw && result != security.Redacted {
			t.Errorf("Render(%v at %v) = %v", c.reveal, c.level, result)
		}
	}
}

func TestRenderPassthrough(t *testing.T) {
	for _, v := range []any{"plain", 42, nil} {
		if security.Render(v, security.ARSecurityLevelStrict) != v {
			t.Errorf("Render(%#v) changed", v)
		}
	}
}

type outcome int

const (
	redacted outcome = iota
	masked
	raw
)

func checkMatrix(t *testing.T, name string, value security.ARSensitive, matrix map[security.ARSecurityLevel]outcome) {
	t.Helper()
	for level, expected := range matrix {
		result := security.Render(value, level)
		switch expected {
		case raw:
			if result != any(value) {
				t.Errorf("%s at %v: want raw, got %v", name, level, result)
			}
		case masked:
			if result != value.Mask() || result == any(value) {
				t.Errorf("%s at %v: want masked, got %v", name, level, result)
			}
		default:
			if result != security.Redacted {
				t.Errorf("%s at %v: want redacted, got %v", name, level, result)
			}
		}
	}
}

func TestRenderMatrix(t *testing.T) {
	mac, _ := identifiers.MacAddressFromValue(extMac)
	ip, _ := identifiers.IpAddressFromValue(extIP)

	// SSID reveals from DEFAULT, never maskable
	checkMatrix(t, "ssid", identifiers.NewSsid(extSsid), map[security.ARSecurityLevel]outcome{
		security.ARSecurityLevelStrict: redacted, security.ARSecurityLevelDefault: raw,
		security.ARSecurityLevelSanitized: raw, security.ARSecurityLevelReasonable: raw,
		security.ARSecurityLevelUnsafe: raw,
	})
	hardware := map[security.ARSecurityLevel]outcome{
		security.ARSecurityLevelStrict: redacted, security.ARSecurityLevelDefault: redacted,
		security.ARSecurityLevelSanitized: masked, security.ARSecurityLevelReasonable: raw,
		security.ARSecurityLevelUnsafe: raw,
	}
	checkMatrix(t, "mac", mac, hardware)
	checkMatrix(t, "ip", ip, hardware)
	// Password is never masked, only raw at UNSAFE
	checkMatrix(t, "password", identifiers.NewPassword(extPassword), map[security.ARSecurityLevel]outcome{
		security.ARSecurityLevelStrict: redacted, security.ARSecurityLevelDefault: redacted,
		security.ARSecurityLevelSanitized: redacted, security.ARSecurityLevelReasonable: redacted,
		security.ARSecurityLevelUnsafe: raw,
	})
}
