package aimesh

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/ports"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	tRoot  = "AA:00:00:00:00:00"
	tWired = "BB:00:00:00:00:00"
	tWL    = "CC:00:00:00:00:00"
	tAP5G  = "AA:00:00:00:00:14"
)

func mac(t *testing.T, s string) identifiers.MacAddress {
	t.Helper()
	m, err := identifiers.MacAddressFromValue(s)
	if err != nil {
		t.Fatalf("bad MAC %q: %v", s, err)
	}
	return m
}

func macIs(t *testing.T, got *identifiers.MacAddress, want string) {
	t.Helper()
	if got == nil || *got != mac(t, want) {
		t.Fatalf("MAC: got %v, want %s", got, want)
	}
}

// rootRaw builds a root (CAP) node.
func rootRaw() map[string]any {
	return map[string]any{
		"mac":             tRoot,
		"alias":           "Root",
		"model_name":      "M1",
		"ui_model_name":   "M1 UI",
		"product_id":      "P1",
		"icon_model_name": "ICON1",
		"tcode":           "US/01",
		"ip":              "192.168.1.1",
		"fwver":           "fw1",
		"newfwver":        "fw2",
		"online":          "1",
		"level":           "0",
		"re_path":         "0",
		"ap2g":            "AA:00:00:00:00:10",
		"ap2g_fh":         "AA:00:00:00:00:11",
		"ap2g_iot_fh":     "AA:00:00:00:00:12",
		"ap2g_ssid":       "S2",
		"ap2g_ssid_fh":    "S2FH",
		"ap5g":            tAP5G,
		"ap5g_fh":         "AA:00:00:00:00:15",
		"ap6g":            "AA:00:00:00:00:18",
		"apdwb":           "AA:00:00:00:00:18",
		"dwb_band":        "2",
		"band_info": map[string]any{
			"0": map[string]any{"unit": int64(0)},
			"1": map[string]any{"unit": int64(1)},
			"4": map[string]any{"unit": int64(2)},
		},
		"wired_mac": []any{tWired},
		"capability": map[string]any{
			"1": int64(95),
			"2": "1",
			"9": "1",
			"26": map[string]any{
				"wifi_band": map[string]any{
					"band1": map[string]any{
						"band":  "1",
						"count": "2",
						"vif": map[string]any{
							"wl0.1": map[string]any{"type": "1", "prefix": "wl0.1"},
							"wl0.4": map[string]any{"type": "8", "prefix": "wl0.4"},
						},
					},
				},
			},
			"27": map[string]any{
				"lan_port": map[string]any{
					"lan1": map[string]any{"label_name": "L1", "max_rate": "2500", "index": "1"},
				},
			},
			"28": map[string]any{
				"wan_port": map[string]any{"wan1": map[string]any{"label_name": "W0", "max_rate": "10000"}},
			},
		},
		"plc_status":  map[string]any{},
		"moca_status": map[string]any{},
		"mlo_status":  map[string]any{"msta": "X"},
		"config":      map[string]any{"ctrl_led": map[string]any{"led_val": "1"}},
	}
}

// wlRaw builds a wireless 5G-2 backhaul node.
func wlRaw() map[string]any {
	return map[string]any{
		"mac":           tWL,
		"alias":         "WL",
		"ui_model_name": "M2",
		"model_name":    "M2_RAW",
		"product_id":    "P2",
		"tcode":         "EU/01",
		"online":        "1",
		"level":         "1",
		"re_path":       "8",
		"band_num":      "3",
		"ap2g":          "CC:00:00:00:00:10",
		"ap5g":          "CC:00:00:00:00:14",
		"ap5g1":         "CC:00:00:00:00:18",
		"apdwb":         "CC:00:00:00:00:18",
		"dwb_band":      "2",
		"band_info": map[string]any{
			"0": map[string]any{"unit": int64(0)},
			"2": map[string]any{"unit": int64(1)},
			"3": map[string]any{"unit": int64(2)},
		},
		"pap5g":        tAP5G,
		"pap5g_ssid":   "PARENT_SSID",
		"rssi5g":       "-54",
		"sta5g":        "CC:00:00:00:00:1A",
		"ap5g_ssid_fh": "FH5",
		"config": map[string]any{
			"wireless":         map[string]any{"wl0_radio": "1", "wl1_radio": "1", "wl2_radio": "0"},
			"link_aggregation": map[string]any{"lacp_enabled": "0"},
			"ctrl_led":         map[string]any{"led_val": "0"},
			"prefer_ap":        map[string]any{"amas_wlc_target_bssid": "<" + tRoot},
		},
	}
}

