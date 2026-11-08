package common

import "testing"

func strp(s string) *string { return &s }

func TestFormatVendor(t *testing.T) {
	cases := []struct {
		vendor *string
		result *string
	}{
		// Prefix-coded values
		{strp("android-dhcp-13"), strp("Android 13")},
		// No version - no trailing space
		{strp("android-dhcp-"), strp("Android")},
		// Exact aliases
		{strp("MSFT 5.0"), strp("Microsoft Corporation")},
		// Passthrough
		{strp("Apple, Inc."), strp("Apple, Inc.")},
		// Empty and None
		{strp(""), strp("")},
		{nil, nil},
	}
	for _, c := range cases {
		got := FormatVendor(c.vendor)
		if (got == nil) != (c.result == nil) || (got != nil && *got != *c.result) {
			t.Errorf("FormatVendor(%v) = %v, want %v", c.vendor, got, c.result)
		}
	}
}
