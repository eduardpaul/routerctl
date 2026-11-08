// Package wan mirrors asusrouter/modules/wan/.
//
// The enums (ARDualWanMode, ARWANCapability) are generated in
// enums_gen.go; `__init__.py` only re-exports.
package wan

import (
	"context"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Units fetched for WAN (primary and secondary)
var wanUnits = []int{0, 1}

// ARWanAddress mirrors `ARWanAddress`: a WAN address block (the runtime
// `main` or configured `extra` set).
type ARWanAddress struct {
	Ip      *identifiers.IpAddress  `json:"ip"`
	Gateway *identifiers.IpAddress  `json:"gateway"`
	Mask    *identifiers.IpAddress  `json:"mask"`
	Dns     []identifiers.IpAddress `json:"dns"`
	Lease   *int                    `json:"lease"`
	Expires *int                    `json:"expires"`
}

// ARWanVlan mirrors `ARWanVlan`: WAN VLAN tagging configuration.
type ARWanVlan struct {
	Tagged bool `json:"tagged"`
	Vid    *int `json:"vid"`
}

// ARWanPPP mirrors `ARWanPPP`: WAN PPP / PPPoE configuration.
type ARWanPPP struct {
	Echo          *int    `json:"echo"`
	EchoInterval  *int    `json:"echo_interval"`
	EchoFailure   *int    `json:"echo_failure"`
	Conn          *string `json:"conn"`
	Heartbeat     *string `json:"heartbeat"`
	PppoeAc       *string `json:"pppoe_ac"`
	PppoeHostuniq *string `json:"pppoe_hostuniq"`
	PppoeIdletime *int    `json:"pppoe_idletime"`
	PppoeMru      *int    `json:"pppoe_mru"`
	PppoeMtu      *int    `json:"pppoe_mtu"`
	PppoeOptions  *string `json:"pppoe_options"`
	PppoeService  *string `json:"pppoe_service"`
}

// ARWanSoftwire mirrors `ARWanSoftwire`: WAN Softwire46 (IPv4-over-IPv6
// transition) configuration.
type ARWanSoftwire struct {
	DsliteMode    *int                   `json:"dslite_mode"`
	DsliteService *string                `json:"dslite_service"`
	Aftr          *identifiers.IpAddress `json:"aftr"`
	B4addr        *identifiers.IpAddress `json:"b4addr"`
	Peer          *identifiers.IpAddress `json:"peer"`
	Prefix4       *identifiers.IpAddress `json:"prefix4"`
	Prefix4Len    *int                   `json:"prefix4_len"`
	Prefix6       *identifiers.IpAddress `json:"prefix6"`
	Prefix6Len    *int                   `json:"prefix6_len"`
	EaLen         *int                   `json:"ea_len"`
	Offset        *int                   `json:"offset"`
	Psid          *int                   `json:"psid"`
	PsidLen       *int                   `json:"psid_len"`
}

// ARWanUnit mirrors `ARWanUnit`: a single WAN unit (runtime status and
// configuration).
type ARWanUnit struct {
	Index       int                       `json:"index"`
	Enable      bool                      `json:"enable"`
	Primary     bool                      `json:"primary"`
	Protocol    common.ARConnectionMethod `json:"protocol"`
	Status      common.ARConnectionStatus `json:"status"`
	StatusSub   common.ARConnectionStatus `json:"status_sub"`
	StatusAux   common.ARConnectionStatus `json:"status_aux"`
	Link        bool                      `json:"link"`
	RealIp      *identifiers.IpAddress    `json:"real_ip"`
	RealIpState bool                      `json:"real_ip_state"`
	Main        ARWanAddress              `json:"main"`
	Extra       ARWanAddress              `json:"extra"`

	// Configuration
	Mtu              *int                    `json:"mtu"`
	Nat              bool                    `json:"nat"`
	DhcpEnable       bool                    `json:"dhcp_enable"`
	DnsAuto          bool                    `json:"dns_auto"`
	DnsServers       []identifiers.IpAddress `json:"dns_servers"`
	Hostname         *string                 `json:"hostname"`
	ClientId         *string                 `json:"client_id"`
	VendorId         *string                 `json:"vendor_id"`
	MacClone         *identifiers.MacAddress `json:"mac_clone"`
	Vlan             ARWanVlan               `json:"vlan"`
	Ppp              ARWanPPP                `json:"ppp"`
	Softwire         ARWanSoftwire           `json:"softwire"`
	RoutingIsp       *string                 `json:"routing_isp"`
	RoutingIspEnable bool                    `json:"routing_isp_enable"`
	IspCountry       *string                 `json:"isp_country"`
	IspList          *string                 `json:"isp_list"`
	IspNum           *string                 `json:"isp_num"`
}

// NewARWanUnit returns a unit with the Python dataclass defaults.
func NewARWanUnit(index int) ARWanUnit {
	return ARWanUnit{
		Index:     index,
		Protocol:  common.ARConnectionMethodUnknown,
		Status:    common.ARConnectionStatusUnknown,
		StatusSub: common.ARConnectionStatusUnknown,
		StatusAux: common.ARConnectionStatusUnknown,
	}
}

// ARDualWan mirrors `ARDualWan`: dual WAN configuration.
type ARDualWan struct {
	Mode       ARDualWanMode `json:"mode"`
	Priority   []string      `json:"priority"`
	State      bool          `json:"state"`
	Capability []string      `json:"capability"`
	LanPort    *int          `json:"lan_port"`
	LbRatio    *[2]int       `json:"lb_ratio"`
	Standby    bool          `json:"standby"`
	Routing    bool          `json:"routing"`
	UsbBackup  bool          `json:"usb_backup"`
	Autodetect bool          `json:"autodetect"`
	Extwan     bool          `json:"extwan"`
}

// ARWanAggregation mirrors `ARWanAggregation`: WAN link aggregation
// configuration.
type ARWanAggregation struct {
	State bool     `json:"state"`
	Ports []string `json:"ports"`
}

// ARWanWatchdog mirrors `ARWanWatchdog`: WAN failover watchdog
// configuration.
type ARWanWatchdog struct {
	Enable        bool    `json:"enable"`
	Target        *string `json:"target"`
	Interval      *int    `json:"interval"`
	MaxFail       *int    `json:"max_fail"`
	FailbackCount *int    `json:"failback_count"`
}

// ARWan mirrors `ARWan`: the WAN state of the router.
type ARWan struct {
	ActiveUnit     *int                      `json:"active_unit"`
	Units          map[int]ARWanUnit         `json:"units"`
	Dualwan        *ARDualWan                `json:"dualwan"`
	Aggregation    *ARWanAggregation         `json:"aggregation"`
	Watchdog       *ARWanWatchdog            `json:"watchdog"`
	InternetStatus common.ARConnectionStatus `json:"internet_status"`
}

// NewARWan returns a WAN state with the Python dataclass defaults.
func NewARWan() ARWan {
	return ARWan{Units: map[int]ARWanUnit{}, InternetStatus: common.ARConnectionStatusUnknown}
}

// Flat (global) NVRAM members for the WAN state
var wanFlat = []nvram.ARNvramType{
	nvram.ARNvramTypeDualWanCapability,
	nvram.ARNvramTypeDualWanConfig,
	nvram.ARNvramTypeDualWanExtwan,
	nvram.ARNvramTypeDualWanLanport,
	nvram.ARNvramTypeDualWanLbRatio,
	nvram.ARNvramTypeDualWanMode,
	nvram.ARNvramTypeDualWanRouting,
	nvram.ARNvramTypeDualWanStandby,
	nvram.ARNvramTypeDualWanUsbBackup,
	nvram.ARNvramTypeLinkInternet,
	nvram.ARNvramTypeLinkWan0,
	nvram.ARNvramTypeLinkWan1,
	nvram.ARNvramTypeWanAggregation,
	nvram.ARNvramTypeWanAggregationPorts,
	nvram.ARNvramTypeWanAutodetect,
	nvram.ARNvramTypeWanS46Aftr,
	nvram.ARNvramTypeWanS46B4addr,
	nvram.ARNvramTypeWanUnit,
	nvram.ARNvramTypeWatchdogEnable,
	nvram.ARNvramTypeWatchdogFailbackCount,
	nvram.ARNvramTypeWatchdogInterval,
	nvram.ARNvramTypeWatchdogMaxFail,
	nvram.ARNvramTypeWatchdogTarget,
}

// Indexed (per-unit) NVRAM members for the WAN state
var wanIndexed = []nvram.ARNvramIndexType{
	// Status
	nvram.ARNvramIndexTypeWanDot1q,
	nvram.ARNvramIndexTypeWanEnable,
	nvram.ARNvramIndexTypeWanPrimary,
	nvram.ARNvramIndexTypeWanProto,
	nvram.ARNvramIndexTypeWanRealip,
	nvram.ARNvramIndexTypeWanRealipState,
	nvram.ARNvramIndexTypeWanState,
	nvram.ARNvramIndexTypeWanStateAux,
	nvram.ARNvramIndexTypeWanStateSub,
	nvram.ARNvramIndexTypeWanVid,
	// Addresses
	nvram.ARNvramIndexTypeWanDns,
	nvram.ARNvramIndexTypeWanDnsX,
	nvram.ARNvramIndexTypeWanExpires,
	nvram.ARNvramIndexTypeWanExpiresX,
	nvram.ARNvramIndexTypeWanGateway,
	nvram.ARNvramIndexTypeWanGatewayX,
	nvram.ARNvramIndexTypeWanIpaddr,
	nvram.ARNvramIndexTypeWanIpaddrX,
	nvram.ARNvramIndexTypeWanLease,
	nvram.ARNvramIndexTypeWanLeaseX,
	nvram.ARNvramIndexTypeWanNetmask,
	nvram.ARNvramIndexTypeWanNetmaskX,
	// Connection config
	nvram.ARNvramIndexTypeWanClientid,
	nvram.ARNvramIndexTypeWanDhcpEnable,
	nvram.ARNvramIndexTypeWanDns1,
	nvram.ARNvramIndexTypeWanDns2,
	nvram.ARNvramIndexTypeWanDnsEnable,
	nvram.ARNvramIndexTypeWanHostname,
	nvram.ARNvramIndexTypeWanMacClone,
	nvram.ARNvramIndexTypeWanMtu,
	nvram.ARNvramIndexTypeWanNat,
	nvram.ARNvramIndexTypeWanVendorid,
	// ISP routing
	nvram.ARNvramIndexTypeWanIspCountry,
	nvram.ARNvramIndexTypeWanIspList,
	nvram.ARNvramIndexTypeWanIspNum,
	nvram.ARNvramIndexTypeWanRoutingIsp,
	nvram.ARNvramIndexTypeWanRoutingIspEnable,
	// PPP
	nvram.ARNvramIndexTypeWanHeartbeat,
	nvram.ARNvramIndexTypeWanPppoeAc,
	nvram.ARNvramIndexTypeWanPppoeHostuniq,
	nvram.ARNvramIndexTypeWanPppoeIdletime,
	nvram.ARNvramIndexTypeWanPppoeMru,
	nvram.ARNvramIndexTypeWanPppoeMtu,
	nvram.ARNvramIndexTypeWanPppoeOptions,
	nvram.ARNvramIndexTypeWanPppoeService,
	nvram.ARNvramIndexTypeWanPppConn,
	nvram.ARNvramIndexTypeWanPppEcho,
	nvram.ARNvramIndexTypeWanPppEchoFailure,
	nvram.ARNvramIndexTypeWanPppEchoInterval,
	// Softwire
	nvram.ARNvramIndexTypeWanS46DsliteMode,
	nvram.ARNvramIndexTypeWanS46DsliteSvc,
	nvram.ARNvramIndexTypeWanS46Ealen,
	nvram.ARNvramIndexTypeWanS46Offset,
	nvram.ARNvramIndexTypeWanS46Peer,
	nvram.ARNvramIndexTypeWanS46Prefix4,
	nvram.ARNvramIndexTypeWanS46Prefix4len,
	nvram.ARNvramIndexTypeWanS46Prefix6,
	nvram.ARNvramIndexTypeWanS46Prefix6len,
	nvram.ARNvramIndexTypeWanS46Psid,
	nvram.ARNvramIndexTypeWanS46Psidlen,
}

// Full NVRAM request for the WAN state, built once
var wanRequest = func() []nvram.ARNvramItem {
	items := make([]nvram.ARNvramItem, 0, len(wanFlat)+len(wanUnits)*len(wanIndexed))
	for _, kind := range wanFlat {
		items = append(items, kind)
	}
	for _, index := range wanUnits {
		for _, kind := range wanIndexed {
			items = append(items, nvram.NewARNvramIndexSource(kind, index))
		}
	}
	return items
}()

// ARWanSource mirrors `ARWanSource`: WAN data source for the connected
// router.
type ARWanSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARWanSource) SourceName() string { return "ARWanSource" }