// wiredRaw builds a wired backhaul node.
func wiredRaw() map[string]any {
	return map[string]any{
		"mac":           tWired,
		"alias":         "WIRED",
		"ui_model_name": "M3",
		"online":        "1",
		"level":         "0",
		"re_path":       "1",
		"ap2g":          "",
		"ap5g":          "",
		"sta2g":         "BB:00:00:00:00:91",
		"sta5g":         "BB:00:00:00:00:94",
		"band_info": map[string]any{
			"0": map[string]any{"unit": int64(0)},
			"1": map[string]any{"unit": int64(1)},
		},
		"wired_port": map[string]any{"wan_port": map[string]any{"WAN 0": map[string]any{"link_rate": "G"}}},
		"config":     map[string]any{"wireless": map[string]any{"wl0_radio": "0", "wl1_radio": "0"}},
	}
}

func topo(nodes ...map[string]any) *ARAiMeshTopology {
	list := make([]any, len(nodes))
	for i, n := range nodes {
		list[i] = n
	}
	return TranslateTopology(list, nil, nil)
}

func bandSet(m map[wifi.ARWiFiBand]*ARAiMeshRadio) map[wifi.ARWiFiBand]bool {
	out := map[wifi.ARWiFiBand]bool{}
	for b := range m {
		out[b] = true
	}
	return out
}

func boolIs(t *testing.T, got *bool, want bool) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func intIs(t *testing.T, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("got %v, want %d", got, want)
	}
}

func strIs(t *testing.T, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("got %v, want %q", got, want)
	}
}

// === TestParseTopology ===

func TestParseTopologyThreeNodes(t *testing.T) {
	tp := topo(rootRaw(), wlRaw(), wiredRaw())

	if len(tp.Nodes) != 3 {
		t.Fatalf("nodes: %d", len(tp.Nodes))
	}
	root := tp.Root()
	if root == nil || root.Role != ARAiMeshRoleRouter || root.Backhaul != nil {
		t.Fatalf("root: %+v", root)
	}
}

func TestParseTopologyWirelessBackhaul(t *testing.T) {
	node := topo(rootRaw(), wlRaw()).Get(tWL)
	if node == nil || node.Backhaul == nil {
		t.Fatal("no node/backhaul")
	}
	bh := node.Backhaul
	if bh.Medium != ARAiMeshMediumWireless {
		t.Fatalf("medium %q", bh.Medium)
	}
	if bh.Band == nil || *bh.Band != wifi.ARWiFiBandBand5g2 {
		t.Fatalf("band %v", bh.Band)
	}
	macIs(t, bh.MacParent, tRoot)
	intIs(t, bh.Rssi, -54)
	macIs(t, bh.MacSta, "CC:00:00:00:00:1A")
}

func TestParseTopologyWiredBackhaul(t *testing.T) {
	bh := topo(rootRaw(), wiredRaw()).Get(tWired).Backhaul

	if bh.Medium != ARAiMeshMediumWired {
		t.Fatalf("medium %q", bh.Medium)
	}
	macIs(t, bh.MacParent, tRoot)
	macIs(t, bh.MacSta, tWired)
	intIs(t, bh.Port, 1)
}

