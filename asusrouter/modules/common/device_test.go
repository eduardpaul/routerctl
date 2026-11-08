package common

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

// Known codes resolve; unknown / special codes fall back.
func TestARDeviceTypeFromValue(t *testing.T) {
	cases := []struct {
		name     string
		value    any
		expected ARDeviceType
	}{
		{"int", 5, ARDeviceTypeIpCam},
		{"str", "9", ARDeviceTypeAndroidPhone},
		{"special", 137, ARDeviceTypeUnknown},
		{"none", nil, ARDeviceTypeUnknown},
		{"bad", "nope", ARDeviceTypeUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ARDeviceTypeFromValue(c.value); got != c.expected {
				t.Errorf("got %v, want %v", got, c.expected)
			}
		})
	}
}

// Members exist and carry the expected integer values.
func TestAROperationModeMembersAndValues(t *testing.T) {
	cases := []struct {
		name   string
		member AROperationMode
		value  int
	}{
		{"UNKNOWN", AROperationModeUnknown, consts.UnknownMember},
		{"ROUTER", AROperationModeRouter, 1},
		{"REPEATER", AROperationModeRepeater, 2},
		{"ACCESS_POINT", AROperationModeAccessPoint, 3},
		{"MEDIA_BRIDGE", AROperationModeMediaBridge, 4},
		{"AIMESH_NODE", AROperationModeAimeshNode, 5},
	}
	for _, c := range cases {
		if c.member.Name() != c.name || int(c.member) != c.value {
			t.Errorf("%s: name %q value %d", c.name, c.member.Name(), int(c.member))
		}
	}
}

// Every member resolves from its own stored value.
func TestAROperationModeValueRoundTrip(t *testing.T) {
	for _, m := range AROperationModeMembers {
		if got := AROperationModeFromValue(int(m)); got != m {
			t.Errorf("FromValue(%d) = %v", int(m), got)
		}
	}
}

// An unrecognised code falls back to UNKNOWN.
func TestAROperationModeUnknownFallback(t *testing.T) {
	if AROperationModeFromValue(99) != AROperationModeUnknown {
		t.Error("FromValue(99)")
	}
}
