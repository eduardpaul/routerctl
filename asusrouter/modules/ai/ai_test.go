package ai

import "testing"

// TestValueRoundTrips mirrors test_value_round_trips.
func TestValueRoundTrips(t *testing.T) {
	for _, member := range ARAICapabilityMembers {
		if got := ARAICapabilityFromValue(string(member)); got != member {
			t.Fatalf("%s: FromValue(%q) = %v", member.Name(), string(member), got)
		}
	}
}

// TestUnknownFallback mirrors test_unknown_fallback.
func TestUnknownFallback(t *testing.T) {
	if got := ARAICapabilityFromValue("zzz-not-a-real-enum-value"); got != ARAICapabilityUnknown {
		t.Fatalf("FromValue = %v, want UNKNOWN", got)
	}
}