func TestParseTopologyRootRadiosAndPorts(t *testing.T) {
	root := topo(rootRaw()).Get(tRoot)

	want := map[wifi.ARWiFiBand]bool{wifi.ARWiFiBandBand2g1: true, wifi.ARWiFiBandBand5g1: true, wifi.ARWiFiBandBand6g1: true}
	if got := bandSet(root.Radios); !reflect.DeepEqual(got, want) {
		t.Fatalf("radios %v", got)
	}
	r2 := root.Radios[wifi.ARWiFiBandBand2g1]
	intIs(t, r2.Unit, 0)
	strIs(t, r2.Ssid, "S2")
	macIs(t, r2.MacIot, "AA:00:00:00:00:12")
	if !root.Radios[wifi.ARWiFiBandBand6g1].Dwb {
		t.Fatal("6g dwb")
	}
	if len(root.Ports) != 2 {
		t.Fatalf("ports %d", len(root.Ports))
	}
	found := false
	for _, p := range root.Ports {
		if p.IsWan && p.MaxRate == 10000 {
			found = true
		}
	}
	if !found {
		t.Fatal("no 10G WAN port")
	}
}

func TestParseTopologyRadioOnFromConfig(t *testing.T) {
	node := topo(wlRaw()).Get(tWL)

	boolIs(t, node.Radios[wifi.ARWiFiBandBand5g1].State, true)
	boolIs(t, node.Radios[wifi.ARWiFiBandBand5g2].State, false)
}

func TestParseTopologyRadiosOffStillPresent(t *testing.T) {
	radios := topo(wiredRaw()).Get(tWired).Radios

	want := map[wifi.ARWiFiBand]bool{wifi.ARWiFiBandBand2g1: true, wifi.ARWiFiBandBand5g1: true}
	if got := bandSet(radios); !reflect.DeepEqual(got, want) {
		t.Fatalf("radios %v", got)
	}
	radio := radios[wifi.ARWiFiBandBand2g1]
	if radio.MacAp != nil {
		t.Fatalf("mac_ap %v", radio.MacAp)
	}
	boolIs(t, radio.State, false)
}

func TestParseTopologyRadioStaMac(t *testing.T) {
	radios := topo(wiredRaw()).Get(tWired).Radios

	macIs(t, radios[wifi.ARWiFiBandBand2g1].MacSta, "BB:00:00:00:00:91")
	macIs(t, radios[wifi.ARWiFiBandBand5g1].MacSta, "BB:00:00:00:00:94")
}

func TestParseTopologyRadioStaSharedPerFrequency(t *testing.T) {
	radios := topo(wlRaw()).Get(tWL).Radios

	macIs(t, radios[wifi.ARWiFiBandBand5g1].MacSta, "CC:00:00:00:00:1A")
	macIs(t, radios[wifi.ARWiFiBandBand5g2].MacSta, "CC:00:00:00:00:1A")
}

func TestParseTopologyRawStatusDictsPreserved(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	if node.PlcStatus == nil || len(node.PlcStatus) != 0 {
		t.Fatalf("plc %v", node.PlcStatus)
	}
	if node.MocaStatus == nil || len(node.MocaStatus) != 0 {
		t.Fatalf("moca %v", node.MocaStatus)
	}
	if !reflect.DeepEqual(node.MloStatus, RawDict{"msta": "X"}) {
		t.Fatalf("mlo %v", node.MloStatus)
	}
}

// === TestOnboardingStatus ===

func TestOnboardingStatusTranslate(t *testing.T) {
	status := TranslateOnboardingStatus(map[string]any{
		"cfg_ready":        "1",
		"cfg_re_maxnum":    "16",
		"cfg_recount":      "2",
		"cfg_obrssi":       "-20",
		"cfg_wifi_quality": "-65",
		"cfg_ui_obmodel":   "RT-AX88U",
	})

	if !status.Ready {
		t.Fatal("ready")
	}
	intIs(t, status.ReMaxnum, 16)
	intIs(t, status.ReCount, 2)
	intIs(t, status.Rssi, -20)
	intIs(t, status.WifiQuality, -65)
	strIs(t, status.Model, "RT-AX88U")
}

