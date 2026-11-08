// SDN backend for the network module.
//
// Reads the Software-Defined Network profiles (main + guest/IoT) on modern
// firmware and builds the per-type network profiles.

package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// `sdn_rl` column indices (see `set_sdn_profile` in the firmware)
const (
	colIdx    = 0
	colName   = 1
	colEnable = 2
	colApgIdx = 5
)

// SDN profile names that store their AP group under the `apm` prefix
var mainNames = []string{"MAINFH", "MAINBH"}

// Minimum `>`-columns in a security / dut_list row to be usable
const (
	securityCols = 4
	dutCols      = 2
)

// `bw_limit` row columns (`<enabled>up>down`)
const (
	bwEnabledCol  = 0
	bwUploadCol   = 1
	bwDownloadCol = 2
)

// Per-profile AP-group keys we read and parse
var apKeys = []nvram.ARNvramIndexType{
	nvram.ARNvramIndexTypeApEnable,
	nvram.ARNvramIndexTypeApSsid,
	nvram.ARNvramIndexTypeApHideSsid,
	nvram.ARNvramIndexTypeApSecurity,
	nvram.ARNvramIndexTypeApApIsolate,
	nvram.ARNvramIndexTypeApMacmode,
	nvram.ARNvramIndexTypeApMlo,
	nvram.ARNvramIndexTypeApMaclist,
	nvram.ARNvramIndexTypeApDutList,
	nvram.ARNvramIndexTypeAp11be,
	nvram.ARNvramIndexTypeApTimesched,
	nvram.ARNvramIndexTypeApSched,
	nvram.ARNvramIndexTypeApExpiretime,
	nvram.ARNvramIndexTypeApBwLimit,
}

// apIndex mirrors `_ap_index`: build the AP-group template index (`g1`
// for `apg1`).
func apIndex(prefix *string, idx *int) string {
	p := ""
	if prefix != nil {
		p = strings.TrimPrefix(*prefix, "ap")
	}
	return p + optInt(idx)
}

// WiFi band bitmask -> band (`wifi_band_options_cb`): a band matches any of
// its bits (single vs dual-band variants)
var bandBits = map[wifi.ARWiFiBand]int{
	wifi.ARWiFiBandBand2g1: 1,
	wifi.ARWiFiBandBand5g1: 2 | 4,
	wifi.ARWiFiBandBand5g2: 8,
	wifi.ARWiFiBandBand6g1: 16 | 32,
	wifi.ARWiFiBandBand6g2: 64,
}

// rows mirrors `_rows`: decode a char-encoded rule-list into rows of
// `>`-separated columns.
func rows(raw any) [][]string {
	var out [][]string
	for _, row := range readers.SplitRows(raw) {
		if row != "" {
			out = append(out, strings.Split(row, ">"))
		}
	}
	return out
}

// sdnProfile is one parsed `sdn_rl` row: `(name, prefix, apg_idx,
// sdn_idx, enabled)`.
type sdnProfile struct {
	name    string
	prefix  string
	apgIdx  int
	sdnIdx  int
	enabled bool
}

// parseSdnRl mirrors `_parse_sdn_rl`: parse `sdn_rl` into `(name, prefix,
// apg_idx, sdn_idx, enabled)`.
func parseSdnRl(raw any) []sdnProfile {
	var profiles []sdnProfile
	for _, cols := range rows(raw) {
		if len(cols) <= colApgIdx || cols[colIdx] == "0" {
			continue
		}
		name := cols[colName]
		apgIdx := converters.RawToInt(cols[colApgIdx])
		sdnIdx := converters.RawToInt(cols[colIdx])
		if apgIdx == nil || sdnIdx == nil {
			continue
		}
		prefix := "apg"
		for _, n := range mainNames {
			if name == n {
				prefix = "apm"
			}
		}
		enabled := boolOr(cols[colEnable])
		profiles = append(profiles, sdnProfile{name, prefix, *apgIdx, *sdnIdx, enabled})
	}
	return profiles
}

// securityEntry is one `(bitmask, auth, cipher, password)` security row.
type securityEntry struct {
	bitmask  int
	auth     string
	cipher   string
	password string
}

