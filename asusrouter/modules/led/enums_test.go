package led

import "testing"

// Ported from tests/modules/led/test_enums.py.

func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARLedFieldMembers {
		if got := ARLedFieldFromValue(string(m)); got != m {
			t.Errorf("%s: got %q", m.Name(), got)
		}
	}
}

func TestUnknownFallback(t *testing.T) {
	if got := ARLedFieldFromValue("zzz-not-a-real-enum-value"); got != ARLedFieldUnknown {
		t.Errorf("got %q", got)
	}
}
