package device

import (
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func id(src source.DataSource) source.ID { return source.IDOf(src) }

func intp(v int) *int { return &v }

func strp(s string) *string { return &s }

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func wifiCaps(units ...int) map[support.ARSupportType]any {
	return map[support.ARSupportType]any{
		support.ARSupportTypeWifiCapabilities: map[wifi.ARWiFiCapability]any{wifi.ARWiFiCapabilityUnits: units},
	}
}

// TestTranslateFirmware reads FW_MAJOR/MINOR/BUILD keys and builds
// ARFirmware.
func TestTranslateFirmware(t *testing.T) {
	cases := []struct {
		name  string
		data  IdentityData
		major *[4]int
		minor *int
		build *int
	}{
		{
			"stock_388",
			IdentityData{
				id(nvram.ARNvramTypeFwMajor): "3.0.0.4",
				id(nvram.ARNvramTypeFwMinor): 388,
				id(nvram.ARNvramTypeFwBuild): "40456_g8178ee0",
			},
			&[4]int{3, 0, 0, 4}, intp(388), intp(40456),
		},
		{
			"beta_102",
			IdentityData{
				id(nvram.ARNvramTypeFwMajor): "9.0.0.6",
				id(nvram.ARNvramTypeFwMinor): "102",
				id(nvram.ARNvramTypeFwBuild): "4856_g8178ee0",
			},
			&[4]int{9, 0, 0, 6}, intp(102), intp(4856),
		},
		{"empty", IdentityData{}, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fw := translateFirmware(tc.data)
			if fw == nil {
				t.Fatal("nil firmware")
			}
			if !reflect.DeepEqual(fw.Major(), tc.major) {
				t.Errorf("major = %v, want %v", fw.Major(), tc.major)
			}
			if !eqIntPtr(fw.Minor(), tc.minor) || !eqIntPtr(fw.Build(), tc.build) {
				t.Errorf("minor/build = %v/%v, want %v/%v", fw.Minor(), fw.Build(), tc.minor, tc.build)
			}
		})
	}
}

// TestTranslateWifi parses the bands string and WIFI_UNITS into the band
// map.
func TestTranslateWifi(t *testing.T) {
	bands := func(v string) IdentityData { return IdentityData{id(nvram.ARNvramTypeWirelessBands): v} }
	cases := []struct {
		name     string
		data     IdentityData
		support  map[support.ARSupportType]any
		expected map[wifi.ARWiFiBand]int
	}{
		{"two_bands", bands("2g1&#605g1"), wifiCaps(0, 1),
			map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1}},
		{"three_bands", bands("2g1&#605g1&#605g2"), wifiCaps(0, 1, 2),
			map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1, wifi.ARWiFiBandBand5g2: 2}},
		// Invalid band string raises ValueError in ARWiFiBand -> skipped
		{"invalid_band_skipped", bands("2g1&#60invalid_band&#605g1"), wifiCaps(0, 1, 2),
			map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 2}},
		// No WIFI_UNITS -> zip stops immediately
		{"no_units", bands("2g1&#605g1"), map[support.ARSupportType]any{}, map[wifi.ARWiFiBand]int{}},
		// Empty band string
		{"empty_bands", bands(""), wifiCaps(0, 1), map[wifi.ARWiFiBand]int{}},
		// Missing WIRELESS_BANDS key
		{"missing_bands", IdentityData{}, wifiCaps(0, 1), map[wifi.ARWiFiBand]int{}},
		// Fewer ids than bands -> zip stops early
		{"fewer_ids", bands("2g1&#605g1&#605g2"), wifiCaps(0),
			map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0}},
		// ARWiFiBand.UNKNOWN is a valid member ("unknown") but must be skipped
		{"unknown_skipped", bands("2g1&#60unknown&#605g1"), wifiCaps(0, 1, 2),
			map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateWifi(tc.data, tc.support).units
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}

// nband builds a payload with `wl{}_nband` values keyed by unit.
func nband(units map[int]string) IdentityData {
	out := IdentityData{}
	for unit, value := range units {
		out[id(nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWlNband, unit))] = value
	}
	return out
}

