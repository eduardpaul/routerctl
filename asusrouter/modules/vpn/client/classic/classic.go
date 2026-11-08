// Package classic mirrors asusrouter/modules/vpn/client/classic.py: the
// classic (non-Fusion) VPN client backend for the VPN module.
//
// Python keeps the Fusion and classic backends as sibling modules with the
// same function names (`nvram_items`, `translate`, ...), so in Go each
// backend module is its own package (`classic.Translate` mirrors
// `classic.translate`).
package classic

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client/fusion"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Units are the client units exposed by the firmware
var Units = []int{1, 2, 3, 4, 5}

// StatusKey is the key under which the source stashes the ajax_vpn_status
// result
const StatusKey = "_vpn_status"

// Values that mean "no data" in the status endpoint
var empty = []string{"", "None"}

var (
	toInt = converters.AsAny(converters.RawToInt)
	toStr = converters.AsAny(converters.RawToStr)
)

// OpenVPN client nvram key -> field, converter (`vpn_client{unit}_*`)
var ovpn = []readers.TableEntry[nvram.ARNvramIndexType, vpn.ARVpnClientField]{
	{Key: nvram.ARNvramIndexTypeVpnClientDesc, Field: vpn.ARVpnClientFieldName, Converter: toStr},
	{Key: nvram.ARNvramIndexTypeVpnClientAddr, Field: vpn.ARVpnClientFieldServer, Converter: toStr},
	{
		Key:       nvram.ARNvramIndexTypeVpnClientPort,
		Field:     vpn.ARVpnClientFieldEndpointPort,
		Converter: toInt,
	},
	{
		Key:       nvram.ARNvramIndexTypeVpnClientUsername,
		Field:     vpn.ARVpnClientFieldUsername,
		Converter: toStr,
	},
}

type statRow struct {
	token     string
	field     vpn.ARVpnClientField
	converter func(any) any
	// re is the compiled `{token},(.*?)(?=>)` (RE2: the lookahead becomes
	// a trailing `>` outside the capture, which matches identically)
	re *regexp.Regexp
}

func stat(token string, field vpn.ARVpnClientField, converter func(any) any) statRow {
	return statRow{token, field, converter, regexp.MustCompile(regexp.QuoteMeta(token) + `,(.*?)>`)}
}

// vpn.cgi status token -> field, converter (parsed from `vpn_client{n}_status`)
var stats = []statRow{
	stat("Updated", vpn.ARVpnClientFieldConnectedSince, toStr),
	stat("TCP/UDP read bytes", vpn.ARVpnClientFieldRxBytes, toInt),
	stat("TCP/UDP write bytes", vpn.ARVpnClientFieldTxBytes, toInt),
	stat("TUN/TAP read bytes", vpn.ARVpnClientFieldTunTapRxBytes, toInt),
	stat("TUN/TAP write bytes", vpn.ARVpnClientFieldTunTapTxBytes, toInt),
	stat("Auth read bytes", vpn.ARVpnClientFieldAuthRxBytes, toInt),
	stat("pre-compress bytes", vpn.ARVpnClientFieldPreCompressBytes, toInt),
	stat("post-compress bytes", vpn.ARVpnClientFieldPostCompressBytes, toInt),
	stat(
		"pre-decompress bytes",
		vpn.ARVpnClientFieldPreDecompressBytes,
		toInt,
	),
	stat(
		"post-decompress bytes",
		vpn.ARVpnClientFieldPostDecompressBytes,
		toInt,
	),
}

// NvramItems mirrors `nvram_items`: the nvram items required to read
// every classic client unit.
func NvramItems() []nvram.ARNvramItem {
	items := []nvram.ARNvramItem{nvram.ARNvramTypeVpnClientEas}
	for _, unit := range Units {
		items = append(items, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeVpnClientState, unit))
		items = append(items, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeVpnClientErrno, unit))
		for _, row := range ovpn {
			items = append(items, nvram.NewARNvramIndexSource(row.Key, unit))
		}
		items = append(items, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWgcEnable, unit))
		for _, row := range fusion.WG {
			items = append(items, nvram.NewARNvramIndexSource(row.Key, unit))
		}
	}
	return items
}