// ARWanSourceUniversal is the universal instance - preferred
var ARWanSourceUniversal = &ARWanSource{}

// indexKey is the `(kind, index)` key of `_Indexed`.
type indexKey struct {
	kind  nvram.ARNvramIndexType
	index int
}

type indexedValues map[indexKey]any

// Per-unit address members, ordered as the `ARWanAddress` fields use them
var addrMain = [6]nvram.ARNvramIndexType{
	nvram.ARNvramIndexTypeWanDns,
	nvram.ARNvramIndexTypeWanExpires,
	nvram.ARNvramIndexTypeWanGateway,
	nvram.ARNvramIndexTypeWanIpaddr,
	nvram.ARNvramIndexTypeWanLease,
	nvram.ARNvramIndexTypeWanNetmask,
}
var addrExtra = [6]nvram.ARNvramIndexType{
	nvram.ARNvramIndexTypeWanDnsX,
	nvram.ARNvramIndexTypeWanExpiresX,
	nvram.ARNvramIndexTypeWanGatewayX,
	nvram.ARNvramIndexTypeWanIpaddrX,
	nvram.ARNvramIndexTypeWanLeaseX,
	nvram.ARNvramIndexTypeWanNetmaskX,
}

// idx looks up a translated indexed value, or nil if absent.
func idx(indexed indexedValues, kind nvram.ARNvramIndexType, index int) any {
	return indexed[indexKey{kind, index}]
}

