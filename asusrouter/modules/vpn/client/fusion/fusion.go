// Package fusion mirrors asusrouter/modules/vpn/client/fusion.py: the VPN
// Fusion client backend for the VPN module.
//
// Python keeps the Fusion and classic backends as sibling modules with the
// same function names (`nvram_items`, `translate`, ...), so in Go each
// backend module is its own package (`fusion.Translate` mirrors
// `fusion.translate`).
package fusion

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// StatusHook is the live client status hook
const StatusHook = endpoint.ARHookVpncStatus

// NondefWanHook is the default-WAN support hook
const NondefWanHook = endpoint.ARHookVpncNondefWanProfiles

// WGUnits are the WireGuard client units to probe; OpenVPN client config
// is in the clientlist
var WGUnits = []int{1, 2, 3, 4, 5}

// clientlist `>`-field positions
const (
	fName     = 0
	fProto    = 1
	fServer   = 2
	fUsername = 3
	fPassword = 4
	fActivate = 5
	fVpncIdx  = 6
	fRegion   = 7
)

// get_vpnc_status `>`-field positions
const (
	sStatus  = 0
	sReason  = 1
	sVpncIdx = 2
)

// Live status code -> unified state
var stateMap = map[int]vpn.ARVpnState{
	0: vpn.ARVpnStateError,
	1: vpn.ARVpnStateConnecting,
	2: vpn.ARVpnStateConnected,
	4: vpn.ARVpnStateError,
	5: vpn.ARVpnStateDisconnected,
	6: vpn.ARVpnStateError,
}

type infoRow struct {
	index     int
	field     vpn.ARVpnClientField
	converter func(any) any
}

// clientlist string field -> profile field, converter
var info = []infoRow{
	{fUsername, vpn.ARVpnClientFieldUsername, converters.AsAny(converters.RawToStr)},
	{fPassword, vpn.ARVpnClientFieldPassword, converters.AsAny(identifiers.PasswordFromValueSafe)},
	{fRegion, vpn.ARVpnClientFieldRegion, converters.AsAny(converters.RawToStr)},
}

// WG is the WireGuard client nvram key -> field, converter table
// (`wgc{unit}_*`). Python `_WG`; exported because classic.py imports it.
var WG = []readers.TableEntry[nvram.ARNvramIndexType, vpn.ARVpnClientField]{
	{
		Key:       nvram.ARNvramIndexTypeWgcPriv,
		Field:     vpn.ARVpnClientFieldPrivateKey,
		Converter: converters.AsAny(identifiers.PasswordFromValueSafe),
	},
	{
		Key:       nvram.ARNvramIndexTypeWgcAddr,
		Field:     vpn.ARVpnClientFieldAddress,
		Converter: converters.AsAny(identifiers.IpInterfaceFromValueSafe),
	},
	{
		Key:       nvram.ARNvramIndexTypeWgcDns,
		Field:     vpn.ARVpnClientFieldDns,
		Converter: converters.AsAny(identifiers.IpAddressFromValueSafe),
	},
	{Key: nvram.ARNvramIndexTypeWgcMtu, Field: vpn.ARVpnClientFieldMtu, Converter: converters.AsAny(converters.RawToInt)},
	{Key: nvram.ARNvramIndexTypeWgcPpub, Field: vpn.ARVpnClientFieldPublicKey, Converter: converters.AsAny(converters.RawToStr)},
	{
		Key:       nvram.ARNvramIndexTypeWgcPsk,
		Field:     vpn.ARVpnClientFieldPsk,
		Converter: converters.AsAny(identifiers.PasswordFromValueSafe),
	},
	{
		Key:       nvram.ARNvramIndexTypeWgcAips,
		Field:     vpn.ARVpnClientFieldAllowedIps,
		Converter: func(v any) any { return identifiers.ReadIpInterfaceList(v) },
	},
	{
		Key:       nvram.ARNvramIndexTypeWgcEpAddr,
		Field:     vpn.ARVpnClientFieldEndpointAddress,
		Converter: converters.AsAny(identifiers.IpAddressFromValueSafe),
	},
	{
		Key:       nvram.ARNvramIndexTypeWgcEpPort,
		Field:     vpn.ARVpnClientFieldEndpointPort,
		Converter: converters.AsAny(converters.RawToInt),
	},
	{Key: nvram.ARNvramIndexTypeWgcAlive, Field: vpn.ARVpnClientFieldKeepalive, Converter: converters.AsAny(converters.RawToInt)},
	{Key: nvram.ARNvramIndexTypeWgcNat, Field: vpn.ARVpnClientFieldNat, Converter: converters.AsAny(converters.RawToBool)},
}

// statusEntry is a `(state, reason)` status tuple.
type statusEntry struct {
	state  vpn.ARVpnState
	reason *int
}

