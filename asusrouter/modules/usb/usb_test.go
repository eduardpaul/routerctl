package usb

import "testing"

var storageRaw = map[string]any{
	"type":     "storage",
	"manu":     "JetFlash",
	"product":  "Mass Storage Device",
	"serial":   "08MAGWFG85JV4MQ1",
	"speed":    "5000",
	"dev_name": "JetFlash Transcend 64GB",
	"size_kb":  "59038688",
	"used_kb":  "6226272",
}

var modemRaw = map[string]any{
	"type":    "modem",
	"manu":    "SAMSUNG",
	"product": "SAMSUNG_Android",
	"serial":  "RFGL116XG1B",
	"speed":   "5000",
}

var sentinelRaw = map[string]any{
	"type":     "storage",
	"manu":     "-1",
	"product":  "-1",
	"serial":   "-1",
	"speed":    "0",
	"dev_name": "TS-RDF5 SD Transcend",
	"size_kb":  "125026304",
	"used_kb":  "0",
}

func sp(s string) *string { return &s }

func strEq(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func intEq(a *int, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ip(i int) *int { return &i }

func TestARUSBSpeedFromValue(t *testing.T) {
	cases := []struct {
		value    int
		expected ARUSBSpeed
	}{
		{0, ARUSBSpeedDown},
		{480, ARUSBSpeedUsb2},
		{5000, ARUSBSpeedUsb3},
		{10000, ARUSBSpeedUsb31},
		{20000, ARUSBSpeedUsb32},
		{999, ARUSBSpeedUnknown},
		{-1, ARUSBSpeedUnknown},
	}
	for _, c := range cases {
		if got := ARUSBSpeedFromValue(c.value); got != c.expected {
			t.Errorf("ARUSBSpeedFromValue(%d) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestARUSBDeviceTypeFromValue(t *testing.T) {
	cases := []struct {
		value    any
		expected ARUSBDeviceType
	}{
		{"storage", ARUSBDeviceTypeStorage},
		{"modem", ARUSBDeviceTypeModem},
		{"other", ARUSBDeviceTypeUnknown},
		{nil, ARUSBDeviceTypeUnknown},
	}
	for _, c := range cases {
		if got := ARUSBDeviceTypeFromValue(c.value); got != c.expected {
			t.Errorf("ARUSBDeviceTypeFromValue(%v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestARUSBDeviceFromRawStorage(t *testing.T) {
	d := ARUSBDeviceFromRaw("2", storageRaw)
	if !strEq(d.Serial, sp("08MAGWFG85JV4MQ1")) || !strEq(d.ManufacturerRaw, sp("JetFlash")) ||
		d.Type != ARUSBDeviceTypeStorage || !strEq(d.Name, sp("JetFlash Transcend 64GB")) ||
		d.Speed != ARUSBSpeedUsb3 || !strEq(d.Position, sp("2")) ||
		!intEq(d.Size, ip(59038688*1024)) || !intEq(d.Used, ip(6226272*1024)) {
		t.Errorf("unexpected storage device: %+v", d)
	}
}

func TestARUSBDeviceFromRawModem(t *testing.T) {
	d := ARUSBDeviceFromRaw("1", modemRaw)
	if d.Type != ARUSBDeviceTypeModem || d.Name != nil || d.Size != nil || d.Used != nil {
		t.Errorf("unexpected modem device: %+v", d)
	}
}

func TestARUSBDeviceFromRawSentinel(t *testing.T) {
	d := ARUSBDeviceFromRaw("1.1", sentinelRaw)
	if d.Serial != nil || d.ManufacturerRaw != nil || d.Product != nil ||
		d.Speed != ARUSBSpeedDown || !strEq(d.Name, sp("TS-RDF5 SD Transcend")) {
		t.Errorf("unexpected sentinel device: %+v", d)
	}
}

func TestARUSBDeviceIdentity(t *testing.T) {
	a := ARUSBDevice{Serial: sp("X"), ManufacturerRaw: sp("Y"), Used: ip(1), Position: sp("1")}
	b := ARUSBDevice{Serial: sp("X"), ManufacturerRaw: sp("Y"), Used: ip(999), Position: sp("2.3")}
	if !a.Equal(b) || a.IdentityKey() != b.IdentityKey() {
		t.Error("same serial+manufacturer should match despite transient fields")
	}

	c := ARUSBDevice{Serial: sp("X"), ManufacturerRaw: sp("Y")}
	d := ARUSBDevice{Serial: sp("Z"), ManufacturerRaw: sp("Y")}
	if c.Equal(d) {
		t.Error("different serials should be different devices")
	}

	set := map[ARUSBDeviceKey]struct{}{}
	for _, dev := range []ARUSBDevice{
		{Serial: sp("X"), ManufacturerRaw: sp("Y"), Used: ip(1)},
		{Serial: sp("X"), ManufacturerRaw: sp("Y"), Used: ip(2)},
		{Serial: sp("Z"), ManufacturerRaw: sp("Y")},
	} {
		set[dev.IdentityKey()] = struct{}{}
	}
	if len(set) != 2 {
		t.Errorf("devices should group by identity, got %d groups", len(set))
	}
}

func TestEnumsRoundTrip(t *testing.T) {
	for _, m := range ARUSBCapabilityMembers {
		if ARUSBCapabilityFromValue(string(m)) != m {
			t.Errorf("ARUSBCapability %v does not round-trip", m)
		}
	}
	if ARUSBCapabilityFromValue("zzz-not-a-real-enum-value") != ARUSBCapabilityUnknown {
		t.Error("unknown fallback failed")
	}
}
