package ports

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/usb"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func TestNativeName(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  *string
	}{
		{"Ai", "AI", strp("AI")},
		{"Lan", "L1", strp("L1")},
		{"Wan", "W0", strp("W0")},
		{"Usb", "U1", strp("U1")},
		{"TwoDigits", "L10", strp("L10")},
		{"UnknownPrefix", "X5", nil},
		{"NoIndex", "L", nil},
		{"NonDigitIndex", "LANX", nil},
		{"Empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetWarned()
			if got := NativeName(tt.label); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NativeName(%q) = %v, want %v", tt.label, deref(got), deref(tt.want))
			}
		})
	}
}

func TestReadRole(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		baseRole ARPortType
		caps     map[ARPortCapability]bool
		maxRate  any
		want     ARPortType
	}{
		{"AiLabelWins", "AI", ARPortTypeLan, caps(ARPortCapabilityLan), ARPortSpeedMbps1000, ARPortTypeAi},
		{"Usb", "U1", ARPortTypeUsb, caps(ARPortCapabilityUsb), usb.ARUSBSpeedUsb3, ARPortTypeUsb},
		{"Lan", "L1", ARPortTypeLan, caps(ARPortCapabilityLan), ARPortSpeedMbps1000, ARPortTypeLan},
		{"Wan", "W0", ARPortTypeWan, caps(ARPortCapabilityWan), ARPortSpeedMbps1000, ARPortTypeWan},
		{"SfppOverride", "L1", ARPortTypeLan, caps(ARPortCapabilityLan, ARPortCapabilitySfpp), ARPortSpeedMbps10000, ARPortTypeSfpp},
		{"NoSfppCapStaysLan", "L1", ARPortTypeLan, caps(ARPortCapabilityLan), ARPortSpeedMbps10000, ARPortTypeLan},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadRole(tt.label, tt.baseRole, tt.caps, tt.maxRate); got != tt.want {
				t.Errorf("ReadRole = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadCable(t *testing.T) {
	t.Run("CollapsesAllPairs", func(t *testing.T) {
		values := map[string]any{
			"brown": "1", "brown_len": "2",
			"blue": "3", "blue_len": "4",
			"green": "5", "green_len": "6",
			"orange": "7", "orange_len": "8",
		}
		cable := ReadCable(values)
		if cable[ARPortCablePairBrown] != (ARPortCableState{1, 2}) {
			t.Errorf("brown = %v", cable[ARPortCablePairBrown])
		}
		if cable[ARPortCablePairOrange] != (ARPortCableState{7, 8}) {
			t.Errorf("orange = %v", cable[ARPortCablePairOrange])
		}
		if len(cable) != 4 {
			t.Errorf("len = %d, want 4", len(cable))
		}
	})
	t.Run("MissingPairsAbsent", func(t *testing.T) {
		cable := ReadCable(map[string]any{"brown": "1", "brown_len": "2"})
		if _, ok := cable[ARPortCablePairBrown]; !ok || len(cable) != 1 {
			t.Errorf("cable = %v, want only brown", cable)
		}
	})
	t.Run("NoCableReturnsEmpty", func(t *testing.T) {
		if cable := ReadCable(map[string]any{"is_on": "1"}); len(cable) != 0 {
			t.Errorf("cable = %v, want {}", cable)
		}
	})
}

func TestReadDevices(t *testing.T) {
	t.Run("ParsesConnectedDevices", func(t *testing.T) {
		values := map[string]any{
			"devices": map[string]any{
				"1": map[string]any{"type": "modem", "manu": "SAMSUNG", "serial": "RF1"},
				"2": map[string]any{"type": "storage", "manu": "JetFlash", "serial": "JF1"},
			},
		}
		devices := ReadDevices(values)
		if len(devices) != 2 {
			t.Fatalf("len = %d, want 2", len(devices))
		}
		bySerial := map[string]usb.ARUSBDevice{}
		for _, d := range devices {
			if d.Serial != nil {
				bySerial[*d.Serial] = d
			}
		}
		if bySerial["RF1"].Type != usb.ARUSBDeviceTypeModem {
			t.Errorf("RF1 type = %v", bySerial["RF1"].Type)
		}
		if p := bySerial["RF1"].Position; p == nil || *p != "1" {
			t.Errorf("RF1 position = %v", p)
		}
		if bySerial["JF1"].Type != usb.ARUSBDeviceTypeStorage {
			t.Errorf("JF1 type = %v", bySerial["JF1"].Type)
		}
	})
	for _, tt := range []struct {
		name   string
		values map[string]any
	}{
		{"NoDevices/NoField", map[string]any{}},
		{"NoDevices/None", map[string]any{"devices": nil}},
		{"NoDevices/Empty", map[string]any{"devices": map[string]any{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadDevices(tt.values); len(got) != 0 {
				t.Errorf("got %v, want []", got)
			}
		})
	}
}

func TestReadNodeInfo(t *testing.T) {
	t.Run("ParsesAllFields", func(t *testing.T) {
		info := ReadNodeInfo(map[string]any{
			"cd_good_to_go":        "1",
			"power_limit":          "-1",
			"power_remain":         "30",
			"per_port_power_limit": "15",
		})
		want := map[ARPortsInfo]any{
			ARPortsInfoCdGoodToGo:        true,
			ARPortsInfoPowerLimit:        -1,
			ARPortsInfoPowerRemain:       30,
			ARPortsInfoPerPortPowerLimit: 15,
		}
		if !reflect.DeepEqual(info, want) {
			t.Errorf("info = %v, want %v", info, want)
		}
	})
	t.Run("PartialFields", func(t *testing.T) {
		info := ReadNodeInfo(map[string]any{"cd_good_to_go": "0"})
		if want := (map[ARPortsInfo]any{ARPortsInfoCdGoodToGo: false}); !reflect.DeepEqual(info, want) {
			t.Errorf("info = %v, want %v", info, want)
		}
	})
	t.Run("Empty", func(t *testing.T) {
		if info := ReadNodeInfo(map[string]any{}); len(info) != 0 {
			t.Errorf("info = %v, want {}", info)
		}
	})
}

func TestReadPort(t *testing.T) {
	t.Run("FullLanPort", func(t *testing.T) {
		values := map[string]any{
			"is_on": "1", "cap": "2", "max_rate": "1000", "link_rate": "1000",
			"seq_no": "0", "ui_display": "Port 4", "flag": "0", "phy_port_id": "3",
			"ext_port_id": "0", "ifname": "eth4",
			"brown": "0", "brown_len": "0", "blue": "0", "blue_len": "0",
			"green": "0", "green_len": "0", "orange": "0", "orange_len": "0",
			"linkrecover": "1", "timeout": "1",
		}
		port := ReadPort("L4", values)
		checks := map[ARPortProperty]any{
			ARPortPropertyNativeName:  "L4",
			ARPortPropertyState:       true,
			ARPortPropertyLinkRate:    ARPortSpeedMbps1000,
			ARPortPropertyMaxRate:     ARPortSpeedMbps1000,
			ARPortPropertyRole:        ARPortTypeLan,
			ARPortPropertyExtended:    true,
			ARPortPropertyIfname:      "eth4",
			ARPortPropertyUiDisplay:   "Port 4",
			ARPortPropertyPhyPortId:   3,
			ARPortPropertyFlag:        false,
			ARPortPropertyLinkRecover: true,
		}
		for prop, want := range checks {
			if got := port[prop]; got != want {
				t.Errorf("%s = %#v, want %#v", prop, got, want)
			}
		}
		if got := port[ARPortPropertyCapabilities]; !reflect.DeepEqual(got, []ARPortCapability{ARPortCapabilityLan}) {
			t.Errorf("capabilities = %v, want [LAN]", got)
		}
		if _, ok := port[ARPortPropertyCable]; !ok {
			t.Error("cable missing")
		}
	})
	t.Run("UsbPortSubset", func(t *testing.T) {
		values := map[string]any{
			"is_on": "0", "cap": "128", "max_rate": "5000", "link_rate": "0",
			"seq_no": "0", "ui_display": "", "flag": "0",
		}
		port := ReadPort("U1", values)
		if port[ARPortPropertyRole] != ARPortTypeUsb {
			t.Errorf("role = %v", port[ARPortPropertyRole])
		}
		if port[ARPortPropertyMaxRate] != usb.ARUSBSpeedUsb3 {
			t.Errorf("max rate = %#v", port[ARPortPropertyMaxRate])
		}
		if port[ARPortPropertyLinkRate] != usb.ARUSBSpeedDown {
			t.Errorf("link rate = %#v", port[ARPortPropertyLinkRate])
		}
		for _, prop := range []ARPortProperty{ARPortPropertyCable, ARPortPropertyIfname, ARPortPropertyPhyPortId, ARPortPropertyDevices} {
			if _, ok := port[prop]; ok {
				t.Errorf("%s present, want absent", prop)
			}
		}
	})
	t.Run("UsbPortWithDevices", func(t *testing.T) {
		values := map[string]any{
			"is_on": "0", "cap": "128", "max_rate": "5000", "link_rate": "0",
			"devices": map[string]any{"2": map[string]any{"type": "storage", "serial": "JF1"}},
		}
		port := ReadPort("U1", values)
		devices, _ := port[ARPortPropertyDevices].([]usb.ARUSBDevice)
		if len(devices) != 1 {
			t.Fatalf("devices = %v, want 1", port[ARPortPropertyDevices])
		}
		if s := devices[0].Serial; s == nil || *s != "JF1" {
			t.Errorf("serial = %v, want JF1", s)
		}
	})
}

func nativeNames(ports []map[ARPortProperty]any) []string {
	out := []string{}
	for _, p := range ports {
		out = append(out, p[ARPortPropertyNativeName].(string))
	}
	return out
}

func TestTranslatePortStatus(t *testing.T) {
	t.Run("TranslatesFullPayload", func(t *testing.T) {
		data := map[string]any{
			"node_info": map[string]any{"12:34:56:78:9A:BC": map[string]any{"cd_good_to_go": "1"}},
			"port_info": map[string]any{
				"12:34:56:78:9A:BC": map[string]any{
					"W0": map[string]any{"is_on": "1", "cap": "1", "max_rate": "1000", "link_rate": "1000"},
					"L1": map[string]any{"is_on": "0", "cap": "2", "max_rate": "1000", "link_rate": "0"},
				},
			},
		}
		result := TranslatePortStatus(data, fakeIdentity{})
		node := result[mustMac(t, "12:34:56:78:9A:BC")]
		if want := (map[ARPortsInfo]any{ARPortsInfoCdGoodToGo: true}); !reflect.DeepEqual(node.Info, want) {
			t.Errorf("info = %v, want %v", node.Info, want)
		}
		// Python asserts a set; Go additionally keeps the router order
		if got := nativeNames(node.Ports); !reflect.DeepEqual(got, []string{"W0", "L1"}) {
			t.Errorf("ports = %v, want [W0 L1]", got)
		}
		if role := portsByName(node.Ports)["W0"][ARPortPropertyRole]; role != ARPortTypeWan {
			t.Errorf("W0 role = %v, want WAN", role)
		}
	})
	t.Run("KeysAreMacAddresses", func(t *testing.T) {
		data := map[string]any{
			"node_info": map[string]any{},
			"port_info": map[string]any{"12:34:56:78:9A:BC": map[string]any{"W0": map[string]any{"is_on": "1"}}},
		}
		result := TranslatePortStatus(data, fakeIdentity{})
		// The map key type is identifiers.MacAddress by construction
		var _ map[identifiers.MacAddress]ARPortsData = result
		if len(result) != 1 {
			t.Errorf("result = %v, want one node", result)
		}
	})
	t.Run("SkipsUnknownLabels", func(t *testing.T) {
		resetWarned()
		data := map[string]any{
			"node_info": map[string]any{},
			"port_info": map[string]any{"AA:BB:CC:DD:EE:FF": map[string]any{"ZZ9": map[string]any{"is_on": "1"}}},
		}
		result := TranslatePortStatus(data, fakeIdentity{})
		node, ok := result[mustMac(t, "AA:BB:CC:DD:EE:FF")]
		if !ok || len(node.Ports) != 0 {
			t.Errorf("node = %v, want present with no ports", node)
		}
	})
	t.Run("SkipsMalformedMac", func(t *testing.T) {
		data := map[string]any{
			"node_info": map[string]any{},
			"port_info": map[string]any{"not-a-mac": map[string]any{"W0": map[string]any{"is_on": "1"}}},
		}
		if result := TranslatePortStatus(data, fakeIdentity{}); len(result) != 0 {
			t.Errorf("result = %v, want {}", result)
		}
	})
	t.Run("NodeInfoOnlyMac", func(t *testing.T) {
		data := map[string]any{
			"node_info": map[string]any{"AA:BB:CC:DD:EE:FF": map[string]any{"power_limit": "5"}},
			"port_info": map[string]any{},
		}
		node, ok := TranslatePortStatus(data, fakeIdentity{})[mustMac(t, "AA:BB:CC:DD:EE:FF")]
		if !ok || len(node.Ports) != 0 {
			t.Errorf("node = %v, want present with no ports", node)
		}
		if want := (map[ARPortsInfo]any{ARPortsInfoPowerLimit: 5}); !reflect.DeepEqual(node.Info, want) {
			t.Errorf("info = %v, want %v", node.Info, want)
		}
	})
	t.Run("LabelOrder", func(t *testing.T) {
		// Go addition: the router lists W*, L*, U*, then AI; Go rebuilds
		// that document order (portLabelOrder) since maps lose it
		data := map[string]any{
			"port_info": map[string]any{
				"12:34:56:78:9A:BC": map[string]any{
					"U1": map[string]any{}, "L10": map[string]any{}, "L2": map[string]any{},
					"AI": map[string]any{}, "W0": map[string]any{},
				},
			},
		}
		node := TranslatePortStatus(data, fakeIdentity{})[mustMac(t, "12:34:56:78:9A:BC")]
		if got, want := nativeNames(node.Ports), []string{"W0", "L2", "L10", "U1", "AI"}; !reflect.DeepEqual(got, want) {
			t.Errorf("order = %v, want %v", got, want)
		}
	})
}
