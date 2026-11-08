package common

import "testing"

func TestARMetricType(t *testing.T) {
	// USAGE maps to its string value
	if ARMetricTypeUsage != "usage" {
		t.Error("USAGE value")
	}
	cases := []struct {
		value    any
		expected ARMetricType
	}{
		{"usage", ARMetricTypeUsage},
		{"other", ARMetricTypeUnknown},
		{nil, ARMetricTypeUnknown},
	}
	for _, c := range cases {
		if got := ARMetricTypeFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
}