// clean mirrors `_clean`: a status value, or nil when it is a placeholder.
func clean(value any) *string {
	text := converters.RawToStr(value)
	if text == nil {
		return nil
	}
	for _, e := range empty {
		if *text == e {
			return nil
		}
	}
	return text
}

// ip mirrors `_ip`: parse an IP, treating the unspecified address
// (all-zero) as absent.
func ip(value any) *identifiers.IpAddress {
	var cleaned any
	if c := clean(value); c != nil {
		cleaned = *c
	}
	addr := identifiers.IpAddressFromValueSafe(cleaned)
	if addr == nil || addr.ToInt().Sign() == 0 {
		return nil
	}
	return addr
}

// statusData mirrors `_status_data`: the ajax_vpn_status block the source
// stashed, if any.
func statusData(data map[string]any) map[string]any {
	if status, ok := data[StatusKey].(map[string]any); ok {
		return status
	}
	return map[string]any{}
}

// enabledUnits mirrors `_enabled_units`: parse `vpn_clientx_eas` into the
// set of active OpenVPN units.
func enabledUnits(data map[string]any) map[int]bool {
	units := map[int]bool{}
	text := converters.RawToStr(data[string(nvram.ARNvramTypeVpnClientEas)])
	if text == nil {
		return units
	}
	for _, part := range strings.Split(*text, ",") {
		if n := converters.RawToInt(part); n != nil && *n != 0 {
			units[*n] = true
		}
	}
	return units
}

var remoteRe = regexp.MustCompile(`REMOTE,(.*?)>`)

// parseStatus mirrors `_parse_status`: parse a `vpn_client{n}_status`
// string into live fields.
func parseStatus(raw any) map[vpn.ARVpnClientField]any {
	fields := map[vpn.ARVpnClientField]any{}
	text := clean(raw)
	if text == nil {
		return fields
	}

	if remote := remoteRe.FindStringSubmatch(*text); remote != nil {
		host, _, _ := strings.Cut(remote[1], ",")
		address, port := "", host
		if i := strings.LastIndex(host, ":"); i >= 0 {
			address, port = host[:i], host[i+1:]
		}
		target := address
		if target == "" {
			target = host
		}
		if addr := ip(target); addr != nil {
			fields[vpn.ARVpnClientFieldRemoteAddress] = *addr
		}
		var remotePort *int
		if address != "" {
			remotePort = converters.RawToInt(port)
		}
		if remotePort != nil {
			fields[vpn.ARVpnClientFieldRemotePort] = *remotePort
		}
	}

	for _, row := range stats {
		var matched any
		if match := row.re.FindStringSubmatch(*text); match != nil {
			matched = match[1]
		}
		if value := converters.RawConvert(matched, row.converter); value != nil {
			fields[row.field] = value
		}
	}

	return fields
}