func TestOnboardingStatusTranslateEmpty(t *testing.T) {
	status := TranslateOnboardingStatus("x")

	if status.Ready || status.ReMaxnum != nil {
		t.Fatalf("%+v", status)
	}
}

func TestOnboardingStatusTopologyCarriesStatusAndOnboarding(t *testing.T) {
	status := TranslateOnboardingStatus(map[string]any{"cfg_ready": "1"})
	tp := TranslateTopology([]any{}, &status, RawDict{"a": 1})

	if !tp.Status.Ready {
		t.Fatal("ready")
	}
	if !reflect.DeepEqual(tp.Onboarding, RawDict{"a": 1}) {
		t.Fatalf("onboarding %v", tp.Onboarding)
	}
}

func TestOnboardingStatusTopologyDefaults(t *testing.T) {
	tp := TranslateTopology([]any{}, nil, nil)

	if tp.Status.Ready || tp.Onboarding != nil {
		t.Fatalf("%+v", tp)
	}
}

func TestOnboardingStatusUnusableInput(t *testing.T) {
	cases := []struct {
		name string
		data any
	}{
		{"none", nil},
		{"str", "x"},
		{"int", 5},
		{"dict", map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if n := len(TranslateTopology(c.data, nil, nil).Nodes); n != 0 {
				t.Fatalf("nodes %d", n)
			}
		})
	}
}

func TestOnboardingStatusSkipsNonDictAndMacless(t *testing.T) {
	tp := TranslateTopology([]any{"x", map[string]any{"alias": "no-mac"}, rootRaw()}, nil, nil)

	if got := tp.Macs(); !reflect.DeepEqual(got, []identifiers.MacAddress{mac(t, tRoot)}) {
		t.Fatalf("macs %v", got)
	}
}

// === TestTopologyHelpers ===

func helpersTopo() *ARAiMeshTopology { return topo(rootRaw(), wlRaw(), wiredRaw()) }

func macSet(nodes []*ARAiMeshNode) map[identifiers.MacAddress]bool {
	out := map[identifiers.MacAddress]bool{}
	for _, n := range nodes {
		out[n.Mac] = true
	}
	return out
}

func TestTopologyHelpersGetAndMacs(t *testing.T) {
	tp := helpersTopo()
	if tp.Get(tRoot).Mac != mac(t, tRoot) {
		t.Fatal("get root")
	}
	if tp.Get("bad") != nil {
		t.Fatal("get bad")
	}
	if len(tp.Macs()) != 3 {
		t.Fatal("macs")
	}
}

func TestTopologyHelpersParentAndChildren(t *testing.T) {
	tp := helpersTopo()
	if tp.Parent(tWL).Mac != mac(t, tRoot) {
		t.Fatal("parent")
	}
	want := map[identifiers.MacAddress]bool{mac(t, tWL): true, mac(t, tWired): true}
	if got := macSet(tp.Children(tRoot)); !reflect.DeepEqual(got, want) {
		t.Fatalf("children %v", got)
	}
}

func TestTopologyHelpersParentEdgeCases(t *testing.T) {
	tp := helpersTopo()
	if tp.Parent(tRoot) != nil || tp.Parent("bad") != nil {
		t.Fatal("parent")
	}
	if got := tp.Children("bad"); got == nil || len(got) != 0 {
		t.Fatalf("children %v", got)
	}
}

func TestTopologyHelpersDescendantsAndPath(t *testing.T) {
	tp := helpersTopo()
	want := map[identifiers.MacAddress]bool{mac(t, tWL): true, mac(t, tWired): true}
	if got := macSet(tp.Descendants(tRoot)); !reflect.DeepEqual(got, want) {
		t.Fatalf("descendants %v", got)
	}
	path := tp.PathToRoot(tWL)
	if len(path) != 2 || path[0].Mac != mac(t, tWL) || path[1].Mac != mac(t, tRoot) {
		t.Fatalf("path %v", path)
	}
}

