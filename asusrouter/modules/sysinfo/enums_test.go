package sysinfo

import "testing"

// Ported from tests/modules/sysinfo/test_enums.py.

func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARMemoryTypeMembers {
		if got := ARMemoryTypeFromValue(string(m)); got != m {
			t.Errorf("ARMemoryType.%s: got %q", m.Name(), got)
		}
	}
	for _, m := range ARSysInfoTypeMembers {
		if got := ARSysInfoTypeFromValue(string(m)); got != m {
			t.Errorf("ARSysInfoType.%s: got %q", m.Name(), got)
		}
	}
	for _, m := range ARWlanClientCountMembers {
		if got := ARWlanClientCountFromValue(string(m)); got != m {
			t.Errorf("ARWlanClientCount.%s: got %q", m.Name(), got)
		}
	}
}

func TestUnknownFallback(t *testing.T) {
	const bogus = "zzz-not-a-real-enum-value"
	if ARMemoryTypeFromValue(bogus) != ARMemoryTypeUnknown ||
		ARSysInfoTypeFromValue(bogus) != ARSysInfoTypeUnknown ||
		ARWlanClientCountFromValue(bogus) != ARWlanClientCountUnknown {
		t.Error("no UNKNOWN fallback")
	}
}
