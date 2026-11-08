package common

import "testing"

// Raw tokens (any case) resolve to their member, else UNKNOWN.
func TestARApiClientFromValue(t *testing.T) {
	cases := []struct {
		value    string
		expected ARApiClient
	}{
		{"app", ARApiClientApp},
		{"web", ARApiClientWeb},
		{"APP", ARApiClientApp},
		{"Web", ARApiClientWeb},
		{"other", ARApiClientUnknown},
	}
	for _, c := range cases {
		if got := ARApiClientFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%q) = %v, want %v", c.value, got, c.expected)
		}
	}
}