func TestTopologyHelpersRootAbsent(t *testing.T) {
	if NewARAiMeshTopology().Root() != nil {
		t.Fatal("root")
	}
}

// === TestBackhaulBranches ===

func bandIs(t *testing.T, got *wifi.ARWiFiBand, want wifi.ARWiFiBand) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("band %v, want %q", got, want)
	}
}

func TestBackhaulBranches2g(t *testing.T) {
	raw := map[string]any{"mac": tWL, "re_path": "2", "pap2g": "P", "sta2g": tWL}
	bh := translateBackhaul(raw, 2, []map[string]any{{"mac": tRoot, "ap2g": "P"}})
	bandIs(t, bh.Band, wifi.ARWiFiBandBand2g1)
	macIs(t, bh.MacParent, tRoot)
}

func TestBackhaulBranches6g(t *testing.T) {
	raw := map[string]any{"mac": tWL, "re_path": "128", "pap6g": "P", "sta6g": tWL}
	bh := translateBackhaul(raw, 128, []map[string]any{{"mac": tRoot, "ap6g1": "P"}})
	bandIs(t, bh.Band, wifi.ARWiFiBandBand6g1)
}

func TestBackhaulBranches5gPrimary(t *testing.T) {
	bh := translateBackhaul(map[string]any{"mac": tWL, "re_path": "8"}, 8, nil)
	bandIs(t, bh.Band, wifi.ARWiFiBandBand5g1)
}

func TestBackhaulBranchesMlo(t *testing.T) {
	raw := map[string]any{"mac": tWL, "papmlo": tRoot, "mlo_status": map[string]any{"msta": tWL}}
	bh := translateBackhaul(raw, 512, nil)
	if bh.Medium != ARAiMeshMediumMlo {
		t.Fatalf("medium %q", bh.Medium)
	}
	macIs(t, bh.MacParent, tRoot)
	macIs(t, bh.MacSta, tWL)
}

func TestBackhaulBranchesWiredMedium(t *testing.T) {
	cases := []struct {
		field  string
		medium ARAiMeshMedium
	}{
		{"plc_status", ARAiMeshMediumPlc},
		{"moca_status", ARAiMeshMediumMoca},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			bh := translateBackhaul(map[string]any{"mac": tWired, c.field: map[string]any{"x": int64(1)}}, 1, nil)
			if bh.Medium != c.medium {
				t.Fatalf("medium %q", bh.Medium)
			}
		})
	}
}

func TestBackhaulBranchesUnknownRePath(t *testing.T) {
	if bh := translateBackhaul(map[string]any{"mac": tWL}, 999, nil); bh != nil {
		t.Fatalf("%+v", bh)
	}
}

func TestBackhaulBranchesParentNotFound(t *testing.T) {
	bh := translateBackhaul(map[string]any{"mac": tWL, "re_path": "2", "pap2g": "P"}, 2, nil)
	if bh.MacParent != nil {
		t.Fatal("wireless parent")
	}
	bhWired := translateBackhaul(map[string]any{"mac": tWL}, 1, nil)
	if bhWired.MacParent != nil {
		t.Fatal("wired parent")
	}
}

func TestBackhaulBranchesWirelessParentEmptyPap(t *testing.T) {
	bh := translateBackhaul(map[string]any{"mac": tWL, "re_path": "2", "pap2g": ""}, 2, nil)
	if bh.MacParent != nil {
		t.Fatal("parent")
	}
}

// === TestSmallHelpers ===

