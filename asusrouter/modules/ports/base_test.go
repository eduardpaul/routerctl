package ports

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
)

// caps builds a full capability dict with the given caps true, rest false.
func caps(trueCaps ...ARPortCapability) map[ARPortCapability]bool {
	set := map[ARPortCapability]bool{}
	for _, c := range trueCaps {
		set[c] = true
	}
	out := map[ARPortCapability]bool{}
	for _, c := range ARPortCapabilityMembers {
		if c >= 0 {
			out[c] = set[c]
		}
	}
	return out
}

func TestReadPortCapabilities(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want map[ARPortCapability]bool
	}{
		// Non-int and negative -> early return
		{"None", nil, map[ARPortCapability]bool{}},
		{"Str", "str", map[ARPortCapability]bool{}},
		{"Negative", -1, map[ARPortCapability]bool{}},
		// Zero -> all False
		{"Zero", 0, caps()},
		// Single capability bits
		{"Wan", 1, caps(ARPortCapabilityWan)},
		{"Lan", 2, caps(ARPortCapabilityLan)},
		{"Usb", 128, caps(ARPortCapabilityUsb)},
		{"DualwanPrimary", 1 << 30, caps(ARPortCapabilityDualwanPrimary)},
		// Multiple capability bits
		{"WanLan", 3, caps(ARPortCapabilityWan, ARPortCapabilityLan)},
		{"WanLanDualwan", (1 << 0) | (1 << 1) | (1 << 30), caps(ARPortCapabilityWan, ARPortCapabilityLan, ARPortCapabilityDualwanPrimary)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadPortCapabilities(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadPortCapabilities(%v) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestReadPortType(t *testing.T) {
	tests := []struct {
		name string
		caps map[ARPortCapability]bool
		want ARPortType
	}{
		// Empty dict -> UNKNOWN
		{"Empty", map[ARPortCapability]bool{}, ARPortTypeUnknown},
		// Each mapped capability
		{"Wan", map[ARPortCapability]bool{ARPortCapabilityWan: true}, ARPortTypeWan},
		{"Lan", map[ARPortCapability]bool{ARPortCapabilityLan: true}, ARPortTypeLan},
		{"Usb", map[ARPortCapability]bool{ARPortCapabilityUsb: true}, ARPortTypeUsb},
		{"Moca", map[ARPortCapability]bool{ARPortCapabilityMoca: true}, ARPortTypeMoca},
		// Order: WAN checked before LAN -> WAN wins
		{"WanBeforeLan", map[ARPortCapability]bool{ARPortCapabilityWan: true, ARPortCapabilityLan: true}, ARPortTypeWan},
		// Unmapped capability -> UNKNOWN
		{"Unmapped", map[ARPortCapability]bool{ARPortCapabilityGame: true}, ARPortTypeUnknown},
		// Mapped caps all False -> UNKNOWN
		{"AllFalse", map[ARPortCapability]bool{ARPortCapabilityWan: false, ARPortCapabilityLan: false}, ARPortTypeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadPortType(tt.caps); got != tt.want {
				t.Errorf("ReadPortType = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadPortSpeed(t *testing.T) {
	tests := []struct {
		name     string
		portType ARPortType
		raw      int
		want     any
	}{
		// USB branch
		{"UsbUsb3", ARPortTypeUsb, 5000, usb.ARUSBSpeedUsb3},
		{"UsbDown", ARPortTypeUsb, 0, usb.ARUSBSpeedDown},
		{"UsbUnknown", ARPortTypeUsb, 999, usb.ARUSBSpeedUnknown},
		// Ethernet branch - various non-USB port types
		{"Lan1000", ARPortTypeLan, 1000, ARPortSpeedMbps1000},
		{"WanDown", ARPortTypeWan, 0, ARPortSpeedDown},
		{"Ethernet100", ARPortTypeEthernet, 100, ARPortSpeedMbps100},
		{"Sfpp10000", ARPortTypeSfpp, 10000, ARPortSpeedMbps10000},
		{"LanUnknown", ARPortTypeLan, 999, ARPortSpeedUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadPortSpeed(tt.portType, tt.raw); got != tt.want {
				t.Errorf("ReadPortSpeed(%v, %d) = %#v, want %#v", tt.portType, tt.raw, got, tt.want)
			}
		})
	}
}
