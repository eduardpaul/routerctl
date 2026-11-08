// Package openvpn mirrors asusrouter/modules/vpn/server/openvpn.py: the
// OpenVPN server backend for the VPN module.
//
// Python keeps the OpenVPN and WireGuard backends as sibling modules with
// the same function names (`nvram_items`, `translate`, ...), so in Go each
// backend module is its own package (`openvpn.Translate` mirrors
// `openvpn.translate`).
package openvpn

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Units are the OpenVPN server units exposed by the firmware
var Units = []int{1, 2}

// ClientStatusKey is the key under which the source stashes the
// connected-client status
const ClientStatusKey = "_openvpn_client_status"

var (
	toInt  = converters.AsAny(converters.RawToInt)
	toStr  = converters.AsAny(converters.RawToStr)
	toBool = converters.AsAny(converters.RawToBool)
	toIP   = converters.AsAny(identifiers.IpAddressFromValueSafe)
)

// Settings nvram key -> field, converter (apply to the active unit only)
var settings = []readers.TableEntry[nvram.ARNvramType, vpn.ARVpnServerField]{
	{Key: nvram.ARNvramTypeVpnServerPort, Field: vpn.ARVpnServerFieldPort, Converter: toInt},
	{Key: nvram.ARNvramTypeVpnServerProto, Field: vpn.ARVpnServerFieldProtocol, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerIf, Field: vpn.ARVpnServerFieldInterface, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerCrypt, Field: vpn.ARVpnServerFieldCrypt, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerCipher, Field: vpn.ARVpnServerFieldCipher, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerDigest, Field: vpn.ARVpnServerFieldDigest, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerComp, Field: vpn.ARVpnServerFieldCompression, Converter: toStr},
	{Key: nvram.ARNvramTypeVpnServerHmac, Field: vpn.ARVpnServerFieldHmac, Converter: toInt},
	{
		Key:       nvram.ARNvramTypeVpnServerTlsKeysize,
		Field:     vpn.ARVpnServerFieldTlsKeysize,
		Converter: toInt,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerIgncrt,
		Field:     vpn.ARVpnServerFieldIgnoreCertificate,
		Converter: toBool,
	},
	{Key: nvram.ARNvramTypeVpnServerPdns, Field: vpn.ARVpnServerFieldPushDns, Converter: toBool},
	{
		Key:       nvram.ARNvramTypeVpnServerSn,
		Field:     vpn.ARVpnServerFieldSubnet,
		Converter: toIP,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerNm,
		Field:     vpn.ARVpnServerFieldNetmask,
		Converter: toIP,
	},
	{Key: nvram.ARNvramTypeVpnServerDhcp, Field: vpn.ARVpnServerFieldDhcp, Converter: toBool},
	{
		Key:       nvram.ARNvramTypeVpnServerR1,
		Field:     vpn.ARVpnServerFieldPoolStart,
		Converter: toIP,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerR2,
		Field:     vpn.ARVpnServerFieldPoolEnd,
		Converter: toIP,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerLocal,
		Field:     vpn.ARVpnServerFieldLocalAddress,
		Converter: toIP,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerRemote,
		Field:     vpn.ARVpnServerFieldRemoteAddress,
		Converter: toIP,
	},
	{Key: nvram.ARNvramTypeVpnServerReneg, Field: vpn.ARVpnServerFieldReneg, Converter: toInt},
	{
		Key:       nvram.ARNvramTypeVpnServerRgw,
		Field:     vpn.ARVpnServerFieldRedirectGateway,
		Converter: toInt,
	},
	{
		Key:       nvram.ARNvramTypeVpnServerC2c,
		Field:     vpn.ARVpnServerFieldClientToClient,
		Converter: toBool,
	},
}

// ServerEnabled mirrors `server_enabled`: whether the OpenVPN server is
// enabled (worth fetching live status).
func ServerEnabled(data map[string]any) bool {
	enabled := converters.RawToBool(data[string(nvram.ARNvramTypeVpnServerEnable)])
	return enabled != nil && *enabled
}

// A connected-client status line: `remote_ip:port vpn_ip name`
const statusFields = 3

// Wrapper tag around the plaintext status body
var vpnserverRe = regexp.MustCompile(`(?s)<vpnserver>(.*?)</vpnserver>`)

