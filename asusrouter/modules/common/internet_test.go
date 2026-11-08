package common

import "testing"

func TestARInternetMode(t *testing.T) {
	if ARInternetModeAllow != "allow" || ARInternetModeBlock != "block" || ARInternetModeTime != "time" {
		t.Error("ARInternetMode values")
	}
	if ARInternetModeFromValue("nope") != ARInternetModeUnknown {
		t.Error("unknown input should resolve to UNKNOWN")
	}
}
