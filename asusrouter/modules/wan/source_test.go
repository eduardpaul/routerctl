package wan

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

var testMac = func() identifiers.MacAddress {
	m, err := identifiers.MacAddressFromValue("AA:BB:CC:DD:EE:FF")
	if err != nil {
		panic(err)
	}
	return m
}()

func ip(s string) identifiers.IpAddress {
	v, err := identifiers.IpAddressFromValue(s)
	if err != nil {
		panic(err)
	}
	return v
}

func idxID(kind nvram.ARNvramIndexType, index int) source.ID {
	return source.IDOf(nvram.NewARNvramIndexSource(kind, index))
}

func flat(member nvram.ARNvramType) source.ID { return source.IDOf(member) }

// fullValues builds a translated values map for a fully populated unit 0.
func fullValues() map[source.ID]any {
	return map[source.ID]any{
		// Globals
		flat(nvram.ARNvramTypeWanUnit):               0,
		flat(nvram.ARNvramTypeLinkInternet):          common.ARConnectionStatusConnected,
		flat(nvram.ARNvramTypeLinkWan0):              true,
		flat(nvram.ARNvramTypeDualWanMode):           "lb",
		flat(nvram.ARNvramTypeDualWanConfig):         []string{"lan", "usb"},
		flat(nvram.ARNvramTypeDualWanCapability):     []string{"wan", "usb", "lan"},
		flat(nvram.ARNvramTypeDualWanLanport):        5,
		flat(nvram.ARNvramTypeDualWanLbRatio):        "9:1",
		flat(nvram.ARNvramTypeDualWanStandby):        false,
		flat(nvram.ARNvramTypeDualWanRouting):        false,
		flat(nvram.ARNvramTypeDualWanUsbBackup):      false,
		flat(nvram.ARNvramTypeDualWanExtwan):         false,
		flat(nvram.ARNvramTypeWanAutodetect):         false,
		flat(nvram.ARNvramTypeWanAggregation):        true,
		flat(nvram.ARNvramTypeWanAggregationPorts):   []string{"1", "2"},
		flat(nvram.ARNvramTypeWanS46Aftr):            ip("2001:db8::1"),
		flat(nvram.ARNvramTypeWanS46B4addr):          ip("192.0.0.2"),
		flat(nvram.ARNvramTypeWatchdogEnable):        true,
		flat(nvram.ARNvramTypeWatchdogTarget):        "8.8.8.8",
		flat(nvram.ARNvramTypeWatchdogInterval):      3,
		flat(nvram.ARNvramTypeWatchdogMaxFail):       2,
		flat(nvram.ARNvramTypeWatchdogFailbackCount): 1,
		// Unit 0 status
		idxID(nvram.ARNvramIndexTypeWanEnable, 0):      true,
		idxID(nvram.ARNvramIndexTypeWanPrimary, 0):     true,
		idxID(nvram.ARNvramIndexTypeWanProto, 0):       common.ARConnectionMethodPppoe,
		idxID(nvram.ARNvramIndexTypeWanState, 0):       common.ARConnectionStatusConnected,
		idxID(nvram.ARNvramIndexTypeWanStateSub, 0):    common.ARConnectionStatusDisconnected,
		idxID(nvram.ARNvramIndexTypeWanStateAux, 0):    common.ARConnectionStatusDisconnected,
		idxID(nvram.ARNvramIndexTypeWanRealip, 0):      ip("5.6.7.8"),
		idxID(nvram.ARNvramIndexTypeWanRealipState, 0): true,
		// Unit 0 addresses
		idxID(nvram.ARNvramIndexTypeWanIpaddr, 0):  ip("1.2.3.4"),
		idxID(nvram.ARNvramIndexTypeWanGateway, 0): ip("1.2.3.1"),
		idxID(nvram.ARNvramIndexTypeWanNetmask, 0): ip("255.255.255.0"),
		idxID(nvram.ARNvramIndexTypeWanDns, 0):     []identifiers.IpAddress{ip("8.8.8.8")},
		idxID(nvram.ARNvramIndexTypeWanLease, 0):   3600,
		idxID(nvram.ARNvramIndexTypeWanExpires, 0): 100,
		idxID(nvram.ARNvramIndexTypeWanIpaddrX, 0): ip("9.9.9.9"),
		// Unit 0 config
		idxID(nvram.ARNvramIndexTypeWanMtu, 0):              1492,
		idxID(nvram.ARNvramIndexTypeWanNat, 0):              true,
		idxID(nvram.ARNvramIndexTypeWanDhcpEnable, 0):       true,
		idxID(nvram.ARNvramIndexTypeWanDnsEnable, 0):        true,
		idxID(nvram.ARNvramIndexTypeWanDns1, 0):             ip("1.1.1.1"),
		idxID(nvram.ARNvramIndexTypeWanDns2, 0):             ip("1.0.0.1"),
		idxID(nvram.ARNvramIndexTypeWanHostname, 0):         "router",
		idxID(nvram.ARNvramIndexTypeWanClientid, 0):         "cid",
		idxID(nvram.ARNvramIndexTypeWanVendorid, 0):         "vid",
		idxID(nvram.ARNvramIndexTypeWanMacClone, 0):         testMac,
		idxID(nvram.ARNvramIndexTypeWanDot1q, 0):            true,
		idxID(nvram.ARNvramIndexTypeWanVid, 0):              10,
		idxID(nvram.ARNvramIndexTypeWanRoutingIsp, 0):       "isp",
		idxID(nvram.ARNvramIndexTypeWanRoutingIspEnable, 0): true,
		idxID(nvram.ARNvramIndexTypeWanIspCountry, 0):       "US",
		idxID(nvram.ARNvramIndexTypeWanIspList, 0):          "list",
		idxID(nvram.ARNvramIndexTypeWanIspNum, 0):           "1",
		// Unit 0 PPP
		idxID(nvram.ARNvramIndexTypeWanPppEcho, 0):         1,
		idxID(nvram.ARNvramIndexTypeWanPppEchoInterval, 0): 6,
		idxID(nvram.ARNvramIndexTypeWanPppEchoFailure, 0):  10,
		idxID(nvram.ARNvramIndexTypeWanPppConn, 0):         "conn",
		idxID(nvram.ARNvramIndexTypeWanHeartbeat, 0):       "host",
		idxID(nvram.ARNvramIndexTypeWanPppoeAc, 0):         "ac",
		idxID(nvram.ARNvramIndexTypeWanPppoeHostuniq, 0):   "uniq",
		idxID(nvram.ARNvramIndexTypeWanPppoeIdletime, 0):   0,
		idxID(nvram.ARNvramIndexTypeWanPppoeMru, 0):        1492,
		idxID(nvram.ARNvramIndexTypeWanPppoeMtu, 0):        1492,
		idxID(nvram.ARNvramIndexTypeWanPppoeOptions, 0):    "opts",
		idxID(nvram.ARNvramIndexTypeWanPppoeService, 0):    "svc",
		// Unit 0 softwire
		idxID(nvram.ARNvramIndexTypeWanS46DsliteMode, 0): 1,
		idxID(nvram.ARNvramIndexTypeWanS46DsliteSvc, 0):  "transix",
		idxID(nvram.ARNvramIndexTypeWanS46Ealen, 0):      8,
		idxID(nvram.ARNvramIndexTypeWanS46Offset, 0):     6,
		idxID(nvram.ARNvramIndexTypeWanS46Peer, 0):       ip("2001:db8::2"),
		idxID(nvram.ARNvramIndexTypeWanS46Prefix4, 0):    ip("192.0.2.0"),
		idxID(nvram.ARNvramIndexTypeWanS46Prefix4len, 0): 24,
		idxID(nvram.ARNvramIndexTypeWanS46Prefix6, 0):    ip("2001:db8::"),
		idxID(nvram.ARNvramIndexTypeWanS46Prefix6len, 0): 64,
		idxID(nvram.ARNvramIndexTypeWanS46Psid, 0):       3,
		idxID(nvram.ARNvramIndexTypeWanS46Psidlen, 0):    4,
	}
}

