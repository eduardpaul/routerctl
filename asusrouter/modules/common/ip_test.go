package common

import "testing"

func TestARIPMethod(t *testing.T) {
	if ARIPMethodDhcp != "dhcp" || ARIPMethodManual != "manual" {
		t.Error("ARIPMethod values")
	}
	if ARIPMethodFromValue("nope") != ARIPMethodUnknown {
		t.Error("unknown input should resolve to UNKNOWN")
	}
}
