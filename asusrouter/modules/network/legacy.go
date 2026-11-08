// Legacy backend for the network module.

package network

import (
	"context"
	"fmt"
	"sort"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Per-band main network keys (`wl{unit}_*`)
var mainKeys = []nvram.ARNvramIndexType{
	nvram.ARNvramIndexTypeWlSsid,
	nvram.ARNvramIndexTypeWlWpaPsk,
	nvram.ARNvramIndexTypeWlAuthMode,
	nvram.ARNvramIndexTypeWlCrypto,
	nvram.ARNvramIndexTypeWlClosed,
	nvram.ARNvramIndexTypeWlMacmode,
	nvram.ARNvramIndexTypeWlMaclistX,
	nvram.ARNvramIndexTypeWlRadio,
	nvram.ARNvramIndexTypeWlApIsolate,
}

// Per-band guest network keys (`wl{unit}.{slot}_*`)
var guestKeys = []nvram.ARNvramIndexType{
	nvram.ARNvramIndexTypeWlSsid,
	nvram.ARNvramIndexTypeWlWpaPsk,
	nvram.ARNvramIndexTypeWlAuthMode,
	nvram.ARNvramIndexTypeWlCrypto,
	nvram.ARNvramIndexTypeWlClosed,
	nvram.ARNvramIndexTypeWlMacmode,
	nvram.ARNvramIndexTypeWlMaclist,
	nvram.ARNvramIndexTypeWlBssEnabled,
	nvram.ARNvramIndexTypeWlApIsolate,
	nvram.ARNvramIndexTypeWlLanaccess,
	nvram.ARNvramIndexTypeWlExpire,
	nvram.ARNvramIndexTypeWlExpireTmp,
	nvram.ARNvramIndexTypeWlBwEnabled,
	nvram.ARNvramIndexTypeWlBwDl,
	nvram.ARNvramIndexTypeWlBwUl,
}
var guestSlots = []int{1, 2, 3}

// bandUnit is one `(band, unit)` item of the identity wifi map.
type bandUnit struct {
	band wifi.ARWiFiBand
	unit int
}

// wifiItems returns the wifi map items in the Python dict order.
//
// Go maps are unordered, so the order comes from `bands`
// (`ARDeviceIdentity.WifiBands()`); without it the items are sorted by
// unit, the order the identity normally builds them in.
func wifiItems(m map[wifi.ARWiFiBand]int, bands []wifi.ARWiFiBand) []bandUnit {
	items := make([]bandUnit, 0, len(m))
	if len(bands) > 0 {
		for _, band := range bands {
			if unit, ok := m[band]; ok {
				items = append(items, bandUnit{band, unit})
			}
		}
		return items
	}
	for band, unit := range m {
		items = append(items, bandUnit{band, unit})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].unit != items[j].unit {
			return items[i].unit < items[j].unit
		}
		return items[i].band < items[j].band
	})
	return items
}

// guestIndex renders the `{unit}.{slot}` guest index.
func guestIndex(unit, slot int) string {
	return fmt.Sprintf("%d.%d", unit, slot)
}

// LegacyFetch mirrors `legacy.fetch`: fetch the per-band main and guest
// nvram for all bands. `bands` gives the dict order of `wifiMap` (see
// wifiItems; nil sorts by unit).
func LegacyFetch(ctx context.Context, callback registry.ReadFunc, wifiMap map[wifi.ARWiFiBand]int, bands []wifi.ARWiFiBand) (any, error) {
	if len(wifiMap) == 0 {
		return nil, nil
	}

	var items []endpoint.ARHookItem
	for _, item := range wifiItems(wifiMap, bands) {
		for _, kind := range mainKeys {
			items = append(items, nvram.NewARNvramIndexSource(kind, item.unit))
		}
		for _, slot := range guestSlots {
			for _, kind := range guestKeys {
				items = append(items, nvram.NewARNvramIndexSource(kind, guestIndex(item.unit, slot)))
			}
		}
	}

	return callback(ctx, endpoint.AREndpointFetchData, endpoint.HookRequest(items...))
}

