package ports

import "testing"

func TestEnumValueRoundTrips(t *testing.T) {
	for _, m := range ARPortCablePairMembers {
		if got := ARPortCablePairFromValue(string(m)); got != m {
			t.Errorf("ARPortCablePair %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARPortCapabilityMembers {
		if got := ARPortCapabilityFromValue(int(m)); got != m {
			t.Errorf("ARPortCapability %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARPortPropertyMembers {
		if got := ARPortPropertyFromValue(string(m)); got != m {
			t.Errorf("ARPortProperty %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARPortSpeedMembers {
		if got := ARPortSpeedFromValue(int(m)); got != m {
			t.Errorf("ARPortSpeed %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARPortsInfoMembers {
		if got := ARPortsInfoFromValue(string(m)); got != m {
			t.Errorf("ARPortsInfo %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARPortTypeMembers {
		if got := ARPortTypeFromValue(string(m)); got != m {
			t.Errorf("ARPortType %s: got %v", m.Name(), got)
		}
	}
}

func TestEnumUnknownFallback(t *testing.T) {
	const bad = "zzz-not-a-real-enum-value"
	if ARPortCablePairFromValue(bad) != ARPortCablePairUnknown ||
		ARPortCapabilityFromValue(bad) != ARPortCapabilityUnknown ||
		ARPortPropertyFromValue(bad) != ARPortPropertyUnknown ||
		ARPortSpeedFromValue(bad) != ARPortSpeedUnknown ||
		ARPortsInfoFromValue(bad) != ARPortsInfoUnknown ||
		ARPortTypeFromValue(bad) != ARPortTypeUnknown {
		t.Error("an unrecognised value must fall back to UNKNOWN")
	}
}

func TestARPortSpeedFromValue(t *testing.T) {
	tests := []struct {
		value int
		want  ARPortSpeed
	}{
		{0, ARPortSpeedDown},
		{10, ARPortSpeedMbps10},
		{100, ARPortSpeedMbps100},
		{1000, ARPortSpeedMbps1000},
		{2500, ARPortSpeedMbps2500},
		{5000, ARPortSpeedMbps5000},
		{10000, ARPortSpeedMbps10000},
		{999, ARPortSpeedUnknown},
		{-1, ARPortSpeedUnknown},
	}
	for _, tt := range tests {
		if got := ARPortSpeedFromValue(tt.value); got != tt.want {
			t.Errorf("ARPortSpeedFromValue(%d) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