// TestNbandFallback: with no WIRELESS_BANDS, bands come from per-radio
// nband.
func TestNbandFallback(t *testing.T) {
	// RT-AC66U: 2.4GHz (nband 2) + 5GHz (nband 1)
	got := translateWifi(nband(map[int]string{0: "2", 1: "1"}), nil).units
	want := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestNbandFirstUnitWins: a repeated band keeps the first unit.
func TestNbandFirstUnitWins(t *testing.T) {
	got := translateWifi(nband(map[int]string{0: "1", 1: "1"}), nil).units
	want := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand5g1: 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestNbandUnknownSkipped: an unknown nband code is skipped.
func TestNbandUnknownSkipped(t *testing.T) {
	if got := translateWifi(nband(map[int]string{0: "99"}), nil).units; len(got) != 0 {
		t.Errorf("got %v, want {}", got)
	}
}

// TestBandsPreferredOverNband: WIRELESS_BANDS wins when present, ignoring
// nband.
func TestBandsPreferredOverNband(t *testing.T) {
	data := nband(map[int]string{0: "1"})
	data[id(nvram.ARNvramTypeWirelessBands)] = "2g1"
	got := translateWifi(data, wifiCaps(0)).units
	want := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestTranslateIdentityBase extracts MAC, model, model_original, serial
// from data.
func TestTranslateIdentityBase(t *testing.T) {
	cases := []struct {
		name                    string
		data                    IdentityData
		macAsus                 *string
		model, original, serial *string
	}{
		{
			"full",
			IdentityData{
				id(nvram.ARNvramTypeMac):           "AA:BB:CC:11:22:33",
				id(nvram.ARNvramTypeModel):         "RT-AX88U",
				id(nvram.ARNvramTypeModelOriginal): "RT-AX88U_V2",
				id(nvram.ARNvramTypeSerial):        "ABC123XYZ",
			},
			strp("AA:BB:CC:11:22:33"), strp("RT-AX88U"), strp("RT-AX88U_V2"), strp("ABC123XYZ"),
		},
		// Lowercase MAC is parsed and can round-trip as_asus()
		{
			"lowercase_mac",
			IdentityData{
				id(nvram.ARNvramTypeMac):           "aa:bb:cc:11:22:33",
				id(nvram.ARNvramTypeModel):         "RT-AX56U",
				id(nvram.ARNvramTypeModelOriginal): nil,
				id(nvram.ARNvramTypeSerial):        nil,
			},
			strp("AA:BB:CC:11:22:33"), strp("RT-AX56U"), nil, nil,
		},
		// Blank label MAC falls back to the LAN hardware address
		{
			"lan_fallback",
			IdentityData{
				id(nvram.ARNvramTypeMac):    "",
				id(nvram.ARNvramTypeMacLan): "AA:BB:CC:11:22:33",
				id(nvram.ARNvramTypeModel):  "RT-AC66U",
			},
			strp("AA:BB:CC:11:22:33"), strp("RT-AC66U"), nil, nil,
		},
		// Blank label and LAN MAC fall back to the WAN hardware address
		{
			"wan_fallback",
			IdentityData{
				id(nvram.ARNvramTypeMac):    "",
				id(nvram.ARNvramTypeMacLan): "",
				id(nvram.ARNvramTypeMacWan): "AA:BB:CC:11:22:44",
			},
			strp("AA:BB:CC:11:22:44"), nil, nil, nil,
		},
		// All missing -> all None
		{"empty", IdentityData{}, nil, nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mac, model, original, serial := translateIdentityBase(tc.data)
			if tc.macAsus == nil {
				if mac != nil {
					t.Errorf("mac = %v, want nil", mac)
				}
			} else if mac == nil || mac.AsAsus() != *tc.macAsus {
				t.Errorf("mac = %v, want %s", mac, *tc.macAsus)
			}
			if !eqStrPtr(model, tc.model) || !eqStrPtr(original, tc.original) || !eqStrPtr(serial, tc.serial) {
				t.Errorf("got %v %v %v", model, original, serial)
			}
		})
	}
}

// TestDefaults: a fresh instance has the expected default values.
func TestDefaults(t *testing.T) {
	identity := NewARDeviceIdentity()
	if identity.Brand() != consts.DefaultIdentityBrand {
		t.Errorf("brand = %q", identity.Brand())
	}
	if fw := identity.Firmware(); fw == nil || !fw.Equal(firmware.NewARFirmware(nil, nil, nil, nil, false)) {
		t.Errorf("firmware = %v", fw)
	}
	if identity.Mac() != nil || identity.Model() != nil || identity.ModelOriginal() != nil || identity.Serial() != nil {
		t.Error("expected nil mac/model/model_original/serial")
	}
	if len(identity.Support()) != 0 || len(identity.Wifi()) != 0 {
		t.Error("expected empty support/wifi")
	}
}

// TestProperties covers the property accessors.
func TestProperties(t *testing.T) {
	t.Run("brand", func(t *testing.T) {
		if NewARDeviceIdentity().Brand() != consts.DefaultIdentityBrand {
			t.Error("brand")
		}
	})
	t.Run("firmware", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		fw := firmware.NewARFirmware(&[4]int{3, 0, 0, 4}, intp(388), intp(8), nil, false)
		identity.firmware = fw
		if identity.Firmware() != fw {
			t.Error("firmware is not the stored value")
		}
	})
	t.Run("mac_model_serial", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		if identity.Mac() != nil || identity.Model() != nil || identity.ModelOriginal() != nil || identity.Serial() != nil {
			t.Error("expected nil")
		}
	})
	t.Run("support_wifi", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		if len(identity.Support()) != 0 || len(identity.Wifi()) != 0 {
			t.Error("expected empty")
		}
	})
	t.Run("wifi_by_unit", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.wifi.set(wifi.ARWiFiBandBand2g1, 0)
		identity.wifi.set(wifi.ARWiFiBandBand5g1, 1)
		want := map[int]wifi.ARWiFiBand{0: wifi.ARWiFiBandBand2g1, 1: wifi.ARWiFiBandBand5g1}
		if got := identity.WifiByUnit(); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		if got := identity.WifiBands(); !reflect.DeepEqual(got, []wifi.ARWiFiBand{wifi.ARWiFiBandBand2g1, wifi.ARWiFiBandBand5g1}) {
			t.Errorf("WifiBands = %v", got)
		}
	})
}

