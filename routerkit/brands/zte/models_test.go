package zte

import (
	"testing"
	"time"
)

func TestParseDeviceInfoAndSystem(t *testing.T) {
	r := fixture(t, "devmgr_statusmgr.xml")
	d, err := ParseDeviceInfo(r)
	if err != nil {
		t.Fatal(err)
	}
	want := DeviceInfo{
		Model: "F8648P", Manufacturer: "ZTE", ManufacturerOUI: "646E60",
		SoftwareVersion: "V2.0.12P7N17", SoftwareExtended: "07e8P7N1701a7",
		HardwareVersion: "V2.0", BootVersion: "V2.0.12P10N5", BuildDate: "2024-04-23 02:04:04",
		SerialNumber: "ZTEE00000000001", PONSerial: "ZTEG00000001", Uptime: 1605583 * time.Second,
	}
	if d != want {
		t.Errorf("device info\n got %+v\nwant %+v", d, want)
	}
	s, err := ParseSystem(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.CPUUsage) != 4 || s.CPUAverage != 0 || s.MemUsage == nil || *s.MemUsage != 62 || s.Uptime != want.Uptime {
		t.Errorf("system %+v", s)
	}
	if _, err := ParseDeviceInfo(&Response{}); err == nil {
		t.Error("empty response accepted")
	}
}

func TestParseWANStatus(t *testing.T) {
	w, err := ParseWANStatus(fixture(t, "wan_internetstatus.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Connections) != 2 {
		t.Fatalf("connections %d", len(w.Connections))
	}
	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"0.name", w.Connections[0].Name, "omci_ipv4_static_1"},
		{"0.protocol", w.Connections[0].Protocol, "static"},
		{"0.defaultgw", w.Connections[0].DefaultGW, false},
		{"0.counters", w.Connections[0].RxBytes == nil, true},
		{"1.name", w.Connections[1].Name, "INTERNET"},
		{"1.protocol", w.Connections[1].Protocol, "pppoe"},
		{"1.connected", w.Connections[1].Connected, true},
		{"1.ip", w.Connections[1].IP, "203.0.113.10"},
		{"1.gateway", w.Connections[1].Gateway, "10.0.0.3"},
		{"1.dns", len(w.Connections[1].DNS), 2},
		{"1.ipv6", w.Connections[1].IPv6, "2001:db8:ffff::10"},
		{"1.prefix", w.Connections[1].IPv6Prefix, "2001:db8:100:7c00::/56"},
		{"1.dns6", len(w.Connections[1].DNS6), 2},
		{"1.vlan", w.Connections[1].VLAN, 20},
		{"1.mtu", w.Connections[1].MTU, 1492},
		{"1.uptime", w.Connections[1].Uptime, 1605459 * time.Second},
		{"1.rx", *w.Connections[1].RxBytes, uint64(1514774866)},
		{"1.tx", *w.Connections[1].TxBytes, uint64(3382469842)},
		{"1.error", w.Connections[1].Error, ""},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.field, tc.got, tc.want)
		}
	}
	in, ok := w.Internet()
	if !ok || in.Name != "INTERNET" {
		t.Errorf("internet = %+v", in)
	}
}

func TestParseHosts(t *testing.T) {
	lan := fixture(t, "accessdev_landevs.xml")
	t.Run("real fixtures (no WiFi clients)", func(t *testing.T) {
		hs := ParseHosts(lan, fixture(t, "wlan_client_stat.xml"), nil)
		if len(hs) != 2 {
			t.Fatalf("hosts %d", len(hs))
		}
		if hs[0].Interface != "lan" || hs[0].Port != "LAN1" || hs[0].HostName != "desktop-pc" || hs[0].IP != "192.168.1.129" {
			t.Errorf("host0 %+v", hs[0])
		}
		if hs[1].HostName != "" || hs[1].Port != "LAN5" {
			t.Errorf("host1 %+v", hs[1])
		}
	})
	t.Run("synthetic WiFi clients", func(t *testing.T) {
		hs := ParseHosts(lan, fixture(t, "wlan_client_stat_synthetic.xml"), fixture(t, "wlan_wlanstatus.xml"))
		if len(hs) != 4 {
			t.Fatalf("hosts %d", len(hs))
		}
		w := hs[0]
		if w.MAC != "02:00:5e:00:10:01" || w.Band != "5GHz" || w.SSID != "Example-5G" || w.Interface != "wlan" {
			t.Errorf("wifi0 %+v", w)
		}
		if w.RSSI == nil || *w.RSSI != -52 || w.SNR == nil || *w.SNR != 41 || w.Noise == nil || *w.Noise != -93 {
			t.Errorf("wifi0 signal %+v", w)
		}
		if w.TxRate == nil || *w.TxRate != 866 || w.RxRate == nil || *w.RxRate != 780 {
			t.Errorf("wifi0 rates %v %v", w.TxRate, w.RxRate)
		}
		if w.LinkTime == nil || *w.LinkTime != time.Hour {
			t.Errorf("wifi0 link time %v", w.LinkTime)
		}
		w = hs[1]
		if w.Band != "2.4GHz" || *w.RSSI != -71 || w.SNR != nil || *w.TxRate != 72 || w.SSID != "Example-Guest" {
			t.Errorf("wifi1 %+v", w)
		}
	})
	t.Run("band fallback without radio data", func(t *testing.T) {
		hs := ParseHosts(nil, fixture(t, "wlan_client_stat_synthetic.xml"), nil)
		if hs[0].Band != "5GHz" || hs[1].Band != "2.4GHz" {
			t.Errorf("bands %q %q", hs[0].Band, hs[1].Band)
		}
	})
}

