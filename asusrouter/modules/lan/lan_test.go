package lan

import "testing"

// Every member resolves from its own stored value.
func TestValueRoundTrips(t *testing.T) {
	for _, member := range ARLANCapabilityMembers {
		if got := ARLANCapabilityFromValue(string(member)); got != member {
			t.Errorf("%s: got %v", member.Name(), got)
		}
	}
}

// An unrecognised value falls back to UNKNOWN.
func TestUnknownFallback(t *testing.T) {
	if got := ARLANCapabilityFromValue("zzz-not-a-real-enum-value"); got != ARLANCapabilityUnknown {
		t.Errorf("got %v", got)
	}
}