// ReadClientStatus mirrors `read_client_status`: parse the
// connected-client status payload of the OpenVPN server.
//
// The payload wraps plaintext lines in a `<vpnserver>` tag; each line is
// `remote_ip:port vpn_ip name`.
func ReadClientStatus(content any) map[string]any {
	text := converters.RawToStr(content)
	if text == nil || *text == "" {
		return map[string]any{}
	}

	body := *text
	if match := vpnserverRe.FindStringSubmatch(body); match != nil {
		body = match[1]
	}

	connected := []any{}
	for _, line := range splitLines(body) {
		fields := converters.PySplit(line)
		if len(fields) != statusFields {
			continue
		}
		remote, vpnIP, name := fields[0], fields[1], fields[2]
		connected = append(connected, map[string]any{"name": name, "vpn_ip": vpnIP, "remote": remote})
	}

	return map[string]any{"connected": connected}
}

// splitLines mirrors Python `str.splitlines()` (no line ends kept).
func splitLines(s string) []string {
	var lines []string
	start := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\u0085', ' ', ' ':
			lines = append(lines, string(runes[start:i]))
			start = i + 1
		case '\r':
			lines = append(lines, string(runes[start:i]))
			if i+1 < len(runes) && runes[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(runes) {
		lines = append(lines, string(runes[start:]))
	}
	return lines
}

// isLegacy mirrors `_is_legacy`: whether the firmware uses the legacy
// per-unit start/stop services.
func isLegacy(identity *device.ARDeviceIdentity) bool {
	if identity == nil {
		return true
	}
	fw := identity.Firmware()
	_, merlinLike := firmware.ARFwMerlinLike[fw.FirmwareType()]
	return merlinLike ||
		fw.FirmwareType() != firmware.ARFirmwareTypeStock ||
		fw.Less(firmware.ARFw388)
}

// BuildTogglePayload mirrors `build_toggle_payload`: the `(services,
// arguments)` to enable/disable the OpenVPN server.
func BuildTogglePayload(unit int, state bool, identity *device.ARDeviceIdentity) ([]service.ARServiceInput, [][2]any) {
	if isLegacy(identity) {
		name := fmt.Sprintf("stop_vpnserver%d", unit)
		if state {
			name = fmt.Sprintf("start_vpnserver%d", unit)
		}
		return []service.ARServiceInput{service.ARServiceInput(name)}, [][2]any{{"id", unit}}
	}

	var services []service.ARServiceInput
	if state {
		services = []service.ARServiceInput{
			common.ARServiceOpenvpnRestart,
			common.ARServiceChpassRestart,
			common.ARServiceSambaRestart,
			common.ARServiceDnsRestart,
		}
	} else {
		services = []service.ARServiceInput{
			common.ARServiceOpenvpnStop,
			common.ARServiceSambaRestart,
			common.ARServiceDnsRestart,
		}
	}
	enable := 0
	if state {
		enable = 1
	}
	arguments := [][2]any{
		{string(nvram.ARNvramTypeVpnServerEnable), enable},
		{"id", unit},
	}
	return services, arguments
}

// NvramItems mirrors `nvram_items`: the nvram items required to read
// every OpenVPN server unit.
func NvramItems() []nvram.ARNvramItem {
	items := []nvram.ARNvramItem{
		nvram.ARNvramTypeVpnServerEnable,
		nvram.ARNvramTypeVpnServerUnit,
		nvram.ARNvramTypeVpnServerClientlist,
	}
	for _, row := range settings {
		items = append(items, row.Key)
	}
	for _, unit := range Units {
		items = append(items, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeVpnServerState, unit))
		items = append(items, nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeVpnServerErrno, unit))
	}
	return items
}

// readSettings mirrors `_settings`: read the active-unit settings block.
func readSettings(data map[string]any) map[vpn.ARVpnServerField]any {
	return readers.ReadTable(data, settings, func(kind nvram.ARNvramType) string { return string(kind) })
}

// clients mirrors `_clients`: parse the `<username>password` account list
// of the active unit.
func clients(raw any) []map[vpn.ARVpnPeerField]any {
	result := []map[vpn.ARVpnPeerField]any{}
	for _, entry := range readers.SplitRows(raw) {
		parts := strings.Split(entry, ">")
		name := converters.RawToStr(parts[0])
		if name == nil {
			continue
		}
		client := map[vpn.ARVpnPeerField]any{vpn.ARVpnPeerFieldName: *name}
		if password := identifiers.PasswordFromValueSafe(fieldValue(parts, 1)); password != nil {
			client[vpn.ARVpnPeerFieldPassword] = *password
		}
		result = append(result, client)
	}

	return result
}

// fieldValue is `get_field` as a Python value (nil for None).
func fieldValue(parts []string, index int) any {
	if f := readers.GetField(parts, index); f != nil {
		return *f
	}
	return nil
}

