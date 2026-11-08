package identifiers

import (
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

const testName = "fakeadmin"

func TestUsernameValue(t *testing.T) {
	if NewUsername(testName).Value() != testName {
		t.Fatal("value")
	}
}

func TestUsernameStringRedacts(t *testing.T) {
	u := NewUsername(testName)
	if u.PyStr() != security.RedactedStr || !strings.Contains(u.GoString(), security.RedactedStr) || strings.Contains(u.GoString(), testName) {
		t.Fatal("python forms must redact")
	}
}

func TestUsernameIsCredential(t *testing.T) {
	u := NewUsername(testName)
	if u.RevealLevel() != security.ARSecurityLevelUnsafe || u.Maskable() {
		t.Fatal("classification")
	}
}

func TestUsernameRender(t *testing.T) {
	cases := map[security.ARSecurityLevel]bool{
		security.ARSecurityLevelStrict:     false,
		security.ARSecurityLevelSanitized:  false,
		security.ARSecurityLevelReasonable: false,
		security.ARSecurityLevelUnsafe:     true,
	}
	for level, revealed := range cases {
		if got := security.Render(NewUsername(testName), level) != security.Redacted; got != revealed {
			t.Errorf("%v: revealed = %v", level, got)
		}
	}
}

func TestUsernameFromValue(t *testing.T) {
	u := NewUsername(testName)
	if UsernameFromValue(u) != u || UsernameFromValue(testName) != u {
		t.Fatal("from_value")
	}
	for _, v := range []any{nil, ""} {
		if UsernameFromValueSafe(v) != nil {
			t.Errorf("safe(%#v)", v)
		}
	}
	if got := UsernameFromValueSafe(u); got == nil || *got != u {
		t.Fatal("safe instance")
	}
	if got := UsernameFromValueSafe(testName); got == nil || *got != u {
		t.Fatal("safe string")
	}
}

func TestUsernameEquality(t *testing.T) {
	u := NewUsername(testName)
	if u != NewUsername(testName) || !u.Equal(testName) || u == NewUsername("other") || u.Equal(42) {
		t.Fatal("equality")
	}
}
