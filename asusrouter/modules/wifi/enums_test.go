package wifi

import "testing"

func TestEnumValueRoundTrips(t *testing.T) {
	for _, m := range ARWiFiAuthMembers {
		if got := ARWiFiAuthFromValue(string(m)); got != m {
			t.Errorf("ARWiFiAuth %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiAuthModeMembers {
		if got := ARWiFiAuthModeFromValue(string(m)); got != m {
			t.Errorf("ARWiFiAuthMode %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiBandMembers {
		if got := ARWiFiBandFromValue(string(m)); got != m {
			t.Errorf("ARWiFiBand %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiBandwidthMembers {
		if got := ARWiFiBandwidthFromValue(int(m)); got != m {
			t.Errorf("ARWiFiBandwidth %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiFieldMembers {
		if got := ARWiFiFieldFromValue(string(m)); got != m {
			t.Errorf("ARWiFiField %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiFrequencyMembers {
		if got := ARWiFiFrequencyFromValue(string(m)); got != m {
			t.Errorf("ARWiFiFrequency %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiGenerationMembers {
		if got := ARWiFiGenerationFromValue(int(m)); got != m {
			t.Errorf("ARWiFiGeneration %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiMacFilterModeMembers {
		if got := ARWiFiMacFilterModeFromValue(string(m)); got != m {
			t.Errorf("ARWiFiMacFilterMode %s: got %v", m.Name(), got)
		}
	}
	for _, m := range ARWiFiMultiBandMembers {
		if got := ARWiFiMultiBandFromValue(int(m)); got != m {
			t.Errorf("ARWiFiMultiBand %s: got %v", m.Name(), got)
		}
	}
}

func TestEnumUnknownFallback(t *testing.T) {
	const bad = "zzz-not-a-real-enum-value"
	if ARWiFiAuthFromValue(bad) != ARWiFiAuthUnknown ||
		ARWiFiAuthModeFromValue(bad) != ARWiFiAuthModeUnknown ||
		ARWiFiBandFromValue(bad) != ARWiFiBandUnknown ||
		ARWiFiBandwidthFromValue(bad) != ARWiFiBandwidthUnknown ||
		ARWiFiFieldFromValue(bad) != ARWiFiFieldUnknown ||
		ARWiFiFrequencyFromValue(bad) != ARWiFiFrequencyUnknown ||
		ARWiFiGenerationFromValue(bad) != ARWiFiGenerationUnknown ||
		ARWiFiMacFilterModeFromValue(bad) != ARWiFiMacFilterModeUnknown ||
		ARWiFiMultiBandFromValue(bad) != ARWiFiMultiBandUnknown {
		t.Error("an unrecognised value must fall back to UNKNOWN")
	}
}

func TestARWiFiBandFromNband(t *testing.T) {
	str := func(s string) *string { return &s }
	tests := []struct {
		name string
		code *string
		want ARWiFiBand
	}{
		{"5g", str("1"), ARWiFiBandBand5g1},
		{"2g", str("2"), ARWiFiBandBand2g1},
		{"6g", str("4"), ARWiFiBandBand6g1},
		{"Unmapped", str("9"), ARWiFiBandUnknown},
		{"Empty", str(""), ARWiFiBandUnknown},
		{"None", nil, ARWiFiBandUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ARWiFiBandFromNband(tt.code); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
