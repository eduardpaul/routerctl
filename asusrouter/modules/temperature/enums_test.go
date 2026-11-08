package temperature

import "testing"

// Ported from tests/modules/temperature/test_enums.py.

func TestValueRoundTrips(t *testing.T) {
	for _, m := range ARTemperatureTypeMembers {
		if got := ARTemperatureTypeFromValue(string(m)); got != m {
			t.Errorf("%s: got %q", m.Name(), got)
		}
	}
}

func TestUnknownFallback(t *testing.T) {
	if got := ARTemperatureTypeFromValue("zzz-not-a-real-enum-value"); got != ARTemperatureTypeUnknown {
		t.Errorf("got %q", got)
	}
}
