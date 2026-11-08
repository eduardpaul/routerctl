package common

import "testing"

func TestARStatusCode(t *testing.T) {
	// The keys match the device response fields
	if StatusCodeKey != "statusCode" {
		t.Errorf("StatusCodeKey = %q", StatusCodeKey)
	}
	if ModifyKey != "modify" {
		t.Errorf("ModifyKey = %q", ModifyKey)
	}
	if ARStatusCodeSuccess != "success" || ARStatusCodeFail != "fail" {
		t.Error("ARStatusCode values")
	}
	cases := []struct {
		value    any
		expected ARStatusCode
	}{
		{"success", ARStatusCodeSuccess},
		{"fail", ARStatusCodeFail},
		{"error", ARStatusCodeUnknown},
		{nil, ARStatusCodeUnknown},
	}
	for _, c := range cases {
		if got := ARStatusCodeFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
}
