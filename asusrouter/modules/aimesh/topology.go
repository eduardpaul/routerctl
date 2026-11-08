package aimesh

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/ports"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// RawDict is a raw passthrough dict (`dict[str, Any] | None`): a nil
// RawDict is Python `None` (canonical `null`), an empty one is `{}`.
type RawDict map[string]any

// CanonicalValue keeps `None` distinct from `{}`.
func (d RawDict) CanonicalValue() any {
	if d == nil {
		return nil
	}
	return map[string]any(d)
}

// rawDict returns the value as-is if it is a dict, else nil.
func rawDict(value any) RawDict {
	if m, ok := value.(map[string]any); ok {
		return RawDict(m)
	}
	if m, ok := value.(RawDict); ok {
		return m
	}
	return nil
}

// truthy mirrors Python truthiness for parsed router data.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case map[string]any:
		return len(t) > 0
	case []any:
		return len(t) > 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.String:
		return rv.Len() > 0
	case reflect.Pointer, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}

// strOrNil mirrors `value or None` for a string field.
func strOrNil(v any) *string {
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}

// rawEqual mirrors Python `==` between parsed router values.
func rawEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return reflect.DeepEqual(a, b)
}

func macEqual(a, b *identifiers.MacAddress) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// naturalKeys returns the keys of a parsed dict in a stable order.
//
// Python iterates parsed JSON objects in document order, which Go maps
// do not keep; the router lists these keys in ascending natural order
// (`0`, `1`, ..., `wl0.1`, `wl0.2`, ...), so that order is rebuilt here.
func naturalKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool { return naturalLess(keys[i], keys[j]) })
	return keys
}