// as reads a translated value as *T (a T or a non-nil *T), else nil.
func as[T any](value any) *T {
	switch v := value.(type) {
	case T:
		return &v
	case *T:
		return v
	}
	return nil
}

// idxAs is `_idx` typed: the translated indexed value as *T, or nil.
func idxAs[T any](indexed indexedValues, kind nvram.ARNvramIndexType, index int) *T {
	return as[T](idx(indexed, kind, index))
}

// pyBool mirrors Python `bool(value)` on a translated value.
func pyBool(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case int:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	case []string:
		return len(v) > 0
	case []any:
		return len(v) > 0
	case []identifiers.IpAddress:
		return len(v) > 0
	}
	return true
}

// ipList reads a translated IP list (`[]` when absent).
func ipList(value any) []identifiers.IpAddress {
	if v, ok := value.([]identifiers.IpAddress); ok && v != nil {
		return v
	}
	return []identifiers.IpAddress{}
}

// strList reads a translated string list (nil when absent or empty).
func strList(value any) []string {
	if v, ok := value.([]string); ok {
		return v
	}
	return nil
}

// parseLbRatio mirrors `_parse_lb_ratio`: parse a `primary:secondary`
// load-balance ratio into two ints.
func parseLbRatio(raw any) *[2]int {
	text := converters.RawToStr(raw)
	if text == nil || *text == "" {
		return nil
	}
	parts := strings.Split(*text, ":")
	if len(parts) != 2 {
		return nil
	}
	primary, secondary := converters.RawToInt(parts[0]), converters.RawToInt(parts[1])
	if primary == nil || secondary == nil {
		return nil
	}
	return &[2]int{*primary, *secondary}
}

