package common

import "testing"

func TestARConnectionTypeValues(t *testing.T) {
	if ARConnectionTypeWired != "wired" || ARConnectionTypeWireless != "wireless" {
		t.Error("ARConnectionType values")
	}
	if ARConnectionTypeFromValue("nope") != ARConnectionTypeUnknown {
		t.Error("unknown input should resolve to UNKNOWN")
	}
}

func TestARConnectionMethod(t *testing.T) {
	values := map[ARConnectionMethod]string{
		ARConnectionMethodDhcp:   "dhcp",
		ARConnectionMethodStatic: "static",
		ARConnectionMethodPppoe:  "pppoe",
		ARConnectionMethodPptp:   "pptp",
		ARConnectionMethodL2tp:   "l2tp",
	}
	for m, v := range values {
		if string(m) != v {
			t.Errorf("%v != %q", m, v)
		}
	}
	if ARConnectionMethodFromValue("nope") != ARConnectionMethodUnknown {
		t.Error("unknown input should resolve to UNKNOWN")
	}
}

func TestARConnectionState(t *testing.T) {
	if ARConnectionStateDisconnected != 0 || ARConnectionStateConnected != 1 {
		t.Error("ARConnectionState values")
	}
	if ARConnectionStateFromValue("1") != ARConnectionStateConnected {
		t.Error(`FromValue("1")`)
	}
	if ARConnectionStateFromValue(0) != ARConnectionStateDisconnected {
		t.Error("FromValue(0)")
	}
	if ARConnectionStateFromValue(99) != ARConnectionStateUnknown {
		t.Error("FromValue(99)")
	}
}

func TestARConnectionStatus(t *testing.T) {
	if ARConnectionStatusError != -1 || ARConnectionStatusDisconnected != 0 ||
		ARConnectionStatusConnecting != 1 || ARConnectionStatusConnected != 2 {
		t.Error("ARConnectionStatus values")
	}
	if ARConnectionStatusFromValue("2") != ARConnectionStatusConnected {
		t.Error(`FromValue("2")`)
	}
	if ARConnectionStatusFromValue(0) != ARConnectionStatusDisconnected {
		t.Error("FromValue(0)")
	}
	if ARConnectionStatusFromValue(99) != ARConnectionStatusUnknown {
		t.Error("FromValue(99)")
	}
	if ARConnectionStatusFromValue("nope") != ARConnectionStatusUnknown {
		t.Error(`FromValue("nope")`)
	}
}