// TestBuildFullData: build populates all fields from a complete data dict.
func TestBuildFullData(t *testing.T) {
	supportData := wifiCaps(0, 1)
	data := IdentityData{
		id(support.ARSupportSourceUniversal): supportData,
		id(nvram.ARNvramTypeFwMajor):         "3.0.0.4",
		id(nvram.ARNvramTypeFwMinor):         388,
		id(nvram.ARNvramTypeFwBuild):         "40456_g8178ee0",
		id(nvram.ARNvramTypeMac):             "AA:BB:CC:11:22:33",
		id(nvram.ARNvramTypeModel):           "RT-AX88U",
		id(nvram.ARNvramTypeModelOriginal):   "RT-AX88U_V2",
		id(nvram.ARNvramTypeSerial):          "ABC123XYZ",
		id(nvram.ARNvramTypeWirelessBands):   "2g1&#605g1",
	}
	identity := ARDeviceIdentityBuild(data)

	// Python asserts `identity.support is support_data`; Go keeps a
	// shallow copy (recovery writes to it), so compare the content of the
	// fetched entry
	if !reflect.DeepEqual(identity.Support()[support.ARSupportTypeWifiCapabilities].(map[wifi.ARWiFiCapability]any)[wifi.ARWiFiCapabilityUnits], []int{0, 1}) {
		t.Errorf("support = %v", identity.Support())
	}
	fw := identity.Firmware()
	if !reflect.DeepEqual(fw.Major(), &[4]int{3, 0, 0, 4}) || !eqIntPtr(fw.Minor(), intp(388)) || !eqIntPtr(fw.Build(), intp(40456)) {
		t.Errorf("firmware = %v", fw)
	}
	if mac := identity.Mac(); mac == nil || mac.AsAsus() != "AA:BB:CC:11:22:33" {
		t.Errorf("mac = %v", mac)
	}
	if !eqStrPtr(identity.Model(), strp("RT-AX88U")) || !eqStrPtr(identity.ModelOriginal(), strp("RT-AX88U_V2")) || !eqStrPtr(identity.Serial(), strp("ABC123XYZ")) {
		t.Error("model/model_original/serial")
	}
	want := map[wifi.ARWiFiBand]int{wifi.ARWiFiBandBand2g1: 0, wifi.ARWiFiBandBand5g1: 1}
	if got := identity.Wifi(); !reflect.DeepEqual(got, want) {
		t.Errorf("wifi = %v", got)
	}
}

// TestBuildSupportFallsBackToEmpty: build treats a non-dict or None
// support value as empty dict.
func TestBuildSupportFallsBackToEmpty(t *testing.T) {
	for name, value := range map[string]any{"not_a_dict": "not_a_dict", "none": nil} {
		t.Run(name, func(t *testing.T) {
			identity := ARDeviceIdentityBuild(IdentityData{id(support.ARSupportSourceUniversal): value})
			// Recovery has nothing to recover from an empty identity
			if got := identity.Support(); len(got) != 0 {
				t.Errorf("support = %v", got)
			}
		})
	}
}