// newNetwork mirrors `_new_network`: build the network-level fields for a
// new SSID group.
func newNetwork(
	data map[string]any,
	index any,
	ssid *identifiers.Ssid,
	maclist nvram.ARNvramIndexType,
) map[ARNetworkField]any {
	get := func(kind nvram.ARNvramIndexType) any {
		return data[kind.Key(index)]
	}

	fields := map[ARNetworkField]any{
		ARNetworkFieldSsid: ssid,
		// Aggregated from every band in the group by `_group`; a multi-band
		// network is on if any of its bands is on
		ARNetworkFieldEnabled:   false,
		ARNetworkFieldHidden:    boolOr(get(nvram.ARNvramIndexTypeWlClosed)),
		ARNetworkFieldApIsolate: boolOr(get(nvram.ARNvramIndexTypeWlApIsolate)),
		ARNetworkFieldBands:     []wifi.ARWiFiBand{},
		ARNetworkFieldSecurity:  map[wifi.ARWiFiBand]map[ARNetworkField]any{},
	}

	if password := identifiers.PasswordFromValueSafe(get(nvram.ARNvramIndexTypeWlWpaPsk)); password != nil {
		fields[ARNetworkFieldPassword] = password
	}

	// Guest-only nvram (absent on the main network -> skipped)
	if lanAccess := converters.RawToBool(get(nvram.ARNvramIndexTypeWlLanaccess)); lanAccess != nil {
		fields[ARNetworkFieldLanAccess] = *lanAccess
	}
	if expire := converters.RawToInt(get(nvram.ARNvramIndexTypeWlExpire)); expire != nil {
		fields[ARNetworkFieldExpire] = *expire
	}
	if remaining := converters.RawToInt(get(nvram.ARNvramIndexTypeWlExpireTmp)); remaining != nil {
		fields[ARNetworkFieldExpireRemaining] = *remaining
	}

	for k, v := range BandwidthLimit(
		get(nvram.ARNvramIndexTypeWlBwEnabled),
		get(nvram.ARNvramIndexTypeWlBwDl),
		get(nvram.ARNvramIndexTypeWlBwUl),
	) {
		fields[k] = v
	}

	if mode := MacFilterMode(get(nvram.ARNvramIndexTypeWlMacmode)); mode != nil {
		fields[ARNetworkFieldMacFilterMode] = *mode
	}
	if macs := ReadMacList(get(maclist)); len(macs) > 0 {
		fields[ARNetworkFieldMacFilterList] = macs
	}

	return fields
}

// legacyBandSpec is one `(band, unit, slot)` item of `_group`.
type legacyBandSpec struct {
	band wifi.ARWiFiBand
	unit int
	slot *int
}

// group mirrors `_group`: group per-band configs by SSID into networks.
func group(
	data map[string]any,
	bandSpecs []legacyBandSpec,
	enableKey nvram.ARNvramIndexType,
	maclist nvram.ARNvramIndexType,
) []map[ARNetworkField]any {
	groups := map[string]map[ARNetworkField]any{}
	var order []string
	units := map[string][]ARNetworkHandleUnit{}

	for _, spec := range bandSpecs {
		var index any = spec.unit
		if spec.slot != nil {
			index = guestIndex(spec.unit, *spec.slot)
		}
		rawSSID := data[nvram.ARNvramIndexTypeWlSsid.Key(index)]
		ssid := identifiers.SsidFromValueSafe(rawSSID)
		if ssid == nil {
			continue
		}

		key := converters.PyStr(rawSSID)
		network, ok := groups[key]
		if !ok {
			// Network-level fields come from the first band of the group;
			// bands sharing an SSID share these settings
			network = newNetwork(data, index, ssid, maclist)
			groups[key] = network
			order = append(order, key)
			units[key] = []ARNetworkHandleUnit{}
		}

		units[key] = append(units[key], ARNetworkHandleUnit{Unit: spec.unit, Slot: spec.slot})

		// Enable is per-band (main uses the radio state); the network is on
		// when any of its bands is on, so a single disabled band never hides it
		if b := converters.RawToBool(data[enableKey.Key(index)]); b != nil && *b {
			network[ARNetworkFieldEnabled] = true
		}

		network[ARNetworkFieldBands] = append(network[ARNetworkFieldBands].([]wifi.ARWiFiBand), spec.band)
		bandSecurity := map[ARNetworkField]any{
			ARNetworkFieldAuth: wifi.ARWiFiAuthModeFromValue(
				data[nvram.ARNvramIndexTypeWlAuthMode.Key(index)],
			),
		}
		if cipher := converters.RawToStr(data[nvram.ARNvramIndexTypeWlCrypto.Key(index)]); cipher != nil && *cipher != "" {
			bandSecurity[ARNetworkFieldCipher] = *cipher
		}
		network[ARNetworkFieldSecurity].(map[wifi.ARWiFiBand]map[ARNetworkField]any)[spec.band] = bandSecurity
	}

	for _, key := range order {
		groups[key][ARNetworkFieldHandle] = ARNetworkHandle{
			Backend: ARNetworkBackendLegacy, Units: units[key],
		}
	}

	result := make([]map[ARNetworkField]any, 0, len(order))
	for _, key := range order {
		result = append(result, groups[key])
	}
	return result
}

