package ztedriver

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/brands/zte"
	"github.com/eduardpaul/routerctl/routerkit/brands/zte/ztefake"
)

// testdata is the ZTE fixture set embedded in ztefake.
var testdata = ztefake.FixtureFS()

func load(t *testing.T, name string) *zte.Response {
	t.Helper()
	b, err := fs.ReadFile(testdata, name)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zte.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFingerprint(t *testing.T) {
	login, err := fs.ReadFile(testdata, "login_page.html")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		page string
		want bool
	}{
		{"F8648P login page", string(login), true},
		{"plain title + form", `<title>F6640</title><input id="Frm_Username">`, true},
		{"ZXHN title + loginData", `<title>ZXHN H298A</title><script>$.getJSON("/?_type=loginData&_tag=login_entry")</script>`, true},
		{"generic ZTE markers, other title", `<title>Router</title><div class="logo" style="background:url(Logo_ZTE.png)"></div><input name="Frm_Username">`, true},
		{"asus", `<title>ASUS Login</title><form action="/login.cgi"><input name="login_username"></form>`, false},
		{"model title alone", `<title>F8648P</title>`, false},
		{"empty", ``, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := Fingerprint(routerkit.HomeEvidence(tc.page)); got != tc.want {
				t.Errorf("Fingerprint = %v, want %v", got, tc.want)
			}
		})
	}
	ms := Factory().Matches(routerkit.HomeEvidence(string(login)))
	if len(ms) != 1 || ms[0].Driver != Name || ms[0].Model != "F8648P" || ms[0].Variant != "lua" || ms[0].Score != 90 {
		t.Errorf("login page: %+v", ms)
	}
}

