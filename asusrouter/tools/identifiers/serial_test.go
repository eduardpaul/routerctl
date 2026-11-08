package identifiers

import (
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testSerial = "R2FAKE000000ABC"

func TestSerialValue(t *testing.T) {
	s := NewSerial(testSerial)
	if s.Value() != testSerial || s.String() != testSerial || !strings.Contains(s.GoString(), testSerial) {
		t.Fatal("value")
	}
}

func TestSerialIsHardwareIdentifier(t *testing.T) {
	s := NewSerial(testSerial)
	if s.RevealLevel() != security.ARSecurityLevelReasonable || !s.Maskable() {
		t.Fatal("classification")
	}
}

func TestSerialMaskDeterministic(t *testing.T) {
	masked := NewSerial(testSerial).Mask().(Serial)
	if masked.Equal(testSerial) || !strings.HasPrefix(masked.String(), "sn-") || masked != NewSerial(testSerial).Mask() {
		t.Fatal("mask")
	}
}

func TestSerialRender(t *testing.T) {
	cases := []struct {
		level    security.ARSecurityLevel
		expected string
	}{
		{security.ARSecurityLevelStrict, "redacted"},
		{security.ARSecurityLevelSanitized, "masked"},
		{security.ARSecurityLevelReasonable, "raw"},
		{security.ARSecurityLevelUnsafe, "raw"},
	}
	for _, c := range cases {
		rendered := security.Render(NewSerial(testSerial), c.level)
		switch c.expected {
		case "redacted":
			if rendered != security.Redacted {
				t.Errorf("%v: %v", c.level, rendered)
			}
		case "masked":
			if !strings.HasPrefix(security.PyStrOf(rendered), "sn-") {
				t.Errorf("%v: %v", c.level, rendered)
			}
		default:
			if rendered != NewSerial(testSerial) {
				t.Errorf("%v: %v", c.level, rendered)
			}
		}
	}
}

func TestSerialFromValue(t *testing.T) {
	s := NewSerial(testSerial)
	if SerialFromValue(s) != s || SerialFromValue(testSerial) != s {
		t.Fatal("from_value")
	}
	for _, v := range []any{nil, ""} {
		if SerialFromValueSafe(v) != nil {
			t.Errorf("safe(%#v)", v)
		}
	}
	if got := SerialFromValueSafe(s); got == nil || *got != s {
		t.Fatal("safe instance")
	}
	if got := SerialFromValueSafe(testSerial); got == nil || *got != s {
		t.Fatal("safe string")
	}
}

func TestSerialEquality(t *testing.T) {
	s := NewSerial(testSerial)
	if s != NewSerial(testSerial) || !s.Equal(testSerial) || s == NewSerial("other") || s.Equal(42) {
		t.Fatal("equality")
	}
}
