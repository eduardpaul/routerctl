// Package wireguard mirrors asusrouter/modules/vpn/server/wireguard.py:
// the WireGuard server backend for the VPN module.
//
// Python keeps the OpenVPN and WireGuard backends as sibling modules with
// the same function names (`nvram_items`, `translate`, ...), so in Go each
// backend module is its own package (`wireguard.Translate` mirrors
// `wireguard.translate`).
package wireguard

import (
	"strings"
	"unicode/utf8"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// Unit is the single WireGuard server unit on current firmware
const Unit = 1

// MaxPeers is the number of peer slots to probe; `get_wgsc_status` on
// real devices exposes ten
const MaxPeers = 10

// Hook is the WireGuard server status hook
const Hook = endpoint.ARHookWireguardServerStatus

// decode mirrors `_decode`: URL-decode a percent-encoded nvram value
// (peer fields are encoded).
func decode(value any) *string {
	text := converters.RawToStr(value)
	if text == nil {
		return nil
	}
	decoded := unquote(*text)
	return &decoded
}

// decodeInterfaces mirrors `_decode_interfaces`: URL-decode then read a
// comma/whitespace list of IP interfaces.
func decodeInterfaces(value any) []identifiers.IpInterface {
	text := decode(value)
	if text == nil {
		return identifiers.ReadIpInterfaceList(nil)
	}
	return identifiers.ReadIpInterfaceList(*text)
}

// unquote mirrors `urllib.parse.unquote` (UTF-8, errors="replace"):
// `%XX` escapes inside ASCII runs are decoded; invalid escapes and
// non-ASCII characters are kept as-is.
func unquote(text string) string {
	if !strings.Contains(text, "%") {
		return text
	}
	var out strings.Builder
	var run []byte
	flush := func() {
		for len(run) > 0 {
			r, size := utf8.DecodeRune(run)
			out.WriteRune(r)
			run = run[size:]
		}
	}
	for i := 0; i < len(text); {
		c := text[i]
		if c >= utf8.RuneSelf {
			// Non-ASCII chunk: appended verbatim
			flush()
			_, size := utf8.DecodeRuneInString(text[i:])
			out.WriteString(text[i : i+size])
			i += size
			continue
		}
		if c == '%' && i+2 < len(text) && isHex(text[i+1]) && isHex(text[i+2]) {
			run = append(run, unhex(text[i+1])<<4|unhex(text[i+2]))
			i += 3
			continue
		}
		run = append(run, c)
		i++
	}
	flush()
	return out.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// Server nvram key -> field, converter
var settings = []readers.TableEntry[nvram.ARNvramType, vpn.ARVpnServerField]{
	{Key: nvram.ARNvramTypeWgsEnable, Field: vpn.ARVpnServerFieldEnabled, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramTypeWgsAddr, Field: vpn.ARVpnServerFieldAddress, Converter: converters.AsAny(identifiers.IpInterfaceFromValueSafe)},
	{Key: nvram.ARNvramTypeWgsPort, Field: vpn.ARVpnServerFieldPort, Converter: converters.AsAny(converters.RawToInt)},
	{Key: nvram.ARNvramTypeWgsDns, Field: vpn.ARVpnServerFieldAllowDns, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramTypeWgsNat6, Field: vpn.ARVpnServerFieldNat6, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramTypeWgsPsk, Field: vpn.ARVpnServerFieldPsk, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramTypeWgsAlive, Field: vpn.ARVpnServerFieldKeepalive, Converter: converters.AsAny(converters.RawToInt)},
	{Key: nvram.ARNvramTypeWgsLanaccess, Field: vpn.ARVpnServerFieldLanAccess, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramTypeWgsPub, Field: vpn.ARVpnServerFieldPublicKey, Converter: converters.AsAny(converters.RawToStr)},
	{
		Key:       nvram.ARNvramTypeWgsPriv,
		Field:     vpn.ARVpnServerFieldPrivateKey,
		Converter: converters.AsAny(identifiers.PasswordFromValueSafe),
	},
}

// Per-peer nvram key -> field, converter (`wgs1_c{n}_*`)
var peer = []readers.TableEntry[nvram.ARNvramIndexType, vpn.ARVpnPeerField]{
	{Key: nvram.ARNvramIndexTypeWgsPeerEnable, Field: vpn.ARVpnPeerFieldEnabled, Converter: converters.AsAny(converters.RawToBool)},
	{Key: nvram.ARNvramIndexTypeWgsPeerName, Field: vpn.ARVpnPeerFieldName, Converter: converters.AsAny(decode)},
	{
		Key:       nvram.ARNvramIndexTypeWgsPeerAddr,
		Field:     vpn.ARVpnPeerFieldAddress,
		Converter: func(v any) any { return decodeInterfaces(v) },
	},
	{
		Key:       nvram.ARNvramIndexTypeWgsPeerAips,
		Field:     vpn.ARVpnPeerFieldAllowedIps,
		Converter: func(v any) any { return decodeInterfaces(v) },
	},
	{
		Key:       nvram.ARNvramIndexTypeWgsPeerCaips,
		Field:     vpn.ARVpnPeerFieldClientAllowedIps,
		Converter: func(v any) any { return decodeInterfaces(v) },
	},
}

// BuildTogglePayload mirrors `build_toggle_payload`: the `(services,
// arguments)` to enable/disable the WG server.
func BuildTogglePayload(unit int, state bool) ([]service.ARServiceInput, [][2]any) {
	services := []service.ARServiceInput{
		common.ARServiceWireguardServerRestart,
		common.ARServiceDnsRestart,
	}
	arguments := [][2]any{
		{string(nvram.ARNvramTypeWgsEnable), boolToInt(state)},
		{string(nvram.ARNvramTypeWgsUnit), unit},
		{"id", unit},
	}
	return services, arguments
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NvramItems mirrors `nvram_items`: the nvram items to read the WireGuard
// server and its peers.
func NvramItems() []nvram.ARNvramItem {
	items := make([]nvram.ARNvramItem, 0, len(settings)+MaxPeers*len(peer))
	for _, row := range settings {
		items = append(items, row.Key)
	}
	for index := 1; index <= MaxPeers; index++ {
		for _, row := range peer {
			items = append(items, nvram.NewARNvramIndexSource(row.Key, index))
		}
	}
	return items
}

// peerStatus mirrors `_peer_status`: map peer index -> live state from the
// status hook.
func peerStatus(data map[string]any) map[int]vpn.ARVpnState {
	status := map[int]vpn.ARVpnState{}
	hook, ok := data[string(Hook)].(map[string]any)
	if !ok {
		return status
	}

	entries, _ := hook["client_status"].([]any)
	for _, item := range entries {
		entry, _ := item.(map[string]any)
		index := converters.RawToInt(entry["index"])
		if index == nil {
			continue
		}
		state := converters.RawToInt(entry["status"])
		if state != nil && *state == 1 {
			status[*index] = vpn.ARVpnStateConnected
		} else {
			status[*index] = vpn.ARVpnStateDisconnected
		}
	}

	return status
}

// peerEntry mirrors `_peer`: a single peer entry, or nil when the slot is
// empty.
func peerEntry(data map[string]any, index int, status map[int]vpn.ARVpnState) map[vpn.ARVpnPeerField]any {
	fields := readers.ReadTable(data, peer, func(kind nvram.ARNvramIndexType) string { return kind.Key(index) })
	if len(fields) == 0 {
		return nil
	}

	if state, ok := status[index]; ok {
		fields[vpn.ARVpnPeerFieldState] = state
	}

	return fields
}

// Translate mirrors `translate`: raw data into the WireGuard server
// profile keyed by unit.
func Translate(data map[string]any) map[int]map[vpn.ARVpnServerField]any {
	fields := readers.ReadTable(data, settings, func(kind nvram.ARNvramType) string { return string(kind) })
	if len(fields) == 0 {
		return map[int]map[vpn.ARVpnServerField]any{}
	}

	status := peerStatus(data)
	var peers []map[vpn.ARVpnPeerField]any
	for index := 1; index <= MaxPeers; index++ {
		if p := peerEntry(data, index, status); p != nil {
			peers = append(peers, p)
		}
	}
	if len(peers) > 0 {
		fields[vpn.ARVpnServerFieldClients] = peers
	}

	return map[int]map[vpn.ARVpnServerField]any{Unit: fields}
}