func translate(data any) ARWan { return translateState(data, nil).(ARWan) }

func TestARWanSource(t *testing.T) {
	if source.IDOf(&ARWanSource{}) != source.IDOf(ARWanSourceUniversal) {
		t.Error("all WAN sources should be equal")
	}
	if got := source.Repr(&ARWanSource{}); got != "<ARWanSource>" {
		t.Errorf("repr = %q", got)
	}
}

func TestWanRequest(t *testing.T) {
	ids := map[source.ID]bool{}
	for _, item := range wanRequest {
		ids[source.IDOf(item)] = true
	}
	for _, want := range []source.ID{
		flat(nvram.ARNvramTypeDualWanMode),
		flat(nvram.ARNvramTypeWanUnit),
		idxID(nvram.ARNvramIndexTypeWanMtu, 0),
		idxID(nvram.ARNvramIndexTypeWanMtu, 1),
	} {
		if !ids[want] {
			t.Errorf("request misses %s", want)
		}
	}
	// 23 flat members + 60 indexed members per unit, two units
	if len(wanRequest) != 23+60*2 {
		t.Errorf("request has %d items", len(wanRequest))
	}
}

func TestParseLbRatio(t *testing.T) {
	if got := parseLbRatio("9:1"); got == nil || *got != [2]int{9, 1} {
		t.Errorf("parseLbRatio(9:1) = %v", got)
	}
	for _, raw := range []string{"", "9", "9:1:2", "a:b"} {
		if got := parseLbRatio(raw); got != nil {
			t.Errorf("parseLbRatio(%q) = %v, want nil", raw, *got)
		}
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	// Without a data callback an empty map is returned
	got, err := fetchState(ctx, &registry.Context{}, ARWanSourceUniversal)
	if err != nil || len(got.(map[source.ID]any)) != 0 {
		t.Errorf("no callback: %v, %v", got, err)
	}

	// The fetched NVRAM map is returned as-is, for the full request
	values := map[source.ID]any{flat(nvram.ARNvramTypeWanUnit): 0}
	var requested []source.DataSource
	c := &registry.Context{FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		requested = srcs
		return values, nil
	}}
	got, err = fetchState(ctx, c, ARWanSourceUniversal)
	if err != nil || !reflect.DeepEqual(got, values) {
		t.Errorf("returns values: %v, %v", got, err)
	}
	if len(requested) != len(wanRequest) {
		t.Errorf("requested %d items", len(requested))
	}

	// A nil fetch result degrades to an empty map
	c.FetchDataMulti = func(context.Context, []source.DataSource, registry.Kwargs) (map[source.ID]any, error) {
		return nil, nil
	}
	got, err = fetchState(ctx, c, ARWanSourceUniversal)
	if err != nil || len(got.(map[source.ID]any)) != 0 {
		t.Errorf("nil values: %v, %v", got, err)
	}
}