// LegacyTranslate mirrors `legacy.translate`: translate raw per-band nvram
// into networks grouped by type. `bands` gives the dict order of
// `wifiMap` (see wifiItems; nil sorts by unit).
func LegacyTranslate(data map[string]any, wifiMap map[wifi.ARWiFiBand]int, bands []wifi.ARWiFiBand) map[ARNetworkType][]map[ARNetworkField]any {
	result := map[ARNetworkType][]map[ARNetworkField]any{}
	items := wifiItems(wifiMap, bands)

	mainSpecs := make([]legacyBandSpec, 0, len(items))
	for _, item := range items {
		mainSpecs = append(mainSpecs, legacyBandSpec{item.band, item.unit, nil})
	}
	main := group(
		data,
		mainSpecs,
		nvram.ARNvramIndexTypeWlRadio,
		nvram.ARNvramIndexTypeWlMaclistX,
	)
	if len(main) > 0 {
		result[ARNetworkTypeMainfh] = main
	}

	var guestSpecs []legacyBandSpec
	for _, slot := range guestSlots {
		for _, item := range items {
			s := slot
			guestSpecs = append(guestSpecs, legacyBandSpec{item.band, item.unit, &s})
		}
	}
	guests := group(
		data,
		guestSpecs,
		nvram.ARNvramIndexTypeWlBssEnabled,
		nvram.ARNvramIndexTypeWlMaclist,
	)
	if len(guests) > 0 {
		result[ARNetworkTypeGuest] = guests
	}

	return result
}

// LegacyBuildTogglePayload mirrors `legacy.build_toggle_payload`: build
// the `(rc_service, arguments)` to enable/disable a network. The
// arguments keep the Python dict insertion order.
func LegacyBuildTogglePayload(handle ARNetworkHandle, state bool) (string, [][2]any) {
	arguments := [][2]any{}
	hasGuest := false
	for _, u := range handle.Units {
		if u.Slot == nil {
			arguments = setArg(arguments, nvram.ARNvramIndexTypeWlRadio.Key(u.Unit), boolInt(state))
			continue
		}
		hasGuest = true
		index := guestIndex(u.Unit, *u.Slot)
		arguments = setArg(arguments, nvram.ARNvramIndexTypeWlBssEnabled.Key(index), boolInt(state))
		if state {
			arguments = setArg(arguments, nvram.ARNvramIndexTypeWlExpire.Key(index), 0)
		}
	}

	rcService := "restart_wireless"
	if hasGuest {
		rcService = "restart_wireless;restart_firewall"
	}
	return rcService, arguments
}

// boolOr mirrors `raw_to_bool(value) or False`.
func boolOr(value any) bool {
	b := converters.RawToBool(value)
	return b != nil && *b
}

// boolInt mirrors `int(state)`.
func boolInt(state bool) int {
	if state {
		return 1
	}
	return 0
}

// setArg mirrors `arguments[key] = value` on an insertion-ordered dict.
func setArg(arguments [][2]any, key string, value any) [][2]any {
	for i, kv := range arguments {
		if kv[0] == key {
			arguments[i][1] = value
			return arguments
		}
	}
	return append(arguments, [2]any{key, value})
}