// naturalLess compares strings with digit runs compared numerically.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ad, bd := digitPrefix(a), digitPrefix(b)
		if ad != "" && bd != "" {
			an, _ := strconv.Atoi(ad)
			bn, _ := strconv.Atoi(bd)
			if an != bn {
				return an < bn
			}
			a, b = a[len(ad):], b[len(bd):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// === Radio ===

type apBand struct {
	suffix string
	band   wifi.ARWiFiBand
}

// AP field suffix -> wifi band
var apBands = []apBand{
	{"2g", wifi.ARWiFiBandBand2g1},
	{"5g", wifi.ARWiFiBandBand5g1},
	{"5g1", wifi.ARWiFiBandBand5g2},
	{"6g", wifi.ARWiFiBandBand6g1},
	{"6g1", wifi.ARWiFiBandBand6g2},
}

func apBandOf(suffix string) (wifi.ARWiFiBand, bool) {
	for _, ab := range apBands {
		if ab.suffix == suffix {
			return ab.band, true
		}
	}
	return wifi.ARWiFiBandUnknown, false
}

// wifi band -> sta field suffix (one station per frequency)
var bandSta = map[wifi.ARWiFiBand]string{
	wifi.ARWiFiBandBand2g1: "2g",
	wifi.ARWiFiBandBand2g2: "2g",
	wifi.ARWiFiBandBand5g1: "5g",
	wifi.ARWiFiBandBand5g2: "5g",
	wifi.ARWiFiBandBand6g1: "6g",
	wifi.ARWiFiBandBand6g2: "6g",
}

// band_info index -> ap field suffix
var bandInfoSuffix = map[string]string{
	"0": "2g",
	"1": "5g",
	"2": "5g",
	"3": "5g1",
	"4": "6g",
	"5": "6g",
	"6": "6g1",
}

// ARAiMeshRadio is a fronthaul radio of an AiMesh node.
type ARAiMeshRadio struct {
	Band   wifi.ARWiFiBand         `json:"band"`
	MacAp  *identifiers.MacAddress `json:"mac_ap"`
	MacFh  *identifiers.MacAddress `json:"mac_fh"`
	MacIot *identifiers.MacAddress `json:"mac_iot"`
	MacSta *identifiers.MacAddress `json:"mac_sta"`
	Ssid   *string                 `json:"ssid"`
	SsidFh *string                 `json:"ssid_fh"`
	Unit   *int                    `json:"unit"`
	State  *bool                   `json:"state"`
	Dwb    bool                    `json:"dwb"`
}

// translateBandUnits maps AP field suffix -> wl unit from `band_info`.
func translateBandUnits(raw map[string]any) map[string]int {
	bandInfo, ok := raw["band_info"].(map[string]any)
	if !ok {
		return map[string]int{}
	}

	units := map[string]int{}
	for _, index := range naturalKeys(bandInfo) {
		suffix, known := bandInfoSuffix[index]
		var unit *int
		if info, isDict := bandInfo[index].(map[string]any); isDict {
			unit = converters.RawToInt(info["unit"])
		}
		if known && unit != nil {
			units[suffix] = *unit
		}
	}
	return units
}

// translateRadios builds the per-band fronthaul radios of a node.
func translateRadios(raw map[string]any) map[wifi.ARWiFiBand]*ARAiMeshRadio {
	units := translateBandUnits(raw)
	var wireless map[string]any
	if config, ok := raw["config"].(map[string]any); ok {
		wireless, _ = config["wireless"].(map[string]any)
	}
	dwbMac := identifiers.MacAddressFromValueSafe(raw["apdwb"])

	radios := map[wifi.ARWiFiBand]*ARAiMeshRadio{}
	for _, ab := range apBands {
		suffix, band := ab.suffix, ab.band
		macAp := identifiers.MacAddressFromValueSafe(raw["ap"+suffix])
		macFh := identifiers.MacAddressFromValueSafe(raw["ap"+suffix+"_fh"])
		macIot := identifiers.MacAddressFromValueSafe(raw["ap"+suffix+"_iot_fh"])
		var unit *int
		if u, ok := units[suffix]; ok {
			unit = &u
		}
		// A radio is present if `band_info` lists it (even with radios off
		// and empty MACs) or any MAC is populated
		if unit == nil && macAp == nil && macFh == nil {
			continue
		}
		var state *bool
		if unit != nil {
			if v, ok := wireless["wl"+strconv.Itoa(*unit)+"_radio"]; ok {
				s := rawEqual(v, "1")
				state = &s
			}
		}
		radios[band] = &ARAiMeshRadio{
			Band:   band,
			MacAp:  macAp,
			MacFh:  macFh,
			MacIot: macIot,
			MacSta: identifiers.MacAddressFromValueSafe(raw["sta"+bandSta[band]]),
			Ssid:   strOrNil(raw["ap"+suffix+"_ssid"]),
			SsidFh: strOrNil(raw["ap"+suffix+"_ssid_fh"]),
			Unit:   unit,
			State:  state,
			Dwb:    dwbMac != nil && macEqual(dwbMac, macAp),
		}
	}

	return radios
}

// === Virtual interface (VIF) ===

// ARAiMeshVif is a virtual interface (VIF) slot on an AiMesh radio.
type ARAiMeshVif struct {
	Index int `json:"index"`
	// TODO: decode the type bitmask into purposes; only `free` (type == 1)
	// is currently confirmed
	Type int  `json:"type"`
	Free bool `json:"free"`
}

// parseVifPrefix parses a vif prefix `wl{unit}.{index}` into (unit, index).
func parseVifPrefix(prefix string) (*int, *int) {
	unitStr, indexStr, _ := strings.Cut(strings.TrimPrefix(prefix, "wl"), ".")
	return converters.RawToInt(unitStr), converters.RawToInt(indexStr)
}

// translateVifs builds the per-band virtual interfaces from capability 26.
func translateVifs(raw map[string]any) map[wifi.ARWiFiBand][]ARAiMeshVif {
	var wifiBand map[string]any
	if capability, ok := raw["capability"].(map[string]any); ok {
		if cap26, ok := capability["26"].(map[string]any); ok {
			wifiBand, _ = cap26["wifi_band"].(map[string]any)
		}
	}
	if wifiBand == nil {
		return map[wifi.ARWiFiBand][]ARAiMeshVif{}
	}

	units := translateBandUnits(raw)
	unitToBand := map[int]wifi.ARWiFiBand{}
	// Python builds this dict from `units` in insertion order; a later
	// suffix with the same unit wins
	for _, ab := range apBands {
		if unit, ok := units[ab.suffix]; ok {
			unitToBand[unit] = ab.band
		}
	}

	result := map[wifi.ARWiFiBand][]ARAiMeshVif{}
	for _, bandKey := range naturalKeys(wifiBand) {
		bandEntry, ok := wifiBand[bandKey].(map[string]any)
		if !ok {
			continue
		}
		vifs, ok := bandEntry["vif"].(map[string]any)
		if !ok {
			continue
		}
		for _, prefix := range naturalKeys(vifs) {
			vif, ok := vifs[prefix].(map[string]any)
			if !ok {
				continue
			}
			unit, index := parseVifPrefix(prefix)
			kind := converters.RawToInt(vif["type"])
			if index == nil || kind == nil {
				continue
			}
			band := wifi.ARWiFiBandUnknown
			if unit != nil {
				if b, ok := unitToBand[*unit]; ok {
					band = b
				}
			}
			result[band] = append(result[band], ARAiMeshVif{Index: *index, Type: *kind, Free: *kind == 1})
		}
	}

	return result
}

// === Port ===

// ARAiMeshPort is a wired port of an AiMesh node.
type ARAiMeshPort struct {
	MaxRate ports.ARPortSpeed `json:"max_rate"`
	Label   *string           `json:"label"`
	IsWan   bool              `json:"is_wan"`
	Index   *int              `json:"index"`
}

// translatePorts builds the wired ports of a node from capability 27/28.
func translatePorts(raw map[string]any) []ARAiMeshPort {
	capability, ok := raw["capability"].(map[string]any)
	if !ok {
		return []ARAiMeshPort{}
	}

	out := []ARAiMeshPort{}
	for _, spec := range []struct {
		capKey, subKey string
		isWan          bool
	}{
		{"27", "lan_port", false},
		{"28", "wan_port", true},
	} {
		section, ok := capability[spec.capKey].(map[string]any)
		if !ok {
			continue
		}
		group, ok := section[spec.subKey].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range naturalKeys(group) {
			info, ok := group[key].(map[string]any)
			if !ok {
				continue
			}
			out = append(out, ARAiMeshPort{
				Label:   strOrNil(info["label_name"]),
				IsWan:   spec.isWan,
				MaxRate: ports.ARPortSpeedFromValue(info["max_rate"]),
				Index:   converters.RawToInt(info["index"]),
			})
		}
	}
	return out
}

// === Capability passthrough ===

// Capability keys already decoded into typed fields, stripped from the
// raw `capability` passthrough so data is not duplicated
var consumedCapability = map[string]struct{}{"26": {}, "27": {}, "28": {}}

// remainingCapability returns the raw capability without the keys we
// already decoded.
func remainingCapability(raw map[string]any) RawDict {
	capability, ok := raw["capability"].(map[string]any)
	if !ok {
		return nil
	}
	remaining := RawDict{}
	for key, value := range capability {
		if _, consumed := consumedCapability[key]; consumed {
			continue
		}
		if IsFullyDecoded(key, value) {
			continue
		}
		remaining[key] = value
	}
	if len(remaining) == 0 {
		return nil
	}
	return remaining
}

// === Backhaul ===

type backhaulBand struct {
	band     wifi.ARWiFiBand
	apFields []string
	staField string
}

// pap field suffix -> (band, ap fields to match a parent on, sta field)
var backhaulBands = map[string]backhaulBand{
	"2g": {wifi.ARWiFiBandBand2g1, []string{"ap2g"}, "sta2g"},
	"5g": {wifi.ARWiFiBandBand5g1, []string{"ap5g", "ap5g1"}, "sta5g"},
	"6g": {wifi.ARWiFiBandBand6g1, []string{"ap6g", "ap6g1"}, "sta6g"},
}

// Wired re_path -> backhaul port sequence index
var ethRePathPort = map[int]int{1: 1, 16: 2, 32: 3, 64: 4}

// re_path -> wireless backhaul band suffix
var wirelessRePaths = map[int]string{
	2:   "2g",
	4:   "5g",
	6:   "5g",
	8:   "5g",
	10:  "5g",
	128: "6g",
}

const (
	mloRePath = 512
	// On tri-band routers re_path 8 is the secondary 5 GHz (5G-2)
	rePath5gSecondary = 8
	triBand           = "3"
)

// ARAiMeshBackhaul is an uplink of an AiMesh node to its parent.
type ARAiMeshBackhaul struct {
	Medium     ARAiMeshMedium          `json:"medium"`
	MacParent  *identifiers.MacAddress `json:"mac_parent"`
	SsidParent *string                 `json:"ssid_parent"`
	Band       *wifi.ARWiFiBand        `json:"band"`
	MacSta     *identifiers.MacAddress `json:"mac_sta"`
	Rssi       *int                    `json:"rssi"`
	Port       *int                    `json:"port"`
	LinkRate   ports.ARPortSpeed       `json:"link_rate"`
}

// NewARAiMeshBackhaul mirrors `ARAiMeshBackhaul(medium=...)` with the
// Python defaults (`link_rate` is UNKNOWN, not the Go zero value).
func NewARAiMeshBackhaul(medium ARAiMeshMedium) *ARAiMeshBackhaul {
	return &ARAiMeshBackhaul{Medium: medium, LinkRate: ports.ARPortSpeedUnknown}
}

// parentWired finds the parent whose `wired_mac` lists this node.
func parentWired(raw map[string]any, nodes []map[string]any) *identifiers.MacAddress {
	mac := raw["mac"]
	for _, other := range nodes {
		wired, ok := other["wired_mac"].([]any)
		if !ok {
			continue
		}
		for _, w := range wired {
			if rawEqual(w, mac) {
				return identifiers.MacAddressFromValueSafe(other["mac"])
			}
		}
	}
	return nil
}

// parentWireless finds the parent whose AP BSSID matches this node's `pap`.
func parentWireless(raw map[string]any, suffix string, apFields []string, nodes []map[string]any) *identifiers.MacAddress {
	pap := raw["pap"+suffix]
	if !truthy(pap) {
		return nil
	}
	for _, other := range nodes {
		for _, ap := range apFields {
			if rawEqual(other[ap], pap) {
				return identifiers.MacAddressFromValueSafe(other["mac"])
			}
		}
	}
	return nil
}

// wiredLinkRate reads the wired uplink link rate from `wired_port`.
func wiredLinkRate(raw map[string]any) ports.ARPortSpeed {
	wiredPort, ok := raw["wired_port"].(map[string]any)
	if !ok {
		return ports.ARPortSpeedUnknown
	}
	wanPorts, ok := wiredPort["wan_port"].(map[string]any)
	if !ok {
		return ports.ARPortSpeedUnknown
	}
	for _, key := range naturalKeys(wanPorts) {
		info, ok := wanPorts[key].(map[string]any)
		if !ok {
			continue
		}
		if code := info["link_rate"]; truthy(code) {
			s, _ := code.(string)
			return ports.ReadEthernetPortSpeed(s)
		}
	}
	return ports.ARPortSpeedUnknown
}

// translateBackhaul resolves the single backhaul uplink of a node.
func translateBackhaul(raw map[string]any, rePath int, nodes []map[string]any) *ARAiMeshBackhaul {
	if port, wired := ethRePathPort[rePath]; wired {
		medium := ARAiMeshMediumWired
		if truthy(raw["plc_status"]) {
			medium = ARAiMeshMediumPlc
		} else if truthy(raw["moca_status"]) {
			medium = ARAiMeshMediumMoca
		}
		return &ARAiMeshBackhaul{
			Medium:    medium,
			MacParent: parentWired(raw, nodes),
			MacSta:    identifiers.MacAddressFromValueSafe(raw["mac"]),
			Port:      &port,
			LinkRate:  wiredLinkRate(raw),
		}
	}

	if rePath == mloRePath {
		var msta any
		if mlo, ok := raw["mlo_status"].(map[string]any); ok {
			msta = mlo["msta"]
		}
		return &ARAiMeshBackhaul{
			Medium:    ARAiMeshMediumMlo,
			MacParent: identifiers.MacAddressFromValueSafe(raw["papmlo"]),
			MacSta:    identifiers.MacAddressFromValueSafe(msta),
			LinkRate:  ports.ARPortSpeedUnknown,
		}
	}

	suffix, ok := wirelessRePaths[rePath]
	if !ok {
		return nil
	}

	bb := backhaulBands[suffix]
	band := bb.band
	// Tri-band 5 GHz second radio is the 5G-2 backhaul band
	if suffix == "5g" && rePath == rePath5gSecondary && converters.PyStr(raw["band_num"]) == triBand {
		band = wifi.ARWiFiBandBand5g2
	}
	return &ARAiMeshBackhaul{
		Medium:     ARAiMeshMediumWireless,
		MacParent:  parentWireless(raw, suffix, bb.apFields, nodes),
		SsidParent: strOrNil(raw["pap"+suffix+"_ssid"]),
		Band:       &band,
		MacSta:     identifiers.MacAddressFromValueSafe(raw[bb.staField]),
		Rssi:       converters.RawToInt(raw["rssi"+suffix]),
		LinkRate:   ports.ARPortSpeedUnknown,
	}
}

// === Node ===

// ARAiMeshNode is an AiMesh device description.
//
// Alias is nil, a string, or an identifiers.MacAddress (an alias that is
// a MAC is wrapped so it can be masked when logged).
type ARAiMeshNode struct {
	Mac               identifiers.MacAddress             `json:"mac"`
	Alias             any                                `json:"alias"`
	Model             *string                            `json:"model"`
	ModelOriginal     *string                            `json:"model_original"`
	ModelDisplay      *string                            `json:"model_display"`
	Icon              *string                            `json:"icon"`
	Ip                *identifiers.IpAddress             `json:"ip"`
	Region            common.ARRegion                    `json:"region"`
	RegionArea        *int                               `json:"region_area"`
	Firmware          *firmware.ARFirmware               `json:"firmware"`
	FirmwareAvailable *firmware.ARFirmware               `json:"firmware_available"`
	FirmwareUpdate    bool                               `json:"firmware_update"`
	Online            bool                               `json:"online"`
	Level             int                                `json:"level"`
	Role              ARAiMeshRole                       `json:"role"`
	Backhaul          *ARAiMeshBackhaul                  `json:"backhaul"`
	PreferredParent   *identifiers.MacAddress            `json:"preferred_parent"`
	Radios            map[wifi.ARWiFiBand]*ARAiMeshRadio `json:"radios"`
	VirtualIf         map[wifi.ARWiFiBand][]ARAiMeshVif  `json:"virtual_if"`
	Features          FeatureSet                         `json:"features"`
	Dwb               *wifi.ARWiFiBand                   `json:"dwb"`
	Ports             []ARAiMeshPort                     `json:"ports"`
	Lacp              *bool                              `json:"lacp"`
	Led               *bool                              `json:"led"`
	// Kept raw for now (option b) - decoded incrementally later
	PlcStatus  RawDict `json:"plc_status"`
	MocaStatus RawDict `json:"moca_status"`
	MloStatus  RawDict `json:"mlo_status"`
	Capability RawDict `json:"capability"`
	Config     RawDict `json:"config"`
}

// NewARAiMeshNode mirrors `ARAiMeshNode(mac=...)`: a node with the Python
// dataclass defaults (Go zero values cannot express the default factories).
func NewARAiMeshNode(mac identifiers.MacAddress) *ARAiMeshNode {
	return &ARAiMeshNode{
		Mac:               mac,
		Region:            common.ARRegionUnknown,
		Firmware:          firmware.NewARFirmware(nil, nil, nil, nil, false),
		FirmwareAvailable: firmware.NewARFirmware(nil, nil, nil, nil, false),
		Role:              ARAiMeshRoleUnknown,
		Radios:            map[wifi.ARWiFiBand]*ARAiMeshRadio{},
		VirtualIf:         map[wifi.ARWiFiBand][]ARAiMeshVif{},
		Features:          FeatureSet{},
		Ports:             []ARAiMeshPort{},
	}
}

// configFlag reads a boolean flag from a config section.
func configFlag(config map[string]any, section, key string) *bool {
	sub, ok := config[section].(map[string]any)
	if !ok {
		return nil
	}
	v, ok := sub[key]
	if !ok {
		return nil
	}
	return converters.RawToBool(v)
}

// preferredParent reads the configured preferred parent BSSID (`<`
// prefix stripped).
func preferredParent(config map[string]any) *identifiers.MacAddress {
	prefer, ok := config["prefer_ap"].(map[string]any)
	if !ok {
		return nil
	}
	bssid, ok := prefer["amas_wlc_target_bssid"].(string)
	if !ok {
		return nil
	}
	return identifiers.MacAddressFromValueSafe(strings.TrimLeft(bssid, "<"))
}

// dwbBand resolves the dedicated wireless backhaul band from `dwb_band`.
func dwbBand(raw map[string]any, radios map[wifi.ARWiFiBand]*ARAiMeshRadio) *wifi.ARWiFiBand {
	unit := converters.RawToInt(raw["dwb_band"])
	if unit == nil || *unit < 0 {
		return nil
	}
	// Radios are built (inserted) in AP band order
	for _, ab := range apBands {
		radio, ok := radios[ab.band]
		if ok && radio.Unit != nil && *radio.Unit == *unit {
			band := ab.band
			return &band
		}
	}
	return nil
}

// firmwareUpdate reports whether a newer firmware is available for the
// node.
func firmwareUpdate(current, available *firmware.ARFirmware) bool {
	if current.Major() == nil || available.Major() == nil {
		return false
	}
	return current.Less(available)
}

// translateNode builds a single node description.
func translateNode(raw map[string]any, nodes []map[string]any) *ARAiMeshNode {
	mac := identifiers.MacAddressFromValueSafe(raw["mac"])
	if mac == nil {
		return nil
	}

	rePath := 0
	if v := converters.RawToInt(raw["re_path"]); v != nil {
		rePath = *v
	}
	role := ARAiMeshRoleNode
	var backhaul *ARAiMeshBackhaul
	if rePath == 0 {
		role = ARAiMeshRoleRouter
	} else {
		backhaul = translateBackhaul(raw, rePath, nodes)
	}

	// An alias that is a MAC is wrapped so it can be masked when logged
	var alias any
	if truthy(raw["alias"]) {
		alias = raw["alias"]
		if m := identifiers.MacAddressFromValueSafe(alias); m != nil {
			alias = *m
		}
	}

	radios := translateRadios(raw)
	config, _ := raw["config"].(map[string]any)
	region, regionArea := common.TranslateRegion(raw["tcode"])
	fw := firmware.ARFirmwareFromString(raw["fwver"])
	fwAvailable := firmware.ARFirmwareFromString(raw["newfwver"])
	level := 0
	if v := converters.RawToInt(raw["level"]); v != nil {
		level = *v
	}
	var configOut RawDict
	if len(config) > 0 {
		configOut = RawDict(config)
	}

	return &ARAiMeshNode{
		Mac:               *mac,
		Alias:             alias,
		Model:             strOrNil(raw["product_id"]),
		ModelOriginal:     strOrNil(raw["model_name"]),
		ModelDisplay:      strOrNil(raw["ui_model_name"]),
		Icon:              strOrNil(raw["icon_model_name"]),
		Ip:                identifiers.IpAddressFromValueSafe(raw["ip"]),
		Region:            region,
		RegionArea:        regionArea,
		Firmware:          fw,
		FirmwareAvailable: fwAvailable,
		FirmwareUpdate:    firmwareUpdate(fw, fwAvailable),
		Online:            rawEqual(raw["online"], "1"),
		Level:             level,
		Role:              role,
		Backhaul:          backhaul,
		PreferredParent:   preferredParent(config),
		Radios:            radios,
		VirtualIf:         translateVifs(raw),
		Features:          TranslateFeatures(raw["capability"]),
		Dwb:               dwbBand(raw, radios),
		Ports:             translatePorts(raw),
		Lacp:              configFlag(config, "link_aggregation", "lacp_enabled"),
		Led:               configFlag(config, "ctrl_led", "led_val"),
		PlcStatus:         rawDict(raw["plc_status"]),
		MocaStatus:        rawDict(raw["moca_status"]),
		MloStatus:         rawDict(raw["mlo_status"]),
		Capability:        remainingCapability(raw),
		Config:            configOut,
	}
}

// === Onboarding status ===

// ARAiMeshOnboardingStatus is the AiMesh onboarding process state
// (`get_onboardingstatus`).
type ARAiMeshOnboardingStatus struct {
	Ready       bool    `json:"ready"`
	Status      *int    `json:"status"`
	Stage       *int    `json:"stage"`
	Result      *int    `json:"result"`
	FailResult  *int    `json:"fail_result"`
	Count       *int    `json:"count"`
	ReCount     *int    `json:"re_count"`
	ReMaxnum    *int    `json:"re_maxnum"`
	WifiQuality *int    `json:"wifi_quality"`
	Rssi        *int    `json:"rssi"`
	Model       *string `json:"model"`
}

// TranslateOnboardingStatus builds the onboarding status from
// `get_onboardingstatus`.
func TranslateOnboardingStatus(raw any) ARAiMeshOnboardingStatus {
	m, ok := raw.(map[string]any)
	if !ok {
		return ARAiMeshOnboardingStatus{}
	}

	var model *string
	switch {
	case truthy(m["cfg_ui_obmodel"]):
		model = strOrNil(m["cfg_ui_obmodel"])
	case truthy(m["cfg_obmodel"]):
		model = strOrNil(m["cfg_obmodel"])
	}

	return ARAiMeshOnboardingStatus{
		Ready:       rawEqual(m["cfg_ready"], "1"),
		Status:      converters.RawToInt(m["cfg_obstatus"]),
		Stage:       converters.RawToInt(m["cfg_obstage"]),
		Result:      converters.RawToInt(m["cfg_obresult"]),
		FailResult:  converters.RawToInt(m["cfg_obfailresult"]),
		Count:       converters.RawToInt(m["cfg_obcount"]),
		ReCount:     converters.RawToInt(m["cfg_recount"]),
		ReMaxnum:    converters.RawToInt(m["cfg_re_maxnum"]),
		WifiQuality: converters.RawToInt(m["cfg_wifi_quality"]),
		Rssi:        converters.RawToInt(m["cfg_obrssi"]),
		Model:       model,
	}
}

// === Topology ===

// ARAiMeshTopology is a graph of AiMesh nodes with reconstruction helpers.
//
// Nodes keeps the Python dict; the insertion order (the order of the
// router's node list) is kept separately, so build topologies with
// TranslateTopology or NewARAiMeshTopology + Add.
type ARAiMeshTopology struct {
	Nodes      map[identifiers.MacAddress]*ARAiMeshNode `json:"nodes"`
	Status     ARAiMeshOnboardingStatus                 `json:"status"`
	Onboarding RawDict                                  `json:"onboarding"`

	order []identifiers.MacAddress
}

// NewARAiMeshTopology returns an empty topology (Python
// `ARAiMeshTopology()`).
func NewARAiMeshTopology() *ARAiMeshTopology {
	return &ARAiMeshTopology{Nodes: map[identifiers.MacAddress]*ARAiMeshNode{}}
}

// Add inserts (or replaces in place) a node, keeping insertion order.
func (t *ARAiMeshTopology) Add(node *ARAiMeshNode) {
	if t.Nodes == nil {
		t.Nodes = map[identifiers.MacAddress]*ARAiMeshNode{}
	}
	if _, ok := t.Nodes[node.Mac]; !ok {
		t.order = append(t.order, node.Mac)
	}
	t.Nodes[node.Mac] = node
}

// values returns the nodes in insertion order (nodes added to the map
// directly come last, sorted by MAC).
func (t *ARAiMeshTopology) values() []*ARAiMeshNode {
	out := make([]*ARAiMeshNode, 0, len(t.Nodes))
	seen := make(map[identifiers.MacAddress]struct{}, len(t.Nodes))
	for _, mac := range t.order {
		if node, ok := t.Nodes[mac]; ok {
			if _, dup := seen[mac]; !dup {
				seen[mac] = struct{}{}
				out = append(out, node)
			}
		}
	}
	if len(out) == len(t.Nodes) {
		return out
	}
	var extra []*ARAiMeshNode
	for mac, node := range t.Nodes {
		if _, ok := seen[mac]; !ok {
			extra = append(extra, node)
		}
	}
	sort.Slice(extra, func(i, j int) bool {
		return converters.PyStr(extra[i].Mac) < converters.PyStr(extra[j].Mac)
	})
	return append(out, extra...)
}

// Get gets a node by MAC.
func (t *ARAiMeshTopology) Get(mac any) *ARAiMeshNode {
	key := identifiers.MacAddressFromValueSafe(mac)
	if key == nil || t == nil {
		return nil
	}
	return t.Nodes[*key]
}

// Macs returns all node MACs (for `all nodes` discovery).
func (t *ARAiMeshTopology) Macs() []identifiers.MacAddress {
	nodes := t.values()
	out := make([]identifiers.MacAddress, len(nodes))
	for i, n := range nodes {
		out[i] = n.Mac
	}
	return out
}

// Root returns the router / CAP node, if present.
func (t *ARAiMeshTopology) Root() *ARAiMeshNode {
	for _, n := range t.values() {
		if n.Role == ARAiMeshRoleRouter {
			return n
		}
	}
	return nil
}

// Parent returns the direct parent of a node.
func (t *ARAiMeshTopology) Parent(mac any) *ARAiMeshNode {
	node := t.Get(mac)
	if node == nil || node.Backhaul == nil {
		return nil
	}
	parent := node.Backhaul.MacParent
	if parent == nil {
		return nil
	}
	return t.Nodes[*parent]
}

// Children returns the direct children of a node.
func (t *ARAiMeshTopology) Children(mac any) []*ARAiMeshNode {
	target := identifiers.MacAddressFromValueSafe(mac)
	if target == nil {
		return []*ARAiMeshNode{}
	}
	out := []*ARAiMeshNode{}
	for _, n := range t.values() {
		if n.Backhaul != nil && macEqual(n.Backhaul.MacParent, target) {
			out = append(out, n)
		}
	}
	return out
}

// Descendants returns all nodes below a node (every generation).
func (t *ARAiMeshTopology) Descendants(mac any) []*ARAiMeshNode {
	result := []*ARAiMeshNode{}
	queue := t.Children(mac)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)
		queue = append(queue, t.Children(node.Mac)...)
	}
	return result
}

// PathToRoot returns the nodes from the given node up to the root
// (inclusive).
func (t *ARAiMeshTopology) PathToRoot(mac any) []*ARAiMeshNode {
	path := []*ARAiMeshNode{}
	node := t.Get(mac)
	seen := map[identifiers.MacAddress]struct{}{}
	for node != nil {
		if _, ok := seen[node.Mac]; ok {
			break
		}
		seen[node.Mac] = struct{}{}
		path = append(path, node)
		node = t.Parent(node.Mac)
	}
	return path
}

// TranslateTopology builds a topology from the `get_cfg_clientlist` node
// list. status may be nil (the default status).
func TranslateTopology(nodes any, status *ARAiMeshOnboardingStatus, onboarding RawDict) *ARAiMeshTopology {
	var rawNodes []map[string]any
	if list, ok := nodes.([]any); ok {
		for _, n := range list {
			if m, ok := n.(map[string]any); ok {
				rawNodes = append(rawNodes, m)
			}
		}
	}
	result := NewARAiMeshTopology()
	for _, raw := range rawNodes {
		if node := translateNode(raw, rawNodes); node != nil {
			result.Add(node)
		}
	}

	if status != nil {
		result.Status = *status
	}
	result.Onboarding = onboarding
	return result
}
