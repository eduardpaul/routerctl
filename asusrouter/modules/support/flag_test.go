package support

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

func TestARSupportType(t *testing.T) {
	// UNKNOWN carries the sentinel value
	if ARSupportTypeUnknown != consts.UnknownMemberStr {
		t.Errorf("UNKNOWN = %q", ARSupportTypeUnknown)
	}
	// Every member resolves from its own raw value
	for _, m := range ARSupportTypeMembers {
		if got := ARSupportTypeFromValue(string(m)); got != m {
			t.Errorf("FromValue(%q) = %v", m, got)
		}
	}
	// An unrecognised value falls back to UNKNOWN
	if ARSupportTypeFromValue("no-such-flag") != ARSupportTypeUnknown {
		t.Error("unknown fallback")
	}
}

func TestARSupportValue(t *testing.T) {
	if ARSupportValueUnknown != consts.UnknownMemberStr {
		t.Errorf("UNKNOWN = %q", ARSupportValueUnknown)
	}
	for _, m := range ARSupportValueMembers {
		if got := ARSupportValueFromValue(string(m)); got != m {
			t.Errorf("FromValue(%q) = %v", m, got)
		}
	}
	if ARSupportValueFromValue("no-such-value") != ARSupportValueUnknown {
		t.Error("unknown fallback")
	}
}