func TestTranslateNonMap(t *testing.T) {
	wan := translate(nil)
	if wan.ActiveUnit != nil || len(wan.Units) != 0 || wan.Dualwan != nil ||
		wan.InternetStatus != common.ARConnectionStatusUnknown {
		t.Errorf("unexpected default WAN: %+v", wan)
	}
}

func TestTranslateActiveUnitAndInternet(t *testing.T) {
	wan := translate(fullValues())
	if wan.ActiveUnit == nil || *wan.ActiveUnit != 0 {
		t.Errorf("active unit = %v", wan.ActiveUnit)
	}
	if wan.InternetStatus != common.ARConnectionStatusConnected {
		t.Errorf("internet = %v", wan.InternetStatus)
	}
}

func eqIP(p *identifiers.IpAddress, s string) bool { return p != nil && p.Equal(ip(s)) }
func eqInt(p *int, v int) bool                     { return p != nil && *p == v }
func eqStr(p *string, v string) bool               { return p != nil && *p == v }

func TestTranslateFullUnit(t *testing.T) {
	u := translate(fullValues()).Units[0]
	checks := map[string]bool{
		"enable":        u.Enable,
		"primary":       u.Primary,
		"protocol":      u.Protocol == common.ARConnectionMethodPppoe,
		"status":        u.Status == common.ARConnectionStatusConnected,
		"link":          u.Link,
		"real_ip":       eqIP(u.RealIp, "5.6.7.8"),
		"real_ip_state": u.RealIpState,
		"main.ip":       eqIP(u.Main.Ip, "1.2.3.4"),
		"main.dns":      len(u.Main.Dns) == 1 && u.Main.Dns[0].Equal(ip("8.8.8.8")),
		"extra.ip":      eqIP(u.Extra.Ip, "9.9.9.9"),
		"mtu":           eqInt(u.Mtu, 1492),
		"nat":           u.Nat,
		"dhcp_enable":   u.DhcpEnable,
		"dns_auto":      u.DnsAuto,
		"dns_servers": len(u.DnsServers) == 2 && u.DnsServers[0].Equal(ip("1.1.1.1")) &&
			u.DnsServers[1].Equal(ip("1.0.0.1")),
		"hostname":            eqStr(u.Hostname, "router"),
		"client_id":           eqStr(u.ClientId, "cid"),
		"vendor_id":           eqStr(u.VendorId, "vid"),
		"mac_clone":           u.MacClone != nil && *u.MacClone == testMac,
		"routing_isp":         eqStr(u.RoutingIsp, "isp"),
		"routing_isp_enable":  u.RoutingIspEnable,
		"isp_country":         eqStr(u.IspCountry, "US"),
		"vlan.tagged":         u.Vlan.Tagged,
		"vlan.vid":            eqInt(u.Vlan.Vid, 10),
		"ppp.echo":            eqInt(u.Ppp.Echo, 1),
		"ppp.pppoe_mtu":       eqInt(u.Ppp.PppoeMtu, 1492),
		"ppp.pppoe_service":   eqStr(u.Ppp.PppoeService, "svc"),
		"softwire.dslite":     eqInt(u.Softwire.DsliteMode, 1),
		"softwire.aftr":       eqIP(u.Softwire.Aftr, "2001:db8::1"),
		"softwire.b4addr":     eqIP(u.Softwire.B4addr, "192.0.0.2"),
		"softwire.peer":       eqIP(u.Softwire.Peer, "2001:db8::2"),
		"softwire.prefix4len": eqInt(u.Softwire.Prefix4Len, 24),
		"softwire.psid":       eqInt(u.Softwire.Psid, 3),
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("%s mismatch", name)
		}
	}
}