func TestParseRate(t *testing.T) {
	for in, want := range map[string]float64{"866": 866, "780000": 780, "72 Mbps": 72, "1.2Gbps": 1200, "54000 Kbps": 54} {
		if got := parseRate(in); got == nil || *got != want {
			t.Errorf("%q = %v want %v", in, got, want)
		}
	}
	if parseRate("") != nil {
		t.Error("empty rate parsed")
	}
}

func TestParseWiFi(t *testing.T) {
	w := ParseWiFi(fixture(t, "wlan_wlanbasicadconf.xml"), fixture(t, "wlan_wlansssidconf.xml"), fixture(t, "wlan_wlanstatus.xml"))
	if len(w.Radios) != 2 {
		t.Fatalf("radios %+v", w.Radios)
	}
	r24, r5 := w.Radios[0], w.Radios[1]
	if r24.Band != "2.4GHz" || !r24.Enabled || r24.Channel != 1 || !r24.AutoChannel || r24.Standard != "b,g,n,ax" || r24.TxPower != "60%" {
		t.Errorf("2.4 %+v", r24)
	}
	if r5.Band != "5GHz" || r5.Enabled || r5.Channel != 36 {
		t.Errorf("5 %+v", r5)
	}
	if len(r24.SSIDs) != 4 || len(r5.SSIDs) != 4 {
		t.Fatalf("ssids %d/%d", len(r24.SSIDs), len(r5.SSIDs))
	}
	s := r24.SSIDs[0]
	if s.Alias != "SSID1" || s.Name != "Example-Guest" || s.Enabled || !s.Hidden || s.Security != "wpa2" || s.BSSID == "" || s.MaxClients != 32 {
		t.Errorf("ssid1 %+v", s)
	}
	if s.RxBytes == nil || *s.RxBytes != 0 {
		t.Errorf("ssid1 counters %+v", s)
	}
	if r5.SSIDs[0].Name != "Example-5G" {
		t.Errorf("ssid5 %+v", r5.SSIDs[0])
	}
	// status only (Traffic path)
	if w := ParseWiFi(nil, nil, fixture(t, "wlan_wlanstatus.xml")); len(w.Radios) != 2 || len(w.Radios[1].SSIDs) != 4 {
		t.Errorf("status-only %+v", w)
	}
}

func TestParsePON(t *testing.T) {
	p, err := ParsePON(fixture(t, "optical_info_synthetic.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if *p.RxPower != -19.62 || *p.TxPower != 2.31 || *p.Temperature != 47.5 || *p.Voltage != 3.28 || *p.BiasCurrent != 11.2 {
		t.Errorf("pon %+v", p)
	}
	if _, err := ParsePON(fixture(t, "devmgr_statusmgr.xml")); err == nil {
		t.Error("non-optical response accepted")
	}
}

func TestParseTraffic(t *testing.T) {
	tr := ParseTraffic(fixture(t, "status_lan_info.xml"), fixture(t, "wan_internetstatus.xml"), fixture(t, "wlan_wlanstatus.xml"))
	if len(tr.Ports) != 5 {
		t.Fatalf("ports %d", len(tr.Ports))
	}
	p1, p2, p5 := tr.Ports[0], tr.Ports[1], tr.Ports[4]
	if p1.Name != "LAN1" || !p1.Up || p1.SpeedMbps != 100 || p1.RxBytes != 15319675388 || p1.TxBytes != 16145705391 {
		t.Errorf("lan1 %+v", p1)
	}
	if p2.Up || p2.RxBytes != 0 {
		t.Errorf("lan2 %+v", p2)
	}
	if p5.Name != "LAN5" || p5.SpeedMbps != 2500 || p5.RxBytes != 68873273431 {
		t.Errorf("lan5 %+v", p5)
	}
	if len(tr.WAN) != 1 || tr.WAN[0].Name != "INTERNET" {
		t.Errorf("wan %+v", tr.WAN)
	}
	if len(tr.SSIDs) != 0 { // every SSID is disabled on this unit
		t.Errorf("ssids %+v", tr.SSIDs)
	}
}

func TestParseTopology(t *testing.T) {
	tp, err := ParseTopology(fixture(t, "topo.json"))
	if err != nil {
		t.Fatal(err)
	}
	if tp.MAC != "02:00:5e:00:00:01" || tp.IP != "192.168.1.1" || tp.DeviceName != "ZTE:F8648P" || tp.Clients == nil || *tp.Clients != 2 {
		t.Errorf("topology %+v", tp)
	}
}