// connected mirrors `_connected`: map client name -> its live connection
// entry from the status.
func connected(data map[string]any) map[string]map[string]any {
	status, ok := data[ClientStatusKey].(map[string]any)
	if !ok {
		return map[string]map[string]any{}
	}

	result := map[string]map[string]any{}
	entries, _ := status["connected"].([]any)
	for _, item := range entries {
		entry, _ := item.(map[string]any)
		if name := converters.RawToStr(entry["name"]); name != nil {
			result[*name] = entry
		}
	}
	return result
}

// liveFields mirrors `_live_fields`: the live fields (state, address,
// remote) of a connected client.
func liveFields(entry map[string]any) map[vpn.ARVpnPeerField]any {
	fields := map[vpn.ARVpnPeerField]any{
		vpn.ARVpnPeerFieldState: vpn.ARVpnStateConnected,
	}

	if address := identifiers.IpInterfaceFromValueSafe(entry["vpn_ip"]); address != nil {
		fields[vpn.ARVpnPeerFieldAddress] = []identifiers.IpInterface{*address}
	}

	if remote := converters.RawToStr(entry["remote"]); remote != nil {
		host, port, sep := rpartition(*remote, ":")
		target := *remote
		if sep {
			target = host
		}
		if remoteIP := identifiers.IpAddressFromValueSafe(target); remoteIP != nil {
			fields[vpn.ARVpnPeerFieldRemoteAddress] = *remoteIP
		}
		var remotePort *int
		if sep {
			remotePort = converters.RawToInt(port)
		}
		if remotePort != nil {
			fields[vpn.ARVpnPeerFieldRemotePort] = *remotePort
		}
	}

	return fields
}

// rpartition mirrors Python `str.rpartition` (head, tail, found).
func rpartition(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return "", s, false
	}
	return s[:i], s[i+len(sep):], true
}

// applyStatus mirrors `_apply_status`: enrich connected accounts, and add
// connected clients not in the list.
func applyStatus(clients []map[vpn.ARVpnPeerField]any, connected map[string]map[string]any) []map[vpn.ARVpnPeerField]any {
	for _, client := range clients {
		name, isStr := client[vpn.ARVpnPeerFieldName].(string)
		if !isStr {
			continue
		}
		if entry, ok := connected[name]; ok {
			for k, v := range liveFields(entry) {
				client[k] = v
			}
		}
	}

	listed := map[any]bool{}
	for _, client := range clients {
		listed[client[vpn.ARVpnPeerFieldName]] = true
	}
	names := make([]string, 0, len(connected))
	for name := range connected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if listed[name] {
			continue
		}
		client := map[vpn.ARVpnPeerField]any{vpn.ARVpnPeerFieldName: name}
		for k, v := range liveFields(connected[name]) {
			client[k] = v
		}
		clients = append(clients, client)
	}

	return clients
}

// Translate mirrors `translate`: raw data into OpenVPN server profiles
// keyed by unit.
func Translate(data map[string]any) map[int]map[vpn.ARVpnServerField]any {
	active := converters.RawToInt(data[string(nvram.ARNvramTypeVpnServerUnit)])
	enabled := converters.RawToBool(data[string(nvram.ARNvramTypeVpnServerEnable)])
	activeSettings := readSettings(data)

	result := map[int]map[vpn.ARVpnServerField]any{}
	for _, unit := range Units {
		fields := map[vpn.ARVpnServerField]any{}

		state := converters.RawConvert(
			data[nvram.ARNvramIndexTypeVpnServerState.Key(unit)],
			func(v any) any { return vpn.ARVpnStateFromValue(v) },
		)
		if state != nil {
			fields[vpn.ARVpnServerFieldState] = state
		}
		errno := converters.RawConvert(
			data[nvram.ARNvramIndexTypeVpnServerErrno.Key(unit)], toInt,
		)
		if errno != nil {
			fields[vpn.ARVpnServerFieldErrno] = errno
		}

		// Settings, accounts and the enable flag belong to the active unit
		if active != nil && unit == *active {
			if enabled != nil {
				fields[vpn.ARVpnServerFieldEnabled] = *enabled
			}
			for k, v := range activeSettings {
				fields[k] = v
			}
			accounts := applyStatus(
				clients(data[string(nvram.ARNvramTypeVpnServerClientlist)]),
				connected(data),
			)
			if len(accounts) > 0 {
				fields[vpn.ARVpnServerFieldClients] = accounts
			}
		}

		if len(fields) > 0 {
			result[unit] = fields
		}
	}

	return result
}