// context mirrors `_Context`: router-global data shared by every profile
// in one translate pass.
type context struct {
	data        map[string]any
	pptpOptions []string
	status      map[int]statusEntry
	support     map[int]bool
	defaultWan  *int
}

// NvramItems mirrors `nvram_items`: the nvram items required to read all
// Fusion client profiles.
func NvramItems() []nvram.ARNvramItem {
	items := []nvram.ARNvramItem{
		nvram.ARNvramTypeVpncClientlist,
		nvram.ARNvramTypeVpncPptpOptions,
		nvram.ARNvramTypeVpncDefaultWan,
	}
	for _, unit := range WGUnits {
		for _, row := range WG {
			items = append(items, nvram.NewARNvramIndexSource(row.Key, unit))
		}
	}
	return items
}

// fieldValue is `get_field` as a Python value (nil for None).
func fieldValue(parts []string, index int) any {
	if f := readers.GetField(parts, index); f != nil {
		return *f
	}
	return nil
}

// fieldEquals reports `get_field(parts, index) == want`.
func fieldEquals(parts []string, index int, want string) bool {
	f := readers.GetField(parts, index)
	return f != nil && *f == want
}

// readStatus mirrors `_status`: map vpnc_idx -> (state, reason) from the
// status hook.
func readStatus(data map[string]any) map[int]statusEntry {
	status := map[int]statusEntry{}
	for _, row := range readers.SplitRows(data[string(StatusHook)]) {
		parts := strings.Split(row, ">")
		vpncIdx := converters.RawToInt(fieldValue(parts, sVpncIdx))
		if vpncIdx == nil {
			continue
		}
		code := converters.RawToInt(fieldValue(parts, sStatus))
		state := vpn.ARVpnStateUnknown
		if code != nil {
			if s, ok := stateMap[*code]; ok {
				state = s
			}
		}
		status[*vpncIdx] = statusEntry{state, converters.RawToInt(fieldValue(parts, sReason))}
	}
	return status
}

// defaultWanSupport mirrors `_default_wan_support`: map vpnc_idx ->
// whether the profile supports default-WAN routing.
func defaultWanSupport(data map[string]any) map[int]bool {
	support := map[int]bool{}
	for _, row := range readers.SplitRows(data[string(NondefWanHook)]) {
		parts := strings.Split(row, ">")
		vpncIdx := converters.RawToInt(fieldValue(parts, 0))
		if vpncIdx == nil {
			continue
		}
		// `0` means the profile is allowed on the default WAN
		support[*vpncIdx] = fieldEquals(parts, 1, "0")
	}
	return support
}

// wireguard mirrors `_wireguard`: read the `wgc{unit}_*` peer config of a
// WireGuard client.
func wireguard(data map[string]any, unit int) map[vpn.ARVpnClientField]any {
	return readers.ReadTable(data, WG, func(kind nvram.ARNvramIndexType) string { return kind.Key(unit) })
}

// baseFields mirrors `_base_fields`: the clientlist-derived fields of a
// profile.
func baseFields(parts []string, position int) map[vpn.ARVpnClientField]any {
	protocol := vpn.ARVpnProtocolFromValue(fieldValue(parts, fProto))
	var server any
	if serverInt := converters.RawToInt(fieldValue(parts, fServer)); serverInt != nil {
		server = *serverInt
	} else {
		server = fieldValue(parts, fServer)
	}

	var name any
	if n := converters.RawToStr(fieldValue(parts, fName)); n != nil {
		name = *n
	}
	fields := map[vpn.ARVpnClientField]any{
		vpn.ARVpnClientFieldName:     name,
		vpn.ARVpnClientFieldProtocol: protocol,
		vpn.ARVpnClientFieldUnit:     position,
		vpn.ARVpnClientFieldEnabled:  fieldEquals(parts, fActivate, "1"),
	}
	if server != nil && server != "" {
		fields[vpn.ARVpnClientFieldServer] = server
	}

	if vpncIdx := converters.RawToInt(fieldValue(parts, fVpncIdx)); vpncIdx != nil {
		fields[vpn.ARVpnClientFieldVpncIndex] = *vpncIdx
	}

	for _, row := range info {
		if value := converters.RawConvert(fieldValue(parts, row.index), row.converter); value != nil {
			fields[row.field] = value
		}
	}

	return fields
}