func TestMapIdentity(t *testing.T) {
	info, err := zte.ParseDeviceInfo(load(t, "devmgr_statusmgr.xml"))
	if err != nil {
		t.Fatal(err)
	}
	tp, err := zte.ParseTopology(load(t, "topo.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := routerkit.Identity{Vendor: "ZTE", Model: "F8648P", Firmware: "V2.0.12P7N17", Serial: "ZTEE00000000001", MAC: "02:00:5e:00:00:01", Variant: "lua"}
	if got := MapIdentity(info, &tp); got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got := MapIdentity(info, nil); got.MAC != "" || got.Model != "F8648P" {
		t.Errorf("without topology %+v", got)
	}
}

func TestMapWAN(t *testing.T) {
	w, err := zte.ParseWANStatus(load(t, "wan_internetstatus.xml"))
	if err != nil {
		t.Fatal(err)
	}
	got := MapWAN(w)
	checks := []struct {
		name      string
		got, want any
	}{
		{"connected", *got.Connected, true},
		{"ip", got.IP, "203.0.113.10"},
		{"ipv6", got.IPv6, "2001:db8:ffff::10"},
		{"gateway", got.Gateway, "10.0.0.3"},
		{"dns", strings.Join(got.DNS, ","), "192.0.2.53,192.0.2.54"},
		{"protocol", got.Protocol, "pppoe"},
		{"uptime", *got.Uptime, int64(1605459)},
		{"name", got.Extra["name"], "INTERNET"},
		{"vlan", got.Extra["vlan"], 20},
		{"prefix", got.Extra["ipv6_prefix"], "2001:db8:100:7c00::/56"},
		{"connections", len(got.Extra["connections"].([]map[string]any)), 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if empty := MapWAN(zte.WANStatus{}); empty.Connected != nil {
		t.Errorf("empty WAN %+v", empty)
	}
}

func TestMapClients(t *testing.T) {
	hs := zte.ParseHosts(load(t, "accessdev_landevs.xml"), load(t, "wlan_client_stat_synthetic.xml"), load(t, "wlan_wlanstatus.xml"))
	cs := MapClients(hs)
	if len(cs) != 4 {
		t.Fatalf("clients %d", len(cs))
	}
	tests := []struct {
		i          int
		mac, conn  string
		rssi       int
		hasRSSI    bool
		port, name string
	}{
		{0, "02:00:5e:00:10:01", "5ghz", -52, true, "SSID5", "phone"},
		{1, "02:00:5e:00:10:02", "2.4ghz", -71, true, "SSID1", ""},
		{2, "", "wired", 0, false, "LAN1", "desktop-pc"},
		{3, "", "wired", 0, false, "LAN5", ""},
	}
	for _, tc := range tests {
		c := cs[tc.i]
		if tc.mac != "" && c.MAC != tc.mac {
			t.Errorf("%d mac %s", tc.i, c.MAC)
		}
		if c.MAC != strings.ToLower(c.MAC) || !c.Online || c.Connection != tc.conn || c.Name != tc.name || c.Extra["port"] != tc.port {
			t.Errorf("%d: %+v", tc.i, c)
		}
		if (c.RSSI != nil) != tc.hasRSSI || (tc.hasRSSI && *c.RSSI != tc.rssi) {
			t.Errorf("%d rssi %v", tc.i, c.RSSI)
		}
	}
	if cs[0].Since == nil || cs[0].TxRate == nil || *cs[0].TxRate != 866 || cs[0].Extra["ssid"] != "Example-5G" {
		t.Errorf("wifi details %+v", cs[0])
	}
}

func TestMapSystem(t *testing.T) {
	s, err := zte.ParseSystem(load(t, "devmgr_statusmgr.xml"))
	if err != nil {
		t.Fatal(err)
	}
	got := MapSystem(s, nil)
	if *got.CPUUsage != 0 || *got.RAMUsage != 62 || *got.Uptime != 1605583 || got.Temperatures != nil {
		t.Errorf("system %+v", got)
	}
	pon, err := zte.ParsePON(load(t, "optical_info_synthetic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	got = MapSystem(s, &pon)
	if got.Temperatures["pon"] != 47.5 || got.Extra["pon_rx_power_dbm"] != -19.62 || got.Extra["pon_tx_power_dbm"] != 2.31 ||
		got.Extra["pon_voltage_v"] != 3.28 || got.Extra["pon_bias_current_ma"] != 11.2 {
		t.Errorf("pon mapping %+v", got)
	}
}

func TestMapTraffic(t *testing.T) {
	tr := zte.ParseTraffic(load(t, "status_lan_info.xml"), load(t, "wan_internetstatus.xml"), load(t, "wlan_wlanstatus.xml"))
	ifs := MapTraffic(tr)
	names := []string{}
	for _, i := range ifs {
		names = append(names, i.Name)
	}
	if strings.Join(names, ",") != "wan,lan1,lan2,lan3,lan4,lan5" {
		t.Fatalf("names %v", names)
	}
	if *ifs[0].RxBytes != 1514774866 || *ifs[0].TxBytes != 3382469842 || *ifs[5].RxBytes != 68873273431 {
		t.Errorf("counters %+v %+v", ifs[0], ifs[5])
	}
}

func newDriver(t *testing.T, fake *ztefake.Server) routerkit.Driver {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	f := Factory()
	d, err := f.New(routerkit.Config{Host: srv.URL, Username: "user", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDriverEndToEnd(t *testing.T) {
	fake := ztefake.New("user", "pw", testdata)
	fake.HiddenViews["ponopticalinfo"] = true // "user" account
	fake.Files["wlan_client_stat_lua.lua"] = "wlan_client_stat_synthetic.xml"
	d := newDriver(t, fake)
	ctx := context.Background()
	if err := d.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := d.Identity(ctx)
	if err != nil || id.Model != "F8648P" || id.MAC == "" {
		t.Fatalf("identity %+v %v", id, err)
	}
	snap, err := d.Poll(ctx, routerkit.Poll(routerkit.AllParts...))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Errors) != 0 {
		t.Errorf("errors %v", snap.Errors)
	}
	if len(snap.Clients) != 4 || snap.WAN == nil || snap.System == nil || len(snap.Traffic) != 6 || snap.Firmware == nil || snap.Nodes != nil {
		t.Errorf("snapshot %+v", snap)
	}
	if snap.System.Temperatures != nil {
		t.Errorf("pon temperature without PON page: %v", snap.System.Temperatures)
	}
	if _, err := json.Marshal(snap); err != nil {
		t.Errorf("snapshot not JSON-able: %v", err)
	}
	// native sources; the hidden PON page shows in the capabilities
	caps, err := d.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(caps.Sources) < 10 || !caps.SingleSession || caps.Supports(routerkit.PartNodes) || caps.HasFeature(routerkit.FeaturePON) || caps.Parts[routerkit.PartSystem].Level != routerkit.Partial {
		t.Errorf("capabilities %+v", caps)
	}
	v, err := d.Get(ctx, "devmgr_statusmgr")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil || !strings.Contains(string(b), "OBJ_DEVINFO_ID") {
		t.Errorf("get %s %v", b, err)
	}
	if len(caps.Actions) != 0 {
		t.Error("actions exposed")
	}
	if _, err := d.Get(ctx, "no_such_source"); !errors.Is(err, routerkit.ErrNotFound) {
		t.Errorf("unknown source: %v", err)
	}
	if _, err := d.Do(ctx, routerkit.Invocation{Name: "reboot"}); !errors.Is(err, routerkit.ErrUnsupported) {
		t.Errorf("do %v", err)
	}
	if err := d.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if st := fake.Stats(); st.Logins != 1 || st.Logouts != 1 {
		t.Errorf("stats %+v", st)
	}
	if _, err := d.Poll(ctx, routerkit.Poll(routerkit.PartWAN)); !errors.Is(err, routerkit.ErrClosed) {
		t.Errorf("poll after Close: %v", err)
	}
}

func TestOptions(t *testing.T) {
	f := Factory()
	if _, err := f.New(routerkit.Config{Host: "x", Options: map[string]string{"min_interval": "fast"}}); err == nil {
		t.Error("bad min_interval accepted")
	}
	if _, err := f.New(routerkit.Config{Host: "x", Options: map[string]string{"pom": "false"}}); err == nil || !strings.Contains(err.Error(), "unknown option(s) pom") {
		t.Errorf("unknown option: %v", err)
	}
	d, err := New(routerkit.Config{Host: "x", Options: map[string]string{"pon": "off", "min_interval": "250ms"}})
	if err != nil {
		t.Fatal(err)
	}
	caps, _ := d.Capabilities(context.Background())
	if d.usePON || caps.MinInterval != 250*time.Millisecond || caps.Parts[routerkit.PartSystem].Level != routerkit.Partial {
		t.Errorf("options not applied: pon=%v caps=%+v", d.usePON, caps)
	}
}

func TestDriverPONAndUnreachable(t *testing.T) {
	fake := ztefake.New("user", "pw", testdata) // PON page visible (admin-like)
	d := newDriver(t, fake)
	snap, err := d.Poll(context.Background(), routerkit.Poll(routerkit.PartSystem))
	if err != nil {
		t.Fatal(err)
	}
	if snap.System.Temperatures["pon"] != 47.5 {
		t.Errorf("pon temp %v", snap.System.Temperatures)
	}

	f := Factory()
	dead, err := f.New(routerkit.Config{Host: "127.0.0.1:1", Username: "u", Password: "p", Timeout: 1e9})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dead.Poll(context.Background(), routerkit.Poll(routerkit.PartWAN, routerkit.PartSystem)); !errors.Is(err, routerkit.ErrUnreachable) {
		t.Errorf("unreachable device: %v", err)
	}
}