// TestBuildEmptyData: build with empty data produces an all-default
// identity.
func TestBuildEmptyData(t *testing.T) {
	identity := ARDeviceIdentityBuild(IdentityData{})
	if len(identity.Support()) != 0 {
		t.Errorf("support = %v", identity.Support())
	}
	if !identity.Firmware().Equal(firmware.NewARFirmware(nil, nil, nil, nil, false)) {
		t.Errorf("firmware = %v", identity.Firmware())
	}
	if identity.Mac() != nil || identity.Model() != nil || identity.ModelOriginal() != nil || identity.Serial() != nil {
		t.Error("expected nil")
	}
	if len(identity.Wifi()) != 0 {
		t.Error("expected empty wifi")
	}
}

// TestAimesh covers the live AiMesh topology on the identity.
func TestAimesh(t *testing.T) {
	t.Run("default_is_empty_topology", func(t *testing.T) {
		topo := NewARDeviceIdentity().Aimesh()
		if topo == nil || len(topo.Nodes) != 0 {
			t.Errorf("aimesh = %v", topo)
		}
	})
	t.Run("update_swaps_snapshot", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		topology := aimesh.NewARAiMeshTopology()
		identity.UpdateAimesh(topology)
		if identity.Aimesh() != topology {
			t.Error("not swapped")
		}
	})
}

func utc(y int, m time.Month, d, hh, mm, ss int) *time.Time {
	t := time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
	return &t
}

// TestBoottime covers the live boot time on the identity.
func TestBoottime(t *testing.T) {
	t.Run("default_is_none", func(t *testing.T) {
		if NewARDeviceIdentity().Boottime() != nil {
			t.Error("expected nil")
		}
	})
	t.Run("update_sets_boottime", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		boottime := utc(2026, 1, 1, 0, 0, 0)
		identity.UpdateBoottime(boottime)
		if got := identity.Boottime(); got == nil || !got.Equal(*boottime) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("moved_boottime_is_not_a_reboot", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.UpdateBoottime(utc(2026, 1, 1, 0, 0, 0))
		identity.UpdateBoottime(utc(2026, 1, 2, 0, 0, 0))
		if identity.Rebooted() {
			t.Error("rebooted")
		}
	})
	t.Run("mark_reboot_flags_reboot", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.MarkReboot()
		if !identity.Rebooted() {
			t.Error("not rebooted")
		}
	})
}

// TestDeviceTime covers the device's own clock on the identity.
func TestDeviceTime(t *testing.T) {
	t.Run("default_is_none", func(t *testing.T) {
		if NewARDeviceIdentity().DeviceTime() != nil {
			t.Error("expected nil")
		}
	})
	t.Run("update_sets_device_time", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		deviceTime := utc(2026, 7, 31, 10, 24, 4)
		identity.UpdateDeviceTime(deviceTime)
		if got := identity.DeviceTime(); got == nil || !got.Equal(*deviceTime) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("update_does_not_flag_a_reboot", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.UpdateDeviceTime(utc(2026, 1, 1, 0, 0, 0))
		identity.UpdateDeviceTime(utc(2027, 1, 1, 0, 0, 0))
		if identity.Rebooted() {
			t.Error("rebooted")
		}
	})
}

// TestUptime covers the live uptime, which is what detects a reboot.
func TestUptime(t *testing.T) {
	t.Run("default_is_none", func(t *testing.T) {
		if NewARDeviceIdentity().Uptime() != nil {
			t.Error("expected nil")
		}
	})
	t.Run("update_sets_uptime", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.UpdateUptime(intp(2131043))
		if !eqIntPtr(identity.Uptime(), intp(2131043)) {
			t.Errorf("got %v", identity.Uptime())
		}
	})
	for _, tc := range []struct {
		name          string
		first, second *int
	}{
		{"first_read", nil, intp(100)},
		{"unchanged", intp(100), intp(100)},
		{"counting_up", intp(100), intp(200)},
		{"gone", intp(100), nil},
	} {
		t.Run("not_a_reboot/"+tc.name, func(t *testing.T) {
			identity := NewARDeviceIdentity()
			identity.UpdateUptime(tc.first)
			identity.UpdateUptime(tc.second)
			if identity.Rebooted() {
				t.Error("rebooted")
			}
		})
	}
	t.Run("falling_uptime_flags_reboot", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		identity.UpdateUptime(intp(2131043))
		identity.UpdateUptime(intp(12))
		if !identity.Rebooted() {
			t.Error("not rebooted")
		}
		identity.ClearRebooted()
		if identity.Rebooted() {
			t.Error("still rebooted")
		}
	})
}