// parseSecurity mirrors `_parse_security`: decode `security` into the
// network password and per-band auth/cipher.
//
// The password is shared by the whole network (each band group repeats it);
// only auth and cipher vary per band (6G forces SAE).
func parseSecurity(raw any, bands []wifi.ARWiFiBand) (*identifiers.Password, map[wifi.ARWiFiBand]map[ARNetworkField]any) {
	var entries []securityEntry
	for _, cols := range rows(raw) {
		if len(cols) >= securityCols {
			bitmask := 0
			if v := converters.RawToInt(cols[0]); v != nil {
				bitmask = *v
			}
			entries = append(entries, securityEntry{bitmask, cols[1], cols[2], cols[3]})
		}
	}

	var password *identifiers.Password
	for _, entry := range entries {
		password = identifiers.PasswordFromValueSafe(entry.password)
		if password != nil {
			break
		}
	}

	perBand := map[wifi.ARWiFiBand]map[ARNetworkField]any{}
	for _, band := range bands {
		bits := bandBits[band]
		for _, entry := range entries {
			if entry.bitmask&bits != 0 {
				perBand[band] = map[ARNetworkField]any{
					ARNetworkFieldAuth:   wifi.ARWiFiAuthModeFromValue(entry.auth),
					ARNetworkFieldCipher: entry.cipher,
				}
				break
			}
		}
	}

	return password, perBand
}

// dutBands mirrors `_dut_bands`: decode `dut_list` into the device bands
// the profile is bound to.
func dutBands(raw any, bands []wifi.ARWiFiBand) []wifi.ARWiFiBand {
	union := 0
	for _, cols := range rows(raw) {
		if len(cols) >= dutCols {
			if v := converters.RawToInt(cols[1]); v != nil {
				union |= *v
			}
		}
	}

	var out []wifi.ARWiFiBand
	for _, band := range bands {
		if bandBits[band]&union != 0 {
			out = append(out, band)
		}
	}
	return out
}

// bandwidth mirrors `_bandwidth`: decode `bw_limit` (`<enabled>up>down`,
// Kib/s) into rate fields.
func bandwidth(raw any) map[ARNetworkField]any {
	r := rows(raw)
	if len(r) == 0 {
		return map[ARNetworkField]any{}
	}

	row := r[0]
	col := func(i int) any {
		if len(row) > i {
			return row[i]
		}
		return nil
	}
	return BandwidthLimit(col(bwEnabledCol), col(bwDownloadCol), col(bwUploadCol))
}

// schedule mirrors `_schedule`: build the scheduling fields, empty when
// scheduling is off.
func schedule(timesched, sched, expiretime any) map[ARNetworkField]any {
	mode := ARNetworkScheduleFromValue(timesched)
	if mode == ARNetworkScheduleUnknown || mode == ARNetworkScheduleDisabled {
		return map[ARNetworkField]any{}
	}

	fields := map[ARNetworkField]any{ARNetworkFieldScheduleMode: mode}
	if weekly := converters.RawToStr(sched); weekly != nil && *weekly != "" {
		fields[ARNetworkFieldSchedule] = *weekly
	}
	if expiry := converters.RawToStr(expiretime); expiry != nil && *expiry != "" {
		fields[ARNetworkFieldExpiry] = *expiry
	}
	return fields
}

// apRequest mirrors `_ap_request`: build the appGet request for the
// referenced AP groups.
func apRequest(profiles []sdnProfile) string {
	var items []endpoint.ARHookItem
	for _, p := range profiles {
		prefix, idx := p.prefix, p.apgIdx
		for _, kind := range apKeys {
			items = append(items, nvram.NewARNvramIndexSource(kind, apIndex(&prefix, &idx)))
		}
	}
	return endpoint.HookRequest(items...)
}

// SdnFetchSdnRl mirrors `sdn.fetch_sdn_rl`: fetch the raw `sdn_rl` rule
// list, or nil if unavailable.
func SdnFetchSdnRl(ctx context.Context, callback registry.ReadFunc) (any, error) {
	request := endpoint.HookRequest(nvram.ARNvramTypeSdnRl)
	data, err := callback(ctx, endpoint.AREndpointFetchData, request)
	if err != nil {
		return nil, err
	}
	return endpoint.HookValue(data, nvram.ARNvramTypeSdnRl), nil
}

// SdnFetch mirrors `sdn.fetch`: fetch `sdn_rl`, then the AP groups it
// references.
func SdnFetch(ctx context.Context, callback registry.ReadFunc) (any, error) {
	sdnRl, err := SdnFetchSdnRl(ctx, callback)
	if err != nil || sdnRl == nil {
		return nil, err
	}

	profiles := parseSdnRl(sdnRl)
	if len(profiles) == 0 {
		return map[string]any{string(nvram.ARNvramTypeSdnRl): sdnRl}, nil
	}

	second, err := callback(ctx, endpoint.AREndpointFetchData, apRequest(profiles))
	if err != nil {
		return nil, err
	}
	result := map[string]any{string(nvram.ARNvramTypeSdnRl): sdnRl}
	if apData, ok := second.(map[string]any); ok {
		for k, v := range apData {
			result[k] = v
		}
	}
	return result, nil
}