// applyRouting mirrors `_apply_routing`: add live status and default-WAN
// routing keyed by vpnc_idx.
func applyRouting(fields map[vpn.ARVpnClientField]any, vpncIdx int, ctx *context) {
	if state, ok := ctx.status[vpncIdx]; ok {
		fields[vpn.ARVpnClientFieldState] = state.state
		// The reason code is only meaningful for a failed connection
		if state.state == vpn.ARVpnStateError && state.reason != nil {
			fields[vpn.ARVpnClientFieldStateReason] = *state.reason
		}
	}
	if support, ok := ctx.support[vpncIdx]; ok {
		fields[vpn.ARVpnClientFieldDefaultWanSupport] = support
	}
	if ctx.defaultWan != nil {
		fields[vpn.ARVpnClientFieldDefaultWan] = vpncIdx == *ctx.defaultWan
	}
}

// profile mirrors `_profile`: a single client profile, or nil when the
// row is empty.
func profile(row string, position int, ctx *context) map[vpn.ARVpnClientField]any {
	parts := strings.Split(row, ">")
	if converters.RawToStr(fieldValue(parts, fName)) == nil {
		return nil
	}

	fields := baseFields(parts, position)

	var option *string
	if position < len(ctx.pptpOptions) {
		option = &ctx.pptpOptions[position]
	}
	if option != nil && *option != "" && *option != "auto" {
		fields[vpn.ARVpnClientFieldPptpOptions] = *option
	}

	if vpncIdx, ok := fields[vpn.ARVpnClientFieldVpncIndex].(int); ok {
		applyRouting(fields, vpncIdx, ctx)
	}

	if server, isInt := fields[vpn.ARVpnClientFieldServer].(int); isInt &&
		fields[vpn.ARVpnClientFieldProtocol] == vpn.ARVpnProtocolWireguard {
		for k, v := range wireguard(ctx.data, server) {
			fields[k] = v
		}
	}

	return fields
}

// Translate mirrors `translate`: raw data into client profiles grouped by
// protocol.
func Translate(data map[string]any) map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any {
	result := map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any{}
	rows := readers.SplitRows(data[string(nvram.ARNvramTypeVpncClientlist)])
	if len(rows) == 0 {
		return result
	}

	// Parallel list; a leading empty aligns it one ahead of the clientlist
	pptpOptions := readers.SplitRows(data[string(nvram.ARNvramTypeVpncPptpOptions)])
	if len(pptpOptions) > 0 {
		pptpOptions = pptpOptions[1:]
	}
	ctx := &context{
		data:        data,
		pptpOptions: pptpOptions,
		status:      readStatus(data),
		support:     defaultWanSupport(data),
		defaultWan:  converters.RawToInt(data[string(nvram.ARNvramTypeVpncDefaultWan)]),
	}

	position := 0
	for _, row := range rows {
		if row == "" {
			continue
		}
		fields := profile(row, position, ctx)
		vpncIdx := position
		if fields != nil {
			if idx, ok := fields[vpn.ARVpnClientFieldVpncIndex].(int); ok {
				vpncIdx = idx
			}
		}
		position++
		if fields == nil {
			continue
		}
		protocol := fields[vpn.ARVpnClientFieldProtocol].(vpn.ARVpnProtocol)
		if result[protocol] == nil {
			result[protocol] = map[int]map[vpn.ARVpnClientField]any{}
		}
		result[protocol][vpncIdx] = fields
	}

	return result
}

// BuildTogglePayload mirrors `build_toggle_payload`: the `(services,
// arguments)` to toggle a Fusion client, or ok=false.
//
// Locates the profile of `protocol` with server unit `unit`, flips its
// activate flag in a rewritten `vpnc_clientlist`, and targets it by
// position (`vpnc_unit`), mirroring the web UI's `restart_vpnc` /
// `stop_vpnc` toggle.
func BuildTogglePayload(clientlistRaw any, protocol vpn.ARVpnProtocol, unit int, state bool) ([]service.ARServiceInput, [][2]any, bool) {
	rows := readers.SplitRows(clientlistRaw)
	position := 0
	target := -1
	for index, row := range rows {
		if row == "" {
			continue
		}
		parts := strings.Split(row, ">")
		server := converters.RawToInt(fieldValue(parts, fServer))
		if vpn.ARVpnProtocolFromValue(fieldValue(parts, fProto)) == protocol &&
			server != nil && *server == unit &&
			len(parts) > fActivate {
			if state {
				parts[fActivate] = "1"
			} else {
				parts[fActivate] = "0"
			}
			rows[index] = strings.Join(parts, ">")
			target = position
			break
		}
		position++
	}

	if target < 0 {
		return nil, nil, false
	}

	svc := common.ARServiceVpncStop
	if state {
		svc = common.ARServiceVpncRestart
	}
	arguments := [][2]any{
		{string(nvram.ARNvramTypeVpncUnit), target},
		{string(nvram.ARNvramTypeVpncClientlist), strings.Join(rows, "<")},
	}
	return []service.ARServiceInput{svc}, arguments, true
}
