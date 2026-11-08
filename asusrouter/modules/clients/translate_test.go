package clients

import (
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

const (
	router = "AA:BB:CC:00:00:01"
	c1     = "AA:BB:CC:00:00:11"
	c2     = "AA:BB:CC:00:00:12"
	node   = "AA:BB:CC:00:00:2F"
	mib    = 1048576.0
)

func mac(s string) identifiers.MacAddress {
	m, err := identifiers.MacAddressFromValue(s)
	if err != nil {
		panic(err)
	}
	return m
}

// testIdentity is an identity whose MAC is the connected router.
func testIdentity() *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(nvram.ARNvramTypeMac): router,
	})
}

func utc(y int, mo time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, mo, d, h, mi, s, 0, time.UTC)
}

func TestUptimeTimedelta(t *testing.T) {
	cases := map[string]time.Duration{
		"01:30:15   ": time.Hour + 30*time.Minute + 15*time.Second,
		"   30:15:27": 30*time.Hour + 15*time.Minute + 27*time.Second,
		"invalid":     0,
	}
	for value, want := range cases {
		if got := uptimeTimedelta(value); got != want {
			t.Errorf("uptimeTimedelta(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestSinceDelta(t *testing.T) {
	orig := now
	now = func() time.Time { return utc(2023, 8, 15, 0, 0, 0) }
	defer func() { now = orig }()

	// 48 hours, 15 seconds
	if got := sinceDelta("48:00:15"); got == nil || !got.Equal(utc(2023, 8, 12, 23, 59, 45)) {
		t.Errorf("sinceDelta = %v", got)
	}
	if sinceDelta(nil) != nil || sinceDelta("") != nil {
		t.Error("empty input should be nil")
	}
}

func TestHelpers(t *testing.T) {
	if got := clean(map[string]any{"conn_ts ": 1, "os_type ": 2}); !reflect.DeepEqual(got, map[string]any{"conn_ts": 1, "os_type": 2}) {
		t.Errorf("clean = %v", got)
	}

	for _, c := range []struct {
		value any
		want  *float64
	}{{"1", ptr(mib)}, {"0", ptr(0.0)}, {"", nil}, {nil, nil}} {
		got := rate(c.value)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("rate(%v) = %v", c.value, got)
		}
	}

	if got := sinceTs("1700000000"); got == nil || !got.Equal(utc(2023, 11, 14, 22, 13, 20)) {
		t.Errorf("sinceTs = %v", got)
	}
	if sinceTs("0") != nil {
		t.Error("sinceTs(0) should be nil")
	}

	if sinceDelta("01:00:00") == nil || sinceDelta("") != nil {
		t.Error("sinceDelta helper mismatch")
	}

	if got := name(map[string]any{"name": "n", "nickName": "nick"}); got == nil || *got != "nick" {
		t.Errorf("name precedence = %v", got)
	}
	if got := name(map[string]any{"name": "n"}, map[string]any{"nickName": "x"}); got == nil || *got != "x" {
		t.Errorf("name across entries = %v", got)
	}
	if got := name(map[string]any{"name": ""}, map[string]any{}); got != nil {
		t.Errorf("empty name = %v", *got)
	}

	if !isMeshNode(map[string]any{"amesh_isRe": "1"}) || isMeshNode(map[string]any{"amesh_isRe": "0"}) || isMeshNode(map[string]any{}) {
		t.Error("isMeshNode mismatch")
	}
}

func ptr[T any](v T) *T { return &v }

func TestTagToBand(t *testing.T) {
	cases := []struct {
		tag  string
		want *wifi.ARWiFiBand
	}{
		{"5G", ptr(wifi.ARWiFiBandBand5g1)},
		{"5G1", ptr(wifi.ARWiFiBandBand5g1)},
		{"5G2", ptr(wifi.ARWiFiBandBand5g2)},
		{"6G1", ptr(wifi.ARWiFiBandBand6g1)},
		{"2G", ptr(wifi.ARWiFiBandBand2g1)},
		{"7G1", nil},
		{"bad", nil},
	}
	for _, c := range cases {
		got := tagToBand(c.tag)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("tagToBand(%q) = %v", c.tag, got)
		}
	}
}

func TestBuildClientsNonMap(t *testing.T) {
	if got := BuildClients(nil, testIdentity()); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestBuildClientsSkipsNonClientKeys(t *testing.T) {
	raw := map[string]any{"get_clientlist": map[string]any{
		"maclist":        []any{"x"},
		"ClientAPILevel": "7",
		"not-a-mac":      map[string]any{"mac": "not-a-mac"},
		"bad":            "not-a-dict",
	}}
	if got := BuildClients(raw, testIdentity()); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestBuildClientsExcludesMeshNodeAndRouter(t *testing.T) {
	raw := map[string]any{"get_clientlist": map[string]any{
		node:   map[string]any{"mac": node, "amesh_isRe": "1", "isOnline": "1"},
		router: map[string]any{"mac": router, "isOnline": "1"},
		c1:     map[string]any{"mac": c1, "isOnline": "1", "isWL": "0"},
	}}
	got := BuildClients(raw, testIdentity())
	if _, ok := got[mac(c1)]; !ok || len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestBuildClientsDbOnlyOffline(t *testing.T) {
	raw := map[string]any{"get_clientlist_from_json_database": map[string]any{
		c2: map[string]any{"mac": c2, "name": "Old", "vendor": "Acme", "os_type": "3", "amesh_isRe": "0"},
	}}
	client := BuildClients(raw, testIdentity())[mac(c2)]
	if client.Online || client.Connection != nil || client.Name == nil || *client.Name != "Old" ||
		client.OsType == nil || *client.OsType != 3 || client.DeviceType != common.ARDeviceTypeUnknown {
		t.Errorf("got %+v", client)
	}
}

func TestBuildClientsDbOnlyMeshNodeExcluded(t *testing.T) {
	raw := map[string]any{"get_clientlist_from_json_database": map[string]any{
		node: map[string]any{"mac": node, "amesh_isRe": "1"},
	}}
	if got := BuildClients(raw, testIdentity()); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestBuildClientsOnlineWirelessWithDbMerge(t *testing.T) {
	raw := map[string]any{
		"get_clientlist": map[string]any{c1: map[string]any{
			"mac": c1, "isOnline": "1", "isWL": "2", "rssi": "-46", "curRx": "1", "curTx": "0",
			"ip": "192.168.1.5", "ipMethod": "DHCP", "internetState": "1", "internetMode": "allow",
			"amesh_papMac": node, "ssid": "Net", "wlAuth": "WPA2-PSK", "isGN": "", "sdn_type": "MAINFH",
			"mlo": "0", "name": "PC", "nickName": "MyPC", "vendor": "Apple, Inc.", "type": "5",
		}},
		"get_clientlist_from_json_database": map[string]any{c1: map[string]any{
			"mac": c1, "vendorclass": "Apple", "os_type": "2", "conn_ts": "1700000000",
		}},
	}
	client := BuildClients(raw, testIdentity())[mac(c1)]
	if !client.Online || *client.Name != "MyPC" || *client.Vendor != "Apple, Inc." ||
		*client.VendorClass != "Apple" || client.DeviceType != common.ARDeviceTypeIpCam ||
		*client.OsType != 2 || client.InternetMode != common.ARInternetModeAllow {
		t.Errorf("client = %+v", client)
	}

	conn := client.Connection
	if conn == nil {
		t.Fatal("connection is nil")
	}
	wantIP, _ := identifiers.IpAddressFromValue("192.168.1.5")
	if conn.Type != common.ARConnectionTypeWireless || conn.Ip == nil || !conn.Ip.Equal(wantIP) ||
		conn.IpMethod != common.ARIPMethodDhcp || conn.InternetAccess == nil || !*conn.InternetAccess ||
		conn.Node == nil || *conn.Node != mac(node) || *conn.Ssid != "Net" ||
		conn.Security == nil || *conn.Security != wifi.ARWiFiAuthWpa2Psk || conn.Guest || conn.Mlo ||
		conn.ConnectedSince == nil || !conn.ConnectedSince.Equal(utc(2023, 11, 14, 22, 13, 20)) {
		t.Errorf("connection = %+v", conn)
	}
	if len(conn.Links) != 1 {
		t.Fatalf("links = %v", conn.Links)
	}
	link := conn.Links[0]
	if link.Frequency != wifi.ARWiFiFrequencyFreq5g || *link.Rssi != -46 || *link.RxSpeed != mib || *link.TxSpeed != 0 {
		t.Errorf("link = %+v", link)
	}
}

func online(t *testing.T, extra map[string]any) *ARClientConnection {
	t.Helper()
	entry := map[string]any{"mac": c1, "isOnline": "1"}
	for k, v := range extra {
		entry[k] = v
	}
	conn := BuildClients(map[string]any{"get_clientlist": map[string]any{c1: entry}}, testIdentity())[mac(c1)].Connection
	if conn == nil {
		t.Fatal("connection is nil")
	}
	return conn
}

func TestConnection(t *testing.T) {
	// A wired client has WIRED type and no links; empty papMac falls back
	// to the router
	conn := online(t, map[string]any{"isWL": "0", "amesh_papMac": ""})
	if conn.Type != common.ARConnectionTypeWired || len(conn.Links) != 0 || conn.Mlo ||
		conn.Node == nil || *conn.Node != mac(router) {
		t.Errorf("wired = %+v", conn)
	}

	if !online(t, map[string]any{"isWL": "1", "isGN": "2", "sdn_type": "LEGACY"}).Guest {
		t.Error("guest expected")
	}
	if online(t, map[string]any{"isWL": "1", "isGN": "2", "sdn_type": "MAINFH"}).Guest {
		t.Error("main fronthaul is not a guest")
	}
	if online(t, map[string]any{"isWL": "1"}).Security != nil {
		t.Error("security should be unset")
	}
	if online(t, map[string]any{"isWL": "3"}).Links[0].Frequency != wifi.ARWiFiFrequencyFreq5g {
		t.Error("isWL 3 should normalize to 5G")
	}
	if len(online(t, map[string]any{"isWL": "9"}).Links) != 0 {
		t.Error("unknown isWL should give no links")
	}
}

func TestLiveMloLinks(t *testing.T) {
	conn := online(t, map[string]any{
		"isWL": "2", "mlo": "1",
		"mlo_links": map[string]any{
			"5G1": map[string]any{"mac": c2, "rssi": "-63", "tx": "1", "rx": "0", "conn_time": "01:00:00"},
			"7G":  map[string]any{"mac": c2, "rssi": "-70"},
			"bad": "not-a-dict",
		},
	})
	if !conn.Mlo || len(conn.Links) != 2 {
		t.Fatalf("connection = %+v", conn)
	}
	link := conn.Links[0]
	if link.Band == nil || *link.Band != wifi.ARWiFiBandBand5g1 || link.Frequency != wifi.ARWiFiFrequencyFreq5g ||
		*link.Mac != mac(c2) || *link.Rssi != -63 || *link.TxSpeed != mib || link.ConnectedSince == nil {
		t.Errorf("link = %+v", link)
	}
	// Unmapped radio -> no band, unknown frequency
	if conn.Links[1].Band != nil || conn.Links[1].Frequency != wifi.ARWiFiFrequencyUnknown {
		t.Errorf("unmapped link = %+v", conn.Links[1])
	}
}

func TestOfflineMloFromDbMacs(t *testing.T) {
	raw := map[string]any{
		"get_clientlist": map[string]any{c1: map[string]any{"mac": c1, "isOnline": "1", "isWL": "2", "mlo": "1"}},
		"get_clientlist_from_json_database": map[string]any{c1: map[string]any{
			"mac": c1, "mlo": "1",
			"mlo_5G_mac":  c2,       // -> BAND_5G1
			"mlo_5G2_mac": node,     // -> BAND_5G2
			"mlo_7G_mac":  c2,       // unmappable band -> skipped
			"mlo_all_mac": "<" + c2, // combined -> skipped
			"vendor":      "x",      // non-mlo field -> skipped
		}},
	}
	conn := BuildClients(raw, testIdentity())[mac(c1)].Connection
	if conn == nil || !conn.Mlo {
		t.Fatalf("connection = %+v", conn)
	}
	bands := map[wifi.ARWiFiBand]bool{}
	for _, link := range conn.Links {
		bands[*link.Band] = true
	}
	if !reflect.DeepEqual(bands, map[wifi.ARWiFiBand]bool{wifi.ARWiFiBandBand5g1: true, wifi.ARWiFiBandBand5g2: true}) {
		t.Errorf("bands = %v", bands)
	}
}

func TestMloFlaggedNoDataFallsBack(t *testing.T) {
	conn := online(t, map[string]any{"isWL": "1", "mlo": "1"})
	if len(conn.Links) != 1 || conn.Links[0].Frequency != wifi.ARWiFiFrequencyFreq2g {
		t.Errorf("links = %+v", conn.Links)
	}
}
