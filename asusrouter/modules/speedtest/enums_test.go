package speedtest

import "testing"

func TestEnumsRoundTrip(t *testing.T) {
	for _, m := range ARSpeedTestCapabilityMembers {
		if ARSpeedTestCapabilityFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARSpeedTestEventTypeMembers {
		if ARSpeedTestEventTypeFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	for _, m := range ARSpeedTestStateMembers {
		if ARSpeedTestStateFromValue(string(m)) != m {
			t.Errorf("%s", m.Name())
		}
	}
	const bogus = "zzz-not-a-real-enum-value"
	if ARSpeedTestCapabilityFromValue(bogus) != ARSpeedTestCapabilityUnknown ||
		ARSpeedTestEventTypeFromValue(bogus) != ARSpeedTestEventTypeUnknown ||
		ARSpeedTestStateFromValue(bogus) != ARSpeedTestStateUnknown {
		t.Error("unknown fallback")
	}
}
