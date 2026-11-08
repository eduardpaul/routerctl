package normalize

import (
	"encoding/json"
	"testing"
)

// Synthetic canonical trees in the shape of the library output (see
// asusrouter/PARITY.md "Canonical form"); values are made up.
func tree(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestIdentityAndFirmware(t *testing.T) {
	dev := tree(t, `{"brand":"ASUSTek","mac":"C8:00:00:00:00:30","model":"RT-AX88U_PRO","model_original":"RT-AX88U_Pro","serial":"SN1",
		"firmware":{"build":37544,"firmware_type":"stock","major":[3,0,0,6],"minor":102,"revision":"g5bd8654","rog":false}}`)
	id := Identity(dev)
	if id.Vendor != "ASUS" || id.Model != "RT-AX88U_PRO" || id.Firmware != "3.0.0.6.102_37544" || id.MAC != "c8:00:00:00:00:30" || id.Serial != "SN1" {
		t.Errorf("identity: %+v", id)
	}
	fw := Firmware(tree(t, `{"current":{"build":37544,"major":[3,0,0,6],"minor":102,"rog":false},
		"web":{"available":null,"state":false}}`))
	if fw.Current != "3.0.0.6.102_37544" || fw.Available != "" || fw.UpdateAvailable == nil || *fw.UpdateAvailable {
		t.Errorf("no update: %+v", fw)
	}
	fw = Firmware(tree(t, `{"current":{"build":37544,"major":[3,0,0,6],"minor":102},
		"web":{"available":{"build":38000,"major":[3,0,0,6],"minor":102},"state":true}}`))
	if fw.Available != "3.0.0.6.102_38000" || !*fw.UpdateAvailable {
		t.Errorf("update: %+v", fw)
	}
}

func TestClients(t *testing.T) {
	cs := Clients(tree(t, `{
		"aa:bb:cc:00:00:01": {"mac":"aa:bb:cc:00:00:01","name":"tv","online":true,"vendor":"LG","device_type":0,"internet_mode":"allow","bound_node":"c8:00:00:00:00:30",
			"connection":{"type":"wireless","ip":"192.168.50.148","node":"C8:00:00:00:00:31","connected_since":"2026-11-07T09:12:02+00:00",
				"links":[{"frequency":"5g","rssi":-26,"rx_speed":6291456,"tx_speed":454347980.8}],"ssid":null,"guest":false}},
		"aa:bb:cc:00:00:02": {"mac":"aa:bb:cc:00:00:02","name":"cast","online":false,"connection":null,"bound_node":null},
		"aa:bb:cc:00:00:03": {"mac":"aa:bb:cc:00:00:03","online":true,"connection":{"type":"wired","ip":"192.168.50.20","links":[]}}
	}`))
	if len(cs) != 3 {
		t.Fatalf("clients: %d", len(cs))
	}
	tv := cs[0]
	if tv.MAC != "aa:bb:cc:00:00:01" || !tv.Online || tv.IP != "192.168.50.148" || tv.Connection != "5ghz" || tv.Node != "c8:00:00:00:00:31" {
		t.Errorf("tv: %+v", tv)
	}
	if tv.RSSI == nil || *tv.RSSI != -26 || tv.RxRate == nil || *tv.RxRate != 6.29 || *tv.TxRate != 454.35 {
		t.Errorf("tv link: rssi=%v rx=%v tx=%v", tv.RSSI, tv.RxRate, tv.TxRate)
	}
	if tv.Since == nil || tv.Since.Hour() != 9 || tv.Extra["internet_mode"] != "allow" {
		t.Errorf("tv since/extra: %v %v", tv.Since, tv.Extra)
	}
	if cs[1].Online || cs[1].IP != "" || cs[1].Connection != "" {
		t.Errorf("offline: %+v", cs[1])
	}
	if cs[2].Connection != "wired" || cs[2].IP != "192.168.50.20" {
		t.Errorf("wired: %+v", cs[2])
	}
}

func TestWAN(t *testing.T) {
	w := WAN(tree(t, `{"active_unit":0,"internet_status":2,"dualwan":{"state":false},
		"units":{"0":{"index":0,"primary":true,"protocol":"static","status":2,"link":true,"real_ip":"198.51.100.4",
			"main":{"ip":"192.168.1.2","gateway":"192.168.1.1","dns":["76.76.2.2","76.76.10.2"]},"dns_servers":[]},
			"1":{"index":1,"protocol":"dhcp","status":0,"main":{"ip":"0.0.0.0","gateway":"0.0.0.0","dns":[]}}}}`))
	if w.Connected == nil || !*w.Connected || w.IP != "192.168.1.2" || w.Gateway != "192.168.1.1" || len(w.DNS) != 2 || w.Protocol != "static" {
		t.Errorf("wan: %+v", w)
	}
	if w.PublicIP != "198.51.100.4" {
		t.Errorf("public ip: %v", w.Extra)
	}
	down := WAN(tree(t, `{"active_unit":0,"internet_status":0,"units":{"0":{"main":{"ip":"0.0.0.0"}}}}`))
	if *down.Connected || down.IP != "" {
		t.Errorf("down: %+v", down)
	}
	unknown := WAN(tree(t, `{"active_unit":null,"internet_status":-999,"units":{"0":{"primary":true,"status":2,"main":{"ip":"10.0.0.2"}}}}`))
	if unknown.Connected == nil || !*unknown.Connected || unknown.IP != "10.0.0.2" {
		t.Errorf("fallback: %+v", unknown)
	}
}

func TestSystem(t *testing.T) {
	s := System(
		tree(t, `{"c8:00:00:00:00:31":{"cpu":{"usage":50},"ram":{"usage":20}},"c8:00:00:00:00:30":{"cpu":{"usage":1},"ram":{"usage":54}}}`),
		tree(t, `{"cpu":58.163,"2g1":44.01}`),
		tree(t, `{"uptime":19038}`),
		"c8:00:00:00:00:30")
	if *s.CPUUsage != 1 || *s.RAMUsage != 54 || s.Temperatures["cpu"] != 58.2 || s.Temperatures["2.4ghz"] != 44 || *s.Uptime != 19038 {
		t.Errorf("system: %+v %v", s, s.Temperatures)
	}
	if System(nil, nil, nil, "") != nil {
		t.Error("empty system should be nil")
	}
}

func TestTrafficAndNodes(t *testing.T) {
	tr := Traffic(tree(t, `{"wan":{"rx":3124228433,"rx_speed":8000,"tx":85555920,"tx_speed":0},"2g1":{"rx":1,"tx":2}}`))
	if len(tr) != 2 || tr[1].Name != "wan" || *tr[1].RxBytes != 3124228433 || *tr[1].RxRate != 1000 || *tr[1].TxRate != 0 {
		t.Errorf("traffic: %+v", tr)
	}
	ns := Nodes(
		tree(t, `{"nodes":{"c8:00:00:00:00:31":{"mac":"c8:00:00:00:00:31","alias":"Office","model":"XT8_V2","ip":"192.168.50.3","online":true},
			"c8:00:00:00:00:32":{"mac":"c8:00:00:00:00:32","alias":"","model_display":"ZenWiFi AX","online":false}}}`),
		tree(t, `{"C8:00:00:00:00:31":2}`))
	if len(ns) != 2 || ns[0].Name != "Office" || ns[0].Clients == nil || *ns[0].Clients != 2 || !ns[0].Online {
		t.Errorf("node 0: %+v", ns[0])
	}
	if ns[1].Name != "ZenWiFi AX" || ns[1].Online || ns[1].Clients != nil {
		t.Errorf("node 1: %+v", ns[1])
	}
}

func TestCountByNode(t *testing.T) {
	cs := Clients(tree(t, `{"a":{"mac":"aa:bb:cc:00:00:01","online":true,"connection":{"node":"c8:00:00:00:00:31"}},
		"b":{"mac":"aa:bb:cc:00:00:02","online":true,"connection":{"node":"c8:00:00:00:00:31"}},
		"c":{"mac":"aa:bb:cc:00:00:03","online":false,"bound_node":"c8:00:00:00:00:31"}}`))
	ns := Nodes(tree(t, `{"nodes":{"c8:00:00:00:00:31":{"mac":"c8:00:00:00:00:31","online":true}}}`), CountByNode(cs))
	if ns[0].Clients == nil || *ns[0].Clients != 2 {
		t.Errorf("clients = %v", ns[0].Clients)
	}
}