// openvpn mirrors `_openvpn`: an OpenVPN client profile, or nil when the
// unit is empty.
func openvpn(data map[string]any, status map[string]any, unit int, enabled map[int]bool) map[vpn.ARVpnClientField]any {
	fields := readers.ReadTable(data, ovpn, func(kind nvram.ARNvramIndexType) string { return kind.Key(unit) })

	state := converters.RawConvert(
		data[nvram.ARNvramIndexTypeVpnClientState.Key(unit)],
		func(v any) any { return vpn.ARVpnStateFromValue(v) },
	)
	if state != nil {
		fields[vpn.ARVpnClientFieldState] = state
		if state == vpn.ARVpnStateError {
			errno := converters.RawConvert(
				data[nvram.ARNvramIndexTypeVpnClientErrno.Key(unit)],
				toInt,
			)
			if errno != nil {
				fields[vpn.ARVpnClientFieldStateReason] = errno
			}
		}
	}

	if len(fields) == 0 {
		return nil
	}

	fields[vpn.ARVpnClientFieldProtocol] = vpn.ARVpnProtocolOpenvpn
	fields[vpn.ARVpnClientFieldUnit] = unit
	fields[vpn.ARVpnClientFieldEnabled] = enabled[unit]

	if address := ip(status[fmt.Sprintf("vpn_client%d_ip", unit)]); address != nil {
		fields[vpn.ARVpnClientFieldAddress] = *address
	}
	for k, v := range parseStatus(status[fmt.Sprintf("vpn_client%d_status", unit)]) {
		fields[k] = v
	}

	return fields
}

// wireguard mirrors `_wireguard`: a WireGuard client profile, or nil when
// the unit is empty.
func wireguard(data map[string]any, unit int) map[vpn.ARVpnClientField]any {
	fields := readers.ReadTable(data, fusion.WG, func(kind nvram.ARNvramIndexType) string { return kind.Key(unit) })

	enable := converters.RawConvert(
		data[nvram.ARNvramIndexTypeWgcEnable.Key(unit)], toInt,
	)
	if enable == nil && len(fields) == 0 {
		return nil
	}

	fields[vpn.ARVpnClientFieldProtocol] = vpn.ARVpnProtocolWireguard
	fields[vpn.ARVpnClientFieldUnit] = unit
	fields[vpn.ARVpnClientFieldEnabled] = enable == 1
	return fields
}

// Translate mirrors `translate`: raw data into classic client profiles
// grouped by protocol.
func Translate(data map[string]any) map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any {
	status := statusData(data)
	enabled := enabledUnits(data)

	result := map[vpn.ARVpnProtocol]map[int]map[vpn.ARVpnClientField]any{}
	set := func(protocol vpn.ARVpnProtocol, unit int, fields map[vpn.ARVpnClientField]any) {
		if result[protocol] == nil {
			result[protocol] = map[int]map[vpn.ARVpnClientField]any{}
		}
		result[protocol][unit] = fields
	}
	for _, unit := range Units {
		if o := openvpn(data, status, unit, enabled); o != nil {
			set(vpn.ARVpnProtocolOpenvpn, unit, o)
		}
		if wg := wireguard(data, unit); wg != nil {
			set(vpn.ARVpnProtocolWireguard, unit, wg)
		}
	}

	return result
}

// BuildTogglePayload mirrors `build_toggle_payload`: the `(services,
// arguments)` to toggle a classic client, or ok=false.
//
// OpenVPN clients use the per-unit `start/stop_vpnclient{unit}` service;
// WireGuard clients use `start/stop_wgc {unit}` with the enable flag.
func BuildTogglePayload(protocol vpn.ARVpnProtocol, unit int, state bool) ([]service.ARServiceInput, [][2]any, bool) {
	if protocol == vpn.ARVpnProtocolOpenvpn {
		name := fmt.Sprintf("stop_vpnclient%d", unit)
		if state {
			name = fmt.Sprintf("start_vpnclient%d", unit)
		}
		return []service.ARServiceInput{service.ARServiceInput(name)}, [][2]any{{"id", unit}}, true
	}

	if protocol == vpn.ARVpnProtocolWireguard {
		name := fmt.Sprintf("stop_wgc %d", unit)
		enable := 0
		if state {
			name = fmt.Sprintf("start_wgc %d", unit)
			enable = 1
		}
		arguments := [][2]any{
			{string(nvram.ARNvramTypeWgcEnable), enable},
			{string(nvram.ARNvramTypeWgcUnit), unit},
			{"id", unit},
		}
		return []service.ARServiceInput{service.ARServiceInput(name)}, arguments, true
	}

	return nil, nil, false
}