// buildAddress mirrors `_build_address`: build a WAN address block for a
// unit (main or extra set).
func buildAddress(indexed indexedValues, index int, extra bool) ARWanAddress {
	keys := addrMain
	if extra {
		keys = addrExtra
	}
	dns, expires, gateway, ipaddr, lease, netmask := keys[0], keys[1], keys[2], keys[3], keys[4], keys[5]

	return ARWanAddress{
		Ip:      idxAs[identifiers.IpAddress](indexed, ipaddr, index),
		Gateway: idxAs[identifiers.IpAddress](indexed, gateway, index),
		Mask:    idxAs[identifiers.IpAddress](indexed, netmask, index),
		Dns:     ipList(idx(indexed, dns, index)),
		Lease:   idxAs[int](indexed, lease, index),
		Expires: idxAs[int](indexed, expires, index),
	}
}

// buildVlan mirrors `_build_vlan`: build the VLAN config for a unit.
func buildVlan(indexed indexedValues, index int) ARWanVlan {
	return ARWanVlan{
		Tagged: pyBool(idx(indexed, nvram.ARNvramIndexTypeWanDot1q, index)),
		Vid:    idxAs[int](indexed, nvram.ARNvramIndexTypeWanVid, index),
	}
}

// buildPpp mirrors `_build_ppp`: build the PPP / PPPoE config for a unit.
func buildPpp(indexed indexedValues, index int) ARWanPPP {
	return ARWanPPP{
		Echo:          idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppEcho, index),
		EchoInterval:  idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppEchoInterval, index),
		EchoFailure:   idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppEchoFailure, index),
		Conn:          idxAs[string](indexed, nvram.ARNvramIndexTypeWanPppConn, index),
		Heartbeat:     idxAs[string](indexed, nvram.ARNvramIndexTypeWanHeartbeat, index),
		PppoeAc:       idxAs[string](indexed, nvram.ARNvramIndexTypeWanPppoeAc, index),
		PppoeHostuniq: idxAs[string](indexed, nvram.ARNvramIndexTypeWanPppoeHostuniq, index),
		PppoeIdletime: idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppoeIdletime, index),
		PppoeMru:      idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppoeMru, index),
		PppoeMtu:      idxAs[int](indexed, nvram.ARNvramIndexTypeWanPppoeMtu, index),
		PppoeOptions:  idxAs[string](indexed, nvram.ARNvramIndexTypeWanPppoeOptions, index),
		PppoeService:  idxAs[string](indexed, nvram.ARNvramIndexTypeWanPppoeService, index),
	}
}