func TestSmallHelpersBandUnits(t *testing.T) {
	cases := []struct {
		name     string
		bandInfo any
		expected map[string]int
	}{
		{"not_dict", "x", map[string]int{}},
		{"known_unknown_index", map[string]any{"0": map[string]any{"unit": int64(0)}, "9": map[string]any{"unit": int64(5)}}, map[string]int{"2g": 0}},
		{"info_not_dict", map[string]any{"0": "x"}, map[string]int{}},
		{"no_unit", map[string]any{"0": map[string]any{"unit": "bad"}}, map[string]int{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := translateBandUnits(map[string]any{"band_info": c.bandInfo}); !reflect.DeepEqual(got, c.expected) {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestSmallHelpersPortsEmpty(t *testing.T) {
	cases := []struct {
		name       string
		capability any
	}{
		{"not_dict", "x"},
		{"empty", map[string]any{}},
		{"section_not_dict", map[string]any{"27": "x"}},
		{"group_not_dict", map[string]any{"27": map[string]any{"lan_port": "x"}}},
		{"info_not_dict", map[string]any{"27": map[string]any{"lan_port": map[string]any{"l": "x"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := translatePorts(map[string]any{"capability": c.capability}); got == nil || len(got) != 0 {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestSmallHelpersNodeRequiresMac(t *testing.T) {
	if translateNode(map[string]any{"alias": "x"}, nil) != nil {
		t.Fatal("node without MAC")
	}
}

func TestSmallHelpersNodeDataclasses(t *testing.T) {
	node := NewARAiMeshNode(mac(t, tRoot))
	if node.Backhaul != nil {
		t.Fatal("backhaul")
	}
	if node.Firmware == nil {
		t.Fatal("firmware")
	}
	bh := NewARAiMeshBackhaul(ARAiMeshMediumWired)
	if bh.Band != nil {
		t.Fatal("band")
	}
}

// === TestTypedFields ===

func TestTypedFieldsAliasMacIsWrapped(t *testing.T) {
	raw := rootRaw()
	raw["alias"] = "AA:00:00:00:00:99"

	alias := topo(raw).Get(tRoot).Alias

	m, ok := alias.(identifiers.MacAddress)
	if !ok || m != mac(t, "AA:00:00:00:00:99") {
		t.Fatalf("alias %#v", alias)
	}
}

func TestTypedFieldsAliasPlainString(t *testing.T) {
	alias := topo(rootRaw()).Get(tRoot).Alias

	if s, ok := alias.(string); !ok || s != "Root" {
		t.Fatalf("alias %#v", alias)
	}
}

func TestTypedFieldsAliasAbsent(t *testing.T) {
	raw := rootRaw()
	delete(raw, "alias")

	if alias := topo(raw).Get(tRoot).Alias; alias != nil {
		t.Fatalf("alias %#v", alias)
	}
}

func TestTypedFieldsFirmwareTyped(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	if node.Firmware == nil || node.FirmwareAvailable == nil {
		t.Fatal("firmware not parsed")
	}
}

func TestTypedFieldsFirmwareUpdateTrue(t *testing.T) {
	raw := rootRaw()
	raw["fwver"] = "3.0.0.6.102_40456"
	raw["newfwver"] = "3.0.0.6.102_40537"

	if !topo(raw).Get(tRoot).FirmwareUpdate {
		t.Fatal("firmware_update")
	}
}

func TestTypedFieldsFirmwareUpdateFalseOnInvalid(t *testing.T) {
	// rootRaw has fwver/newfwver `fw1`/`fw2` (not valid versions)
	if topo(rootRaw()).Get(tRoot).FirmwareUpdate {
		t.Fatal("firmware_update")
	}
}

func TestTypedFieldsPortSpeedTyped(t *testing.T) {
	for _, p := range topo(rootRaw()).Get(tRoot).Ports {
		if p.IsWan {
			if p.MaxRate != ports.ARPortSpeedMbps10000 {
				t.Fatalf("max_rate %d", p.MaxRate)
			}
			return
		}
	}
	t.Fatal("no WAN port")
}

// === TestEnrichedFields ===

func TestEnrichedFieldsModelFields(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	strIs(t, node.Model, "P1")
	strIs(t, node.ModelOriginal, "M1")
	strIs(t, node.ModelDisplay, "M1 UI")
	strIs(t, node.Icon, "ICON1")
}

func TestEnrichedFieldsRegion(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	if node.Region != common.ARRegionUs {
		t.Fatalf("region %q", node.Region)
	}
	intIs(t, node.RegionArea, 1)
}

func TestEnrichedFieldsLedAndDwb(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	boolIs(t, node.Led, true)
	bandIs(t, node.Dwb, wifi.ARWiFiBandBand6g1)
}

func TestEnrichedFieldsRadioFhSsid(t *testing.T) {
	radio := topo(rootRaw()).Get(tRoot).Radios[wifi.ARWiFiBandBand2g1]

	strIs(t, radio.Ssid, "S2")
	strIs(t, radio.SsidFh, "S2FH")
}

func TestEnrichedFieldsWirelessNodeConfig(t *testing.T) {
	node := topo(rootRaw(), wlRaw()).Get(tWL)

	boolIs(t, node.Lacp, false)
	boolIs(t, node.Led, false)
	macIs(t, node.PreferredParent, tRoot)
}

func TestEnrichedFieldsBackhaulParentSsid(t *testing.T) {
	node := topo(rootRaw(), wlRaw()).Get(tWL)

	strIs(t, node.Backhaul.SsidParent, "PARENT_SSID")
}

func TestEnrichedFieldsWiredLinkRate(t *testing.T) {
	node := topo(rootRaw(), wiredRaw()).Get(tWired)

	if node.Backhaul.LinkRate != ports.ARPortSpeedMbps1000 {
		t.Fatalf("link_rate %d", node.Backhaul.LinkRate)
	}
}

func TestEnrichedFieldsConsumedCapabilityStripped(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	if !reflect.DeepEqual(node.Capability, RawDict{"9": "1"}) {
		t.Fatalf("capability %v", node.Capability)
	}
	for _, k := range []string{"27", "28"} {
		if _, ok := node.Capability[k]; ok {
			t.Fatalf("%s kept", k)
		}
	}
}

func TestEnrichedFieldsCapabilityAllConsumedIsNone(t *testing.T) {
	raw := rootRaw()
	raw["capability"] = map[string]any{
		"27": map[string]any{"lan_port": map[string]any{}},
		"28": map[string]any{"wan_port": map[string]any{}},
	}

	if c := topo(raw).Get(tRoot).Capability; c != nil {
		t.Fatalf("capability %v", c)
	}
}

func TestEnrichedFieldsVirtualIf(t *testing.T) {
	vifs := topo(rootRaw()).Get(tRoot).VirtualIf[wifi.ARWiFiBandBand2g1]

	want := []ARAiMeshVif{{Index: 1, Type: 1, Free: true}, {Index: 4, Type: 8, Free: false}}
	if !reflect.DeepEqual(vifs, want) {
		t.Fatalf("vifs %v", vifs)
	}
}

func TestEnrichedFieldsFeaturesDecoded(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	if !node.Features.Has(ARAiMeshFeatureManualReboot) || !node.Features.Has(ARAiMeshFeatureLedAura) {
		t.Fatalf("features %v", node.Features.Sorted())
	}
}

func TestEnrichedFieldsFullyMappedCapabilityStripped(t *testing.T) {
	node := topo(rootRaw()).Get(tRoot)

	for _, k := range []string{"1", "2"} {
		if _, ok := node.Capability[k]; ok {
			t.Fatalf("%s kept", k)
		}
	}
	if !reflect.DeepEqual(node.Capability, RawDict{"9": "1"}) {
		t.Fatalf("capability %v", node.Capability)
	}
}

// === TestVifHelpers ===

func ptrStr(p *int) string {
	if p == nil {
		return "None"
	}
	return strconv.Itoa(*p)
}

func TestVifHelpersParseVifPrefix(t *testing.T) {
	cases := []struct {
		name, prefix string
		unit, index  string
	}{
		{"unit0", "wl0.1", "0", "1"},
		{"unit2", "wl2.7", "2", "7"},
		{"bad", "bad", "None", "None"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			unit, index := parseVifPrefix(c.prefix)
			if ptrStr(unit) != c.unit || ptrStr(index) != c.index {
				t.Fatalf("got (%s, %s)", ptrStr(unit), ptrStr(index))
			}
		})
	}
}

func TestVifHelpersTranslateVifsEmpty(t *testing.T) {
	cases := []struct {
		name       string
		capability any
	}{
		{"not_dict", "x"},
		{"empty", map[string]any{}},
		{"cap_not_dict", map[string]any{"26": "x"}},
		{"band_not_dict", map[string]any{"26": map[string]any{"wifi_band": "x"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := translateVifs(map[string]any{"capability": c.capability}); len(got) != 0 {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestVifHelpersTranslateVifsSkipsBadEntries(t *testing.T) {
	raw := map[string]any{
		"capability": map[string]any{
			"26": map[string]any{
				"wifi_band": map[string]any{
					"bad":   "x",
					"band1": map[string]any{"vif": "x"},
					"band2": map[string]any{
						"vif": map[string]any{
							"wl0.1": "x",
							"bad":   map[string]any{"type": "1"},
							"wl0.2": map[string]any{"type": "x"},
						},
					},
				},
			},
		},
	}

	if got := translateVifs(raw); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestVifHelpersVifUnknownBand(t *testing.T) {
	raw := map[string]any{
		"capability": map[string]any{
			"26": map[string]any{"wifi_band": map[string]any{"b": map[string]any{"vif": map[string]any{"wl9.1": map[string]any{"type": "1"}}}}},
		},
	}

	vifs := translateVifs(raw)
	if len(vifs) != 1 {
		t.Fatalf("vifs %v", vifs)
	}
	got, ok := vifs[wifi.ARWiFiBandUnknown]
	if !ok || got[0] != (ARAiMeshVif{Index: 1, Type: 1, Free: true}) {
		t.Fatalf("vifs %v", vifs)
	}
}

// === TestEnrichmentHelpers ===

func TestEnrichmentHelpersConfigFlagMissing(t *testing.T) {
	if configFlag(map[string]any{}, "ctrl_led", "led_val") != nil {
		t.Fatal("no section")
	}
	if configFlag(map[string]any{"ctrl_led": map[string]any{}}, "ctrl_led", "led_val") != nil {
		t.Fatal("no key")
	}
}

func TestEnrichmentHelpersPreferredParentAbsent(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
	}{
		{"no_section", map[string]any{}},
		{"no_key", map[string]any{"prefer_ap": map[string]any{}}},
		{"not_str", map[string]any{"prefer_ap": map[string]any{"amas_wlc_target_bssid": int64(5)}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := preferredParent(c.config); got != nil {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestEnrichmentHelpersDwbAbsent(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
	}{
		{"absent", map[string]any{}},
		{"disabled", map[string]any{"dwb_band": "-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dwbBand(c.raw, map[wifi.ARWiFiBand]*ARAiMeshRadio{}); got != nil {
				t.Fatalf("got %v", *got)
			}
		})
	}
}

func TestEnrichmentHelpersWiredLinkRateUnknown(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
	}{
		{"no_port", map[string]any{}},
		{"not_dict", map[string]any{"wired_port": "x"}},
		{"no_rate", map[string]any{"wired_port": map[string]any{"wan_port": map[string]any{"p": map[string]any{}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := wiredLinkRate(c.raw); got != ports.ARPortSpeedUnknown {
				t.Fatalf("got %d", got)
			}
		})
	}
}
