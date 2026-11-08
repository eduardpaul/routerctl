package ports

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const testMac = "12:34:56:78:9A:BC"

// fakeIdentity stands in for `ARDeviceIdentity` (package ports cannot
// import device in tests: device -> aimesh -> ports).
type fakeIdentity struct{ mac *identifiers.MacAddress }

func (f fakeIdentity) Mac() *identifiers.MacAddress { return f.mac }

func identityWith(mac string) fakeIdentity {
	if mac == "" {
		return fakeIdentity{}
	}
	return fakeIdentity{mac: identifiers.MacAddressFromValueSafe(mac)}
}

func mustMac(t *testing.T, s string) identifiers.MacAddress {
	t.Helper()
	m := identifiers.MacAddressFromValueSafe(s)
	if m == nil {
		t.Fatalf("bad MAC %q", s)
	}
	return *m
}

func portsByName(ports []map[ARPortProperty]any) map[string]map[ARPortProperty]any {
	out := map[string]map[ARPortProperty]any{}
	for _, p := range ports {
		out[p[ARPortPropertyNativeName].(string)] = p
	}
	return out
}

func TestReadEthernetPortSpeed(t *testing.T) {
	tests := []struct {
		code string
		want ARPortSpeed
	}{
		{"t", ARPortSpeedMbps10},
		{"X", ARPortSpeedDown},
		{"M", ARPortSpeedMbps100},
		{"G", ARPortSpeedMbps1000},
		{"Q", ARPortSpeedMbps2500},
		{"F", ARPortSpeedMbps5000},
		{"T", ARPortSpeedMbps10000},
		{"?", ARPortSpeedUnknown},
		{"", ARPortSpeedUnknown},
	}
	for _, tt := range tests {
		if got := ReadEthernetPortSpeed(tt.code); got != tt.want {
			t.Errorf("ReadEthernetPortSpeed(%q) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestLegacyNativeName(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  *string
	}{
		{"Wan", "WAN 0", strp("W0")},
		{"Lan", "LAN 1", strp("L1")},
		{"Lan8", "LAN 8", strp("L8")},
		{"Usb", "USB 1", strp("U1")},
		{"TenGig", "10G 1", strp("SFP")},
		{"TenGigNoIndex", "10G", strp("SFP")},
		{"UnknownPrefix", "XYZ 1", nil},
		{"NonIntIndex", "WAN x", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetWarned()
			if got := LegacyNativeName(tt.label); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LegacyNativeName(%q) = %v, want %v", tt.label, deref(got), deref(tt.want))
			}
		})
	}
}

func TestTranslateEthernetPorts(t *testing.T) {
	t.Run("TranslatesUnderIdentityMac", func(t *testing.T) {
		resetWarned()
		data := map[string]any{"portSpeed": map[string]any{"WAN 0": "G", "LAN 1": "G", "LAN 4": "X"}}

		result := TranslateEthernetPorts(data, identityWith(testMac))

		node, ok := result[mustMac(t, testMac)]
		if !ok || len(result) != 1 {
			t.Fatalf("result not keyed by the identity MAC: %v", result)
		}
		if len(node.Info) != 0 {
			t.Errorf("info = %v, want {}", node.Info)
		}
		ports := portsByName(node.Ports)
		want := map[ARPortProperty]any{
			ARPortPropertyNativeName: "W0",
			ARPortPropertyState:      true,
			ARPortPropertyLinkRate:   ARPortSpeedMbps1000,
			ARPortPropertyExtended:   false,
		}
		if !reflect.DeepEqual(ports["W0"], want) {
			t.Errorf("W0 = %v, want %v", ports["W0"], want)
		}
		if ports["L4"][ARPortPropertyState] != false {
			t.Errorf("L4 state = %v, want false", ports["L4"][ARPortPropertyState])
		}
		if ports["L4"][ARPortPropertyLinkRate] != ARPortSpeedDown {
			t.Errorf("L4 link rate = %v, want DOWN", ports["L4"][ARPortPropertyLinkRate])
		}
	})

	t.Run("SkipsUnknownLabels", func(t *testing.T) {
		resetWarned()
		data := map[string]any{"portSpeed": map[string]any{"LAN 1": "G", "XYZ 9": "G"}}

		result := TranslateEthernetPorts(data, identityWith(testMac))

		ports := portsByName(result[mustMac(t, testMac)].Ports)
		if len(ports) != 1 || ports["L1"] == nil {
			t.Errorf("ports = %v, want only L1", ports)
		}
	})

	for _, tt := range []struct {
		name string
		data map[string]any
	}{
		{"EmptyInputReturnsEmpty/Empty", map[string]any{}},
		{"EmptyInputReturnsEmpty/EmptySpeed", map[string]any{"portSpeed": map[string]any{}}},
		{"EmptyInputReturnsEmpty/NoneSpeed", map[string]any{"portSpeed": nil}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := TranslateEthernetPorts(tt.data, identityWith(testMac)); len(got) != 0 {
				t.Errorf("got %v, want {}", got)
			}
		})
	}

	t.Run("NoMacReturnsEmpty", func(t *testing.T) {
		data := map[string]any{"portSpeed": map[string]any{"LAN 1": "G"}}
		if got := TranslateEthernetPorts(data, identityWith("")); len(got) != 0 {
			t.Errorf("got %v, want {}", got)
		}
	})

	t.Run("OrderFollowsRouterOrder", func(t *testing.T) {
		// Go addition: Python keeps the JSON document order (WAN, LAN by
		// index); Go rebuilds it with legacyLabelOrder
		data := map[string]any{"portSpeed": map[string]any{"LAN 2": "G", "LAN 1": "G", "WAN 0": "G"}}
		node := TranslateEthernetPorts(data, identityWith(testMac))[mustMac(t, testMac)]
		var names []string
		for _, p := range node.Ports {
			names = append(names, p[ARPortPropertyNativeName].(string))
		}
		if want := []string{"W0", "L1", "L2"}; !reflect.DeepEqual(names, want) {
			t.Errorf("order = %v, want %v", names, want)
		}
	})
}

func strp(s string) *string { return &s }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