// buildSoftwire mirrors `_build_softwire`: build the Softwire46 config for
// a unit (global AFTR/B4 included).
func buildSoftwire(values map[source.ID]any, indexed indexedValues, index int) ARWanSoftwire {
	return ARWanSoftwire{
		DsliteMode:    idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46DsliteMode, index),
		DsliteService: idxAs[string](indexed, nvram.ARNvramIndexTypeWanS46DsliteSvc, index),
		Aftr:          as[identifiers.IpAddress](get(values, nvram.ARNvramTypeWanS46Aftr)),
		B4addr:        as[identifiers.IpAddress](get(values, nvram.ARNvramTypeWanS46B4addr)),
		Peer:          idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanS46Peer, index),
		Prefix4:       idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanS46Prefix4, index),
		Prefix4Len:    idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Prefix4len, index),
		Prefix6:       idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanS46Prefix6, index),
		Prefix6Len:    idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Prefix6len, index),
		EaLen:         idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Ealen, index),
		Offset:        idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Offset, index),
		Psid:          idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Psid, index),
		PsidLen:       idxAs[int](indexed, nvram.ARNvramIndexTypeWanS46Psidlen, index),
	}
}

// get is `values.get(member)` for a flat member.
func get(values map[source.ID]any, member nvram.ARNvramType) any {
	return values[source.IDOf(member)]
}