// TestTranslateOperationMode resolves the raw sw_mode and wlc_psta to the
// active mode.
func TestTranslateOperationMode(t *testing.T) {
	sw := id(nvram.ARNvramTypeSwMode)
	psta := id(nvram.ARNvramTypeWlcProxySta)
	re := id(nvram.ARNvramTypeReMode)
	cases := []struct {
		name     string
		data     IdentityData
		expected common.AROperationMode
	}{
		// No data -> unknown
		{"empty", IdentityData{}, common.AROperationModeUnknown},
		// Plain sw_mode values
		{"router", IdentityData{sw: 1}, common.AROperationModeRouter},
		{"repeater", IdentityData{sw: 2}, common.AROperationModeRepeater},
		{"ap", IdentityData{sw: 3}, common.AROperationModeAccessPoint},
		{"bridge", IdentityData{sw: 4}, common.AROperationModeMediaBridge},
		{"unknown", IdentityData{sw: 99}, common.AROperationModeUnknown},
		// Proxy-STA repurposes the hardware
		{"repeater_psta1", IdentityData{sw: 2, psta: 1}, common.AROperationModeMediaBridge},
		{"ap_psta1", IdentityData{sw: 3, psta: 1}, common.AROperationModeMediaBridge},
		{"ap_psta3", IdentityData{sw: 3, psta: 3}, common.AROperationModeMediaBridge},
		{"ap_psta2", IdentityData{sw: 3, psta: 2}, common.AROperationModeRepeater},
		// psta=3 only applies to an AP base, not a repeater base
		{"repeater_psta3", IdentityData{sw: 2, psta: 3}, common.AROperationModeRepeater},
		// String values from the device are handled
		{"strings", IdentityData{sw: "3", psta: "2"}, common.AROperationModeRepeater},
		// An AiMesh node runs as a repeater but re_mode overrides it
		{"aimesh_node", IdentityData{sw: 2, re: 1}, common.AROperationModeAimeshNode},
		{"aimesh_node_psta", IdentityData{sw: 3, psta: 2, re: 1}, common.AROperationModeAimeshNode},
		// re_mode 0 does not override
		{"re_mode_0", IdentityData{sw: 1, re: 0}, common.AROperationModeRouter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := translateOperationMode(tc.data); got != tc.expected {
				t.Errorf("got %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestOperationModeDefaultAndBuild: a fresh identity reports unknown;
// build() resolves the mode onto the identity.
func TestOperationModeDefaultAndBuild(t *testing.T) {
	if NewARDeviceIdentity().OperationMode() != common.AROperationModeUnknown {
		t.Error("default should be unknown")
	}
	identity := ARDeviceIdentityBuild(IdentityData{id(nvram.ARNvramTypeSwMode): 3})
	if identity.OperationMode() != common.AROperationModeAccessPoint {
		t.Errorf("got %v", identity.OperationMode())
	}
}

// TestUsername covers the login name injected into the identity.
func TestUsername(t *testing.T) {
	t.Run("absent_by_default", func(t *testing.T) {
		if NewARDeviceIdentity().Username() != nil {
			t.Error("expected nil")
		}
	})
	t.Run("updated", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		u := identifiers.NewUsername("fakeadmin")
		identity.UpdateUsername(&u)
		if got := identity.Username(); got == nil || got.Value() != "fakeadmin" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("cleared", func(t *testing.T) {
		identity := NewARDeviceIdentity()
		u := identifiers.NewUsername("fakeadmin")
		identity.UpdateUsername(&u)
		identity.UpdateUsername(nil)
		if identity.Username() != nil {
			t.Error("expected nil")
		}
	})
}

// TestConcurrentAccess exercises the identity under the race detector.
func TestConcurrentAccess(t *testing.T) {
	identity := NewARDeviceIdentity()
	done := make(chan struct{})
	for i := 0; i < 4; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 200; j++ {
				identity.UpdateUptime(intp(n*1000 + j))
				identity.UpdateBoottime(utc(2026, 1, 1, 0, 0, j%60))
				_ = identity.Rebooted()
				_ = identity.Wifi()
				_ = identity.Aimesh()
				identity.UpdateAimesh(aimesh.NewARAiMeshTopology())
				_ = identity.CanonicalValue()
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}
}