func TestTranslateEmptyUnitDefaults(t *testing.T) {
	u := translate(fullValues()).Units[1]
	if u.Enable || u.Protocol != common.ARConnectionMethodUnknown ||
		u.Status != common.ARConnectionStatusUnknown || u.Link || u.Main.Ip != nil ||
		u.Mtu != nil || u.Nat || len(u.DnsServers) != 0 || u.MacClone != nil ||
		u.Vlan.Tagged || u.Softwire.DsliteMode != nil {
		t.Errorf("unexpected empty unit: %+v", u)
	}
}

func TestTranslateDualwanFull(t *testing.T) {
	d := translate(fullValues()).Dualwan
	if d == nil {
		t.Fatal("dualwan is nil")
	}
	if d.Mode != ARDualWanModeLoadBalance || !reflect.DeepEqual(d.Priority, []string{"lan", "usb"}) ||
		!d.State || !reflect.DeepEqual(d.Capability, []string{"wan", "usb", "lan"}) ||
		!eqInt(d.LanPort, 5) || d.LbRatio == nil || *d.LbRatio != [2]int{9, 1} {
		t.Errorf("unexpected dualwan: %+v", d)
	}
}

func TestTranslateAggregationAndWatchdog(t *testing.T) {
	wan := translate(fullValues())
	if a := wan.Aggregation; a == nil || !a.State || !reflect.DeepEqual(a.Ports, []string{"1", "2"}) {
		t.Errorf("unexpected aggregation: %+v", a)
	}
	if w := wan.Watchdog; w == nil || !w.Enable || !eqStr(w.Target, "8.8.8.8") || !eqInt(w.Interval, 3) {
		t.Errorf("unexpected watchdog: %+v", w)
	}
}

func TestTranslateBlocksNilWhenAbsent(t *testing.T) {
	wan := translate(map[source.ID]any{})
	if wan.Dualwan != nil || wan.Aggregation != nil || wan.Watchdog != nil {
		t.Errorf("blocks should be nil: %+v", wan)
	}
}

func TestTranslateDualwanNilWhenPriorityEmpty(t *testing.T) {
	wan := translate(map[source.ID]any{flat(nvram.ARNvramTypeDualWanConfig): []string{""}})
	if wan.Dualwan != nil {
		t.Errorf("dualwan should be nil: %+v", wan.Dualwan)
	}
}

func TestTranslateDualwanInactivePriority(t *testing.T) {
	wan := translate(map[source.ID]any{
		flat(nvram.ARNvramTypeDualWanMode):   "fo",
		flat(nvram.ARNvramTypeDualWanConfig): []string{"wan", "none"},
	})
	if wan.Dualwan == nil || wan.Dualwan.State {
		t.Errorf("dualwan should be inactive: %+v", wan.Dualwan)
	}
}

func TestModuleRegistersSource(t *testing.T) {
	e := registry.Source(ARWanSourceUniversal)
	if e == nil || e.Fetch == nil || e.Translate == nil {
		t.Error("ARWanSource is not registered")
	}
}

func TestEnumsRoundTrip(t *testing.T) {
	for _, m := range ARDualWanModeMembers {
		if ARDualWanModeFromValue(string(m)) != m {
			t.Errorf("ARDualWanMode %v does not round-trip", m)
		}
	}
	for _, m := range ARWANCapabilityMembers {
		if ARWANCapabilityFromValue(string(m)) != m {
			t.Errorf("ARWANCapability %v does not round-trip", m)
		}
	}
	if ARDualWanModeFromValue("zzz") != ARDualWanModeUnknown {
		t.Error("unknown fallback failed")
	}
}