// buildUnit mirrors `_build_unit`: build a single WAN unit from translated
// values.
func buildUnit(values map[source.ID]any, indexed indexedValues, index int) ARWanUnit {
	linkMember := nvram.ARNvramTypeLinkWan1
	if index == 0 {
		linkMember = nvram.ARNvramTypeLinkWan0
	}

	dnsServers := []identifiers.IpAddress{}
	for _, dns := range []*identifiers.IpAddress{
		idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanDns1, index),
		idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanDns2, index),
	} {
		if dns != nil {
			dnsServers = append(dnsServers, *dns)
		}
	}

	unit := NewARWanUnit(index)
	unit.Enable = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanEnable, index))
	unit.Primary = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanPrimary, index))
	if v := idxAs[common.ARConnectionMethod](indexed, nvram.ARNvramIndexTypeWanProto, index); v != nil {
		unit.Protocol = *v
	}
	if v := idxAs[common.ARConnectionStatus](indexed, nvram.ARNvramIndexTypeWanState, index); v != nil {
		unit.Status = *v
	}
	if v := idxAs[common.ARConnectionStatus](indexed, nvram.ARNvramIndexTypeWanStateSub, index); v != nil {
		unit.StatusSub = *v
	}
	if v := idxAs[common.ARConnectionStatus](indexed, nvram.ARNvramIndexTypeWanStateAux, index); v != nil {
		unit.StatusAux = *v
	}
	unit.Link = pyBool(get(values, linkMember))
	unit.RealIp = idxAs[identifiers.IpAddress](indexed, nvram.ARNvramIndexTypeWanRealip, index)
	unit.RealIpState = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanRealipState, index))
	unit.Main = buildAddress(indexed, index, false)
	unit.Extra = buildAddress(indexed, index, true)
	unit.Mtu = idxAs[int](indexed, nvram.ARNvramIndexTypeWanMtu, index)
	unit.Nat = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanNat, index))
	unit.DhcpEnable = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanDhcpEnable, index))
	unit.DnsAuto = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanDnsEnable, index))
	unit.DnsServers = dnsServers
	unit.Hostname = idxAs[string](indexed, nvram.ARNvramIndexTypeWanHostname, index)
	unit.ClientId = idxAs[string](indexed, nvram.ARNvramIndexTypeWanClientid, index)
	unit.VendorId = idxAs[string](indexed, nvram.ARNvramIndexTypeWanVendorid, index)
	unit.MacClone = idxAs[identifiers.MacAddress](indexed, nvram.ARNvramIndexTypeWanMacClone, index)
	unit.Vlan = buildVlan(indexed, index)
	unit.Ppp = buildPpp(indexed, index)
	unit.Softwire = buildSoftwire(values, indexed, index)
	unit.RoutingIsp = idxAs[string](indexed, nvram.ARNvramIndexTypeWanRoutingIsp, index)
	unit.RoutingIspEnable = pyBool(idx(indexed, nvram.ARNvramIndexTypeWanRoutingIspEnable, index))
	unit.IspCountry = idxAs[string](indexed, nvram.ARNvramIndexTypeWanIspCountry, index)
	unit.IspList = idxAs[string](indexed, nvram.ARNvramIndexTypeWanIspList, index)
	unit.IspNum = idxAs[string](indexed, nvram.ARNvramIndexTypeWanIspNum, index)
	return unit
}