// buildNetwork mirrors `_build_network`: build the network profile dict
// for one AP group.
func buildNetwork(
	data map[string]any,
	handle ARNetworkHandle,
	bands []wifi.ARWiFiBand,
	sdnEnabled bool,
) map[ARNetworkField]any {
	index := apIndex(handle.ApPrefix, handle.ApIdx)

	get := func(kind nvram.ARNvramIndexType) any {
		return data[kind.Key(index)]
	}

	// A network is on only when both the SDN profile and its AP group are on
	enabled := sdnEnabled && boolOr(get(nvram.ARNvramIndexTypeApEnable))

	fields := map[ARNetworkField]any{
		ARNetworkFieldEnabled:   enabled,
		ARNetworkFieldHandle:    handle,
		ARNetworkFieldHidden:    boolOr(get(nvram.ARNvramIndexTypeApHideSsid)),
		ARNetworkFieldWifi7:     boolOr(get(nvram.ARNvramIndexTypeAp11be)),
		ARNetworkFieldApIsolate: boolOr(get(nvram.ARNvramIndexTypeApApIsolate)),
	}

	if ssid := identifiers.SsidFromValueSafe(get(nvram.ARNvramIndexTypeApSsid)); ssid != nil {
		fields[ARNetworkFieldSsid] = ssid
	}

	if mlo := converters.RawToInt(get(nvram.ARNvramIndexTypeApMlo)); mlo != nil {
		fields[ARNetworkFieldMlo] = *mlo
	}

	for k, v := range schedule(
		get(nvram.ARNvramIndexTypeApTimesched),
		get(nvram.ARNvramIndexTypeApSched),
		get(nvram.ARNvramIndexTypeApExpiretime),
	) {
		fields[k] = v
	}
	for k, v := range bandwidth(get(nvram.ARNvramIndexTypeApBwLimit)) {
		fields[k] = v
	}

	if mode := MacFilterMode(get(nvram.ARNvramIndexTypeApMacmode)); mode != nil {
		fields[ARNetworkFieldMacFilterMode] = *mode
	}
	if macList := ReadMacList(get(nvram.ARNvramIndexTypeApMaclist)); len(macList) > 0 {
		fields[ARNetworkFieldMacFilterList] = macList
	}

	dut := dutBands(get(nvram.ARNvramIndexTypeApDutList), bands)
	if len(dut) > 0 {
		fields[ARNetworkFieldBands] = dut
	}

	// Security only for the bands the network is actually bound to
	password, security := parseSecurity(get(nvram.ARNvramIndexTypeApSecurity), dut)
	if password != nil {
		fields[ARNetworkFieldPassword] = password
	}
	if len(security) > 0 {
		fields[ARNetworkFieldSecurity] = security
	}

	return fields
}

// SdnTranslate mirrors `sdn.translate`: translate raw SDN data into
// networks grouped by type.
func SdnTranslate(data map[string]any, bands []wifi.ARWiFiBand) map[ARNetworkType][]map[ARNetworkField]any {
	result := map[ARNetworkType][]map[ARNetworkField]any{}
	for _, p := range parseSdnRl(data[string(nvram.ARNvramTypeSdnRl)]) {
		sdnIdx, prefix, apgIdx := p.sdnIdx, p.prefix, p.apgIdx
		handle := ARNetworkHandle{
			Backend:  ARNetworkBackendSdn,
			SdnIdx:   &sdnIdx,
			ApPrefix: &prefix,
			ApIdx:    &apgIdx,
		}
		network := buildNetwork(data, handle, bands, p.enabled)
		netType := ARNetworkTypeFromValue(p.name)
		result[netType] = append(result[netType], network)
	}

	return result
}

// SdnBuildTogglePayload mirrors `sdn.build_toggle_payload`: build the
// `(rc_service, arguments)` to enable/disable an SDN profile. The
// arguments keep the Python dict insertion order.
func SdnBuildTogglePayload(rawSdnRl any, handle ARNetworkHandle, state bool) (string, [][2]any) {
	r := rows(rawSdnRl)
	for _, cols := range r {
		if idx := converters.RawToInt(cols[colIdx]); len(cols) > colEnable && eqInt(idx, handle.SdnIdx) {
			if state {
				cols[colEnable] = "1"
			} else {
				cols[colEnable] = "0"
			}
			break
		}
	}

	var b strings.Builder
	for _, cols := range r {
		b.WriteString("<" + strings.Join(cols, ">"))
	}
	rcService := fmt.Sprintf("restart_wireless;restart_sdn %s;", optInt(handle.SdnIdx))
	apEnable := nvram.ARNvramIndexTypeApEnable.Key(apIndex(handle.ApPrefix, handle.ApIdx))
	arguments := [][2]any{
		{string(nvram.ARNvramTypeSdnRl), b.String()},
	}
	arguments = setArg(arguments, apEnable, boolInt(state))
	return rcService, arguments
}
