package identifiers

import (
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testPassword = "s3cr3t"

func TestPasswordIsSensitive(t *testing.T) {
	var p security.ARSensitive = NewPassword(testPassword)
	if p.RevealLevel() != security.ARSecurityLevelUnsafe || p.Maskable() {
		t.Fatal("bad classification")
	}
}

func TestPasswordValueButPyStrRedacted(t *testing.T) {
	p := NewPassword(testPassword)
	if p.Value() != testPassword || p.String() != testPassword {
		t.Fatal("raw value")
	}
	if p.PyStr() != security.RedactedStr || p.GoString() != "Password("+security.RedactedStr+")" {
		t.Fatal("python forms must redact")
	}
	if strings.Contains(p.GoString(), testPassword) || strings.Contains(p.LogValue().String(), testPassword) {
		t.Fatal("leak")
	}
	// Even at UNSAFE, Python's str() of the rendered password is redacted
	if security.RenderString(p, security.ARSecurityLevelUnsafe) != security.RedactedStr {
		t.Fatal("rendered string leaks")
	}
}

func TestPasswordFromValue(t *testing.T) {
	p := NewPassword(testPassword)
	if PasswordFromValue(p) != p || PasswordFromValue(testPassword) != p {
		t.Fatal("from_value")
	}
	if got := PasswordFromValueSafe(p); got == nil || *got != p {
		t.Fatal("safe instance")
	}
	if got := PasswordFromValueSafe(testPassword); got == nil || *got != p {
		t.Fatal("safe string")
	}
	if PasswordFromValueSafe(nil) != nil || PasswordFromValueSafe("") != nil {
		t.Fatal("safe empty")
	}
}

func TestPasswordDefaultMask(t *testing.T) {
	if NewPassword(testPassword).Mask() != security.Redacted {
		t.Fatal("mask")
	}
}

func TestPasswordEquality(t *testing.T) {
	p := NewPassword(testPassword)
	if p != NewPassword(testPassword) || !p.Equal(testPassword) || p.Equal(42) {
		t.Fatal("equality")
	}
}