// buildDualwan mirrors `_build_dualwan`: build the dual WAN config, or nil
// when not reported.
func buildDualwan(values map[source.ID]any) *ARDualWan {
	mode := get(values, nvram.ARNvramTypeDualWanMode)
	priority := []string{}
	for _, item := range strList(get(values, nvram.ARNvramTypeDualWanConfig)) {
		if item != "" {
			priority = append(priority, item)
		}
	}
	if mode == nil && len(priority) == 0 {
		return nil
	}

	capability := strList(get(values, nvram.ARNvramTypeDualWanCapability))
	if capability == nil {
		capability = []string{}
	}

	state := len(priority) > 1 && priority[1] != "none"
	return &ARDualWan{
		Mode:       ARDualWanModeFromValue(mode),
		Priority:   priority,
		State:      state,
		Capability: capability,
		LanPort:    as[int](get(values, nvram.ARNvramTypeDualWanLanport)),
		LbRatio:    parseLbRatio(get(values, nvram.ARNvramTypeDualWanLbRatio)),
		Standby:    pyBool(get(values, nvram.ARNvramTypeDualWanStandby)),
		Routing:    pyBool(get(values, nvram.ARNvramTypeDualWanRouting)),
		UsbBackup:  pyBool(get(values, nvram.ARNvramTypeDualWanUsbBackup)),
		Autodetect: pyBool(get(values, nvram.ARNvramTypeWanAutodetect)),
		Extwan:     pyBool(get(values, nvram.ARNvramTypeDualWanExtwan)),
	}
}

// buildAggregation mirrors `_build_aggregation`: build the WAN aggregation
// config, or nil when not reported.
func buildAggregation(values map[source.ID]any) *ARWanAggregation {
	state := get(values, nvram.ARNvramTypeWanAggregation)
	ports := strList(get(values, nvram.ARNvramTypeWanAggregationPorts))
	if state == nil && len(ports) == 0 {
		return nil
	}

	if ports == nil {
		ports = []string{}
	}
	return &ARWanAggregation{State: pyBool(state), Ports: ports}
}

// buildWatchdog mirrors `_build_watchdog`: build the failover watchdog
// config, or nil when not reported.
func buildWatchdog(values map[source.ID]any) *ARWanWatchdog {
	enable := get(values, nvram.ARNvramTypeWatchdogEnable)
	if enable == nil {
		return nil
	}

	target := as[string](get(values, nvram.ARNvramTypeWatchdogTarget))
	if target != nil && *target == "" {
		target = nil
	}
	return &ARWanWatchdog{
		Enable:        pyBool(enable),
		Target:        target,
		Interval:      as[int](get(values, nvram.ARNvramTypeWatchdogInterval)),
		MaxFail:       as[int](get(values, nvram.ARNvramTypeWatchdogMaxFail)),
		FailbackCount: as[int](get(values, nvram.ARNvramTypeWatchdogFailbackCount)),
	}
}

// fetchState mirrors `fetch_state`: fetch the WAN state in a single
// batched NVRAM request.
func fetchState(ctx context.Context, c *registry.Context, _ source.DataSource) (any, error) {
	return nvramsource.AsyncFetchValues(ctx, c, wanRequest, nil)
}

// translateState mirrors `translate_state`: translate the fetched WAN state
// into an `ARWan`.
func translateState(raw any, _ *registry.TranslateContext) any {
	data, ok := raw.(map[source.ID]any)
	if !ok {
		return NewARWan()
	}

	// Re-key indexed values by (kind, index) once, so per-field lookups
	// are plain map gets and not fresh source-object constructions
	indexed := indexedValues{}
	for _, item := range wanRequest {
		if s, ok := item.(*nvram.ARNvramIndexSource); ok {
			if value, found := data[source.IDOf(s)]; found {
				if index, isInt := s.Index.(int); isInt {
					indexed[indexKey{s.Kind, index}] = value
				}
			}
		}
	}

	units := make(map[int]ARWanUnit, len(wanUnits))
	for _, index := range wanUnits {
		units[index] = buildUnit(data, indexed, index)
	}

	internet := common.ARConnectionStatusUnknown
	if v, found := data[source.IDOf(nvram.ARNvramTypeLinkInternet)]; found {
		if status := as[common.ARConnectionStatus](v); status != nil {
			internet = *status
		}
	}

	return ARWan{
		ActiveUnit:     as[int](get(data, nvram.ARNvramTypeWanUnit)),
		Units:          units,
		Dualwan:        buildDualwan(data),
		Aggregation:    buildAggregation(data),
		Watchdog:       buildWatchdog(data),
		InternetStatus: internet,
	}
}

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARWanSource",
		Default:   func() source.DataSource { return &ARWanSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
