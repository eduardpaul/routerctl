package platform

import "testing"

// Every member resolves from its own stored value.
func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARPlatformMembers {
		t.Run(m.Name(), func(t *testing.T) {
			if got := ARPlatformFromValue(string(m)); got != m {
				t.Errorf("FromValue(%q) = %v", m, got)
			}
		})
	}
}

// An unrecognised value falls back to UNKNOWN.
func TestUnknownFallback(t *testing.T) {
	if got := ARPlatformFromValue("zzz-not-a-real-enum-value"); got != ARPlatformUnknown {
		t.Errorf("got %v", got)
	}
}
