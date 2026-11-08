package nvram

import (
	"fmt"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// This file holds the registry-free part of Python
// `asusrouter/modules/nvram/source.py` (the indexed source, the item
// helpers and the translation); fetching and registration live in the
// `nvram/source` subpackage (see PARITY.md, split packages).

var readMac = converters.AsAny(identifiers.MacAddressFromValueSafe)

// ARNvramIndexSource is an `ARNvramIndexType` bound to an index, fetchable
// as one NVRAM key.
//
// The index is usually an int unit; string indices serve composite
// prefixes (e.g. a band `2g1`, a guest slot `0.1`, an AP group `g1`).
type ARNvramIndexSource struct {
	Kind  ARNvramIndexType
	Index any
}

// NewARNvramIndexSource initializes the indexed NVRAM source.
func NewARNvramIndexSource(kind ARNvramIndexType, index any) *ARNvramIndexSource {
	return &ARNvramIndexSource{Kind: kind, Index: index}
}

// SourceName returns the Python class name.
func (*ARNvramIndexSource) SourceName() string { return "ARNvramIndexSource" }

// SourceKey keys by the kind and index.
func (s *ARNvramIndexSource) SourceKey() []any { return []any{s.Kind, s.Index} }

// Key is the resolved NVRAM key (e.g. `wan0_ipaddr`).
func (s *ARNvramIndexSource) Key() string { return s.Kind.Key(s.Index) }

// AsHook renders the source as an `nvram_get` hook call.
func (s *ARNvramIndexSource) AsHook() endpoint.ARHookCall {
	return endpoint.ARHookCall{Hook: endpoint.ARHookNvramGet, Args: s.Key()}
}

// String mirrors `__repr__`.
func (s *ARNvramIndexSource) String() string {
	return fmt.Sprintf("<ARNvramIndexSource %s>", s.Key())
}

// ARNvramItem is a flat (`ARNvramType`) or indexed (`*ARNvramIndexSource`)
// NVRAM request item.
type ARNvramItem interface {
	source.DataSource
	AsHook() endpoint.ARHookCall
}

func rawToStrList(v any) any { return converters.RawToStrList(v, " ") }

func connectionStatusFromValue(v any) any { return common.ARConnectionStatusFromValue(v) }

func connectionMethodFromValue(v any) any { return common.ARConnectionMethodFromValue(v) }

var ipFromValueSafe = converters.AsAny(identifiers.IpAddressFromValueSafe)

func readIPList(v any) any { return identifiers.ReadIpList(v) }

// Translators per member; indexed values key on their template type
// Only general (common / utility) translations belong here - module-specific
// conversions stay in their own module
// Flat and indexed keys never collide: they are distinct Go types
var translation = map[any]func(any) any{
	ARNvramTypeMac:    readMac,
	ARNvramTypeMacLan: readMac,
	ARNvramTypeMacWan: readMac,
	// WAN globals
	ARNvramTypeDualWanCapability:     rawToStrList,
	ARNvramTypeDualWanConfig:         rawToStrList,
	ARNvramTypeDualWanExtwan:         converters.AsAny(converters.RawToBool),
	ARNvramTypeDualWanLanport:        converters.AsAny(converters.RawToInt),
	ARNvramTypeDualWanRouting:        converters.AsAny(converters.RawToBool),
	ARNvramTypeDualWanStandby:        converters.AsAny(converters.RawToBool),
	ARNvramTypeDualWanUsbBackup:      converters.AsAny(converters.RawToBool),
	ARNvramTypeLinkInternet:          connectionStatusFromValue,
	ARNvramTypeLinkWan0:              converters.AsAny(converters.RawToBool),
	ARNvramTypeLinkWan1:              converters.AsAny(converters.RawToBool),
	ARNvramTypeWanAggregation:        converters.AsAny(converters.RawToBool),
	ARNvramTypeWanAggregationPorts:   rawToStrList,
	ARNvramTypeWanAutodetect:         converters.AsAny(converters.RawToBool),
	ARNvramTypeWanS46Aftr:            ipFromValueSafe,
	ARNvramTypeWanS46B4addr:          ipFromValueSafe,
	ARNvramTypeWanUnit:               converters.AsAny(converters.RawToInt),
	ARNvramTypeWatchdogEnable:        converters.AsAny(converters.RawToBool),
	ARNvramTypeWatchdogFailbackCount: converters.AsAny(converters.RawToInt),
	ARNvramTypeWatchdogInterval:      converters.AsAny(converters.RawToInt),
	ARNvramTypeWatchdogMaxFail:       converters.AsAny(converters.RawToInt),
	// WAN per-unit status
	ARNvramIndexTypeWanDot1q:       converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanEnable:      converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanPrimary:     converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanProto:       connectionMethodFromValue,
	ARNvramIndexTypeWanRealip:      ipFromValueSafe,
	ARNvramIndexTypeWanRealipState: converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanState:       connectionStatusFromValue,
	ARNvramIndexTypeWanStateAux:    connectionStatusFromValue,
	ARNvramIndexTypeWanStateSub:    connectionStatusFromValue,
	ARNvramIndexTypeWanVid:         converters.AsAny(converters.RawToInt),
	// WAN per-unit addresses
	ARNvramIndexTypeWanDns:      readIPList,
	ARNvramIndexTypeWanDnsX:     readIPList,
	ARNvramIndexTypeWanExpires:  converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanExpiresX: converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanGateway:  ipFromValueSafe,
	ARNvramIndexTypeWanGatewayX: ipFromValueSafe,
	ARNvramIndexTypeWanIpaddr:   ipFromValueSafe,
	ARNvramIndexTypeWanIpaddrX:  ipFromValueSafe,
	ARNvramIndexTypeWanLease:    converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanLeaseX:   converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanNetmask:  ipFromValueSafe,
	ARNvramIndexTypeWanNetmaskX: ipFromValueSafe,
	// WAN per-unit connection config
	ARNvramIndexTypeWanDhcpEnable:       converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanDns1:             ipFromValueSafe,
	ARNvramIndexTypeWanDns2:             ipFromValueSafe,
	ARNvramIndexTypeWanDnsEnable:        converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanMacClone:         readMac,
	ARNvramIndexTypeWanMtu:              converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanNat:              converters.AsAny(converters.RawToBool),
	ARNvramIndexTypeWanRoutingIspEnable: converters.AsAny(converters.RawToBool),
	// WAN per-unit PPP
	ARNvramIndexTypeWanPppoeIdletime:   converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanPppoeMru:        converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanPppoeMtu:        converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanPppEcho:         converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanPppEchoFailure:  converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanPppEchoInterval: converters.AsAny(converters.RawToInt),
	// WAN per-unit softwire
	ARNvramIndexTypeWanS46DsliteMode: converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Ealen:      converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Offset:     converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Peer:       ipFromValueSafe,
	ARNvramIndexTypeWanS46Prefix4:    ipFromValueSafe,
	ARNvramIndexTypeWanS46Prefix4len: converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Prefix6:    ipFromValueSafe,
	ARNvramIndexTypeWanS46Prefix6len: converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Psid:       converters.AsAny(converters.RawToInt),
	ARNvramIndexTypeWanS46Psidlen:    converters.AsAny(converters.RawToInt),
}

// ResolveKey mirrors `_resolve_key`: resolve a requested item to its raw
// NVRAM key.
func ResolveKey(item ARNvramItem) string {
	if s, ok := item.(*ARNvramIndexSource); ok {
		return s.Key()
	}
	return item.AsHook().Args
}

// AsItems mirrors `_as_items`: coerce a single item, a slice of items or a
// collection of sources into a list (non-NVRAM sources are dropped).
func AsItems(request any) []ARNvramItem {
	switch v := request.(type) {
	case nil:
		return nil
	case ARNvramItem:
		return []ARNvramItem{v}
	case []ARNvramItem:
		return append([]ARNvramItem(nil), v...)
	case []ARNvramType:
		out := make([]ARNvramItem, len(v))
		for i, t := range v {
			out[i] = t
		}
		return out
	case []*ARNvramIndexSource:
		out := make([]ARNvramItem, len(v))
		for i, t := range v {
			out[i] = t
		}
		return out
	case []source.DataSource:
		return dataSourceItems(v)
	case source.ARDataCollection:
		return dataSourceItems(v)
	}
	return nil
}

func dataSourceItems(v []source.DataSource) []ARNvramItem {
	out := make([]ARNvramItem, 0, len(v))
	for _, s := range v {
		if item, ok := s.(ARNvramItem); ok {
			out = append(out, item)
		}
	}
	return out
}

// memberOf returns the translation-table member of a source ID: the
// `ARNvramType` itself, or the template type of an indexed source.
func memberOf(id source.ID) any {
	name, rest, _ := strings.Cut(string(id), "|")
	switch name {
	case "ARNvramType":
		return ARNvramType(rest)
	case "ARNvramIndexSource":
		kind, _, _ := strings.Cut(rest, "|")
		return ARNvramIndexType(kind)
	}
	return nil
}

// TranslateState mirrors `translate_state`: translate the NVRAM data
// state. The data is keyed by the ID of the requesting item.
func TranslateState(data map[source.ID]any) map[source.ID]any {
	result := make(map[source.ID]any, len(data))
	for id, value := range data {
		if converter, ok := translation[memberOf(id)]; ok && converter != nil {
			result[id] = converter(value)
		} else {
			result[id] = value
		}
	}
	return result
}
