package connection

import "testing"

// Every member resolves from its own stored value.
func TestValueRoundTrips(t *testing.T) {
	for _, member := range ARConnectionMembers {
		if got := ARConnectionFromValue(string(member)); got != member {
			t.Errorf("%s: got %v", member.Name(), got)
		}
	}
}

// An unrecognised value falls back to UNKNOWN.
func TestUnknownFallback(t *testing.T) {
	if got := ARConnectionFromValue("zzz-not-a-real-enum-value"); got != ARConnectionUnknown {
		t.Errorf("got %v", got)
	}
}
