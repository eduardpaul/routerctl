package zte

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DeviceInfo is the device identity (Administration > Status).
type DeviceInfo struct {
	Model            string        `json:"model"`
	Manufacturer     string        `json:"manufacturer,omitempty"`
	ManufacturerOUI  string        `json:"manufacturer_oui,omitempty"`
	SoftwareVersion  string        `json:"software_version"`
	SoftwareExtended string        `json:"software_version_extended,omitempty"`
	HardwareVersion  string        `json:"hardware_version,omitempty"`
	BootVersion      string        `json:"boot_version,omitempty"`
	BuildDate        string        `json:"build_date,omitempty"`
	SerialNumber     string        `json:"serial_number,omitempty"`
	PONSerial        string        `json:"pon_serial,omitempty"`
	Alias            string        `json:"alias,omitempty"`
	Uptime           time.Duration `json:"uptime"`
}

// System is the device health (Administration > Status).
type System struct {
	// CPUUsage per core, percent
	CPUUsage []float64 `json:"cpu_usage"`
	// CPUAverage is the mean of CPUUsage
	CPUAverage float64 `json:"cpu_average"`
	// MemUsage percent (nil when not reported)
	MemUsage *float64      `json:"mem_usage,omitempty"`
	Uptime   time.Duration `json:"uptime"`
}

// WANConnection is one WAN connection (Internet > Status > WAN).
type WANConnection struct {
	ID        string `json:"id"`   // IGD.WD1.WCD1.WCPPP2
	Name      string `json:"name"` // WANCName ("INTERNET", "omci_ipv4_static_1")
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
	Status    string `json:"status"`            // ConnStatus ("Connected", ...)
	Status6   string `json:"status6,omitempty"` // ConnStatus6
	Error     string `json:"error,omitempty"`   // ConnError
	// Protocol: "pppoe", "dhcp", "static" or the raw type
	Protocol   string        `json:"protocol"`
	LinkMode   string        `json:"link_mode,omitempty"` // PPP / IP
	IPMode     string        `json:"ip_mode,omitempty"`   // IPv4 / IPv6 / Both
	Services   string        `json:"services,omitempty"`  // StrServList: INTERNET, TR069_VoIP, ...
	DefaultGW  bool          `json:"default_gw"`
	NAT        bool          `json:"nat"`
	IP         string        `json:"ip,omitempty"`
	Netmask    string        `json:"netmask,omitempty"`
	Gateway    string        `json:"gateway,omitempty"`
	DNS        []string      `json:"dns,omitempty"`
	IPv6       string        `json:"ipv6,omitempty"`        // first global address
	IPv6Prefix string        `json:"ipv6_prefix,omitempty"` // delegated prefix, "p::/56"
	Gateway6   string        `json:"gateway6,omitempty"`
	DNS6       []string      `json:"dns6,omitempty"`
	MAC        string        `json:"mac,omitempty"`
	VLAN       int           `json:"vlan,omitempty"`
	MTU        int           `json:"mtu,omitempty"`
	Uptime     time.Duration `json:"uptime"`
	Uptime6    time.Duration `json:"uptime6,omitempty"`
	RxBytes    *uint64       `json:"rx_bytes,omitempty"`
	TxBytes    *uint64       `json:"tx_bytes,omitempty"`
	RxPackets  *uint64       `json:"rx_packets,omitempty"`
	TxPackets  *uint64       `json:"tx_packets,omitempty"`
}

// WANStatus lists the WAN connections.
type WANStatus struct {
	Connections []WANConnection `json:"connections"`
}

// Internet returns the connection carrying the default route (or the
// first connected one with the INTERNET service, or the first one).
func (w WANStatus) Internet() (WANConnection, bool) {
	for _, c := range w.Connections {
		if c.DefaultGW {
			return c, true
		}
	}
	for _, c := range w.Connections {
		if c.Connected && strings.Contains(strings.ToUpper(c.Services), "INTERNET") {
			return c, true
		}
	}
	if len(w.Connections) > 0 {
		return w.Connections[0], true
	}
	return WANConnection{}, false
}

// Host is a client seen by the device (wired or WiFi).
type Host struct {
	MAC       string `json:"mac"` // lower-case, colon separated
	IP        string `json:"ip,omitempty"`
	IPv6      string `json:"ipv6,omitempty"`
	HostName  string `json:"hostname,omitempty"`
	Interface string `json:"interface"` // "lan" or "wlan"
	// Port: "LAN1".."LAN5" for wired, the SSID alias ("SSID5") for WiFi
	Port string `json:"port,omitempty"`
	// WiFi only
	Band      string         `json:"band,omitempty"` // "2.4GHz", "5GHz"
	SSID      string         `json:"ssid,omitempty"`
	RSSI      *int           `json:"rssi,omitempty"` // dBm
	SNR       *int           `json:"snr,omitempty"`
	Noise     *int           `json:"noise,omitempty"`
	TxRate    *float64       `json:"tx_rate,omitempty"` // Mbit/s
	RxRate    *float64       `json:"rx_rate,omitempty"` // Mbit/s
	LinkTime  *time.Duration `json:"link_time,omitempty"`
	Mode      string         `json:"mode,omitempty"`      // CurrentMode (802.11 mode)
	Bandwidth string         `json:"bandwidth,omitempty"` // BAND
	RxBytes   *uint64        `json:"rx_bytes,omitempty"`
	TxBytes   *uint64        `json:"tx_bytes,omitempty"`
}

// SSID is one WLAN access point (virtual AP).
type SSID struct {
	ID             string  `json:"id"`    // DEV.WIFI.AP5
	Alias          string  `json:"alias"` // SSID5
	Name           string  `json:"name"`  // ESSID
	Enabled        bool    `json:"enabled"`
	Hidden         bool    `json:"hidden"`
	Security       string  `json:"security,omitempty"` // open, wep, wpa, wpa2, wpa/wpa2, wpa3, wpa2/wpa3
	BSSID          string  `json:"bssid,omitempty"`
	ChannelInUse   int     `json:"channel_in_use,omitempty"`
	BandwidthInUse string  `json:"bandwidth_in_use,omitempty"`
	MaxClients     int     `json:"max_clients,omitempty"`
	RxBytes        *uint64 `json:"rx_bytes,omitempty"`
	TxBytes        *uint64 `json:"tx_bytes,omitempty"`
	RxPackets      *uint64 `json:"rx_packets,omitempty"`
	TxPackets      *uint64 `json:"tx_packets,omitempty"`
}

// Radio is one WLAN radio with its SSIDs.
type Radio struct {
	ID          string `json:"id"`   // DEV.WIFI.RD1
	Band        string `json:"band"` // "2.4GHz", "5GHz"
	Enabled     bool   `json:"enabled"`
	Channel     int    `json:"channel"` // configured (0/auto: see AutoChannel)
	AutoChannel bool   `json:"auto_channel"`
	Bandwidth   string `json:"bandwidth,omitempty"`
	Standard    string `json:"standard,omitempty"`
	TxPower     string `json:"tx_power,omitempty"`
	Country     string `json:"country,omitempty"`
	SSIDs       []SSID `json:"ssids"`
}

// WiFi is the WLAN state.
type WiFi struct {
	Radios []Radio `json:"radios"`
}

// PON is the optical module state. Fields are nil when not reported.
type PON struct {
	RxPower     *float64 `json:"rx_power_dbm,omitempty"`
	TxPower     *float64 `json:"tx_power_dbm,omitempty"`
	Temperature *float64 `json:"temperature_c,omitempty"`
	Voltage     *float64 `json:"voltage_v,omitempty"`
	BiasCurrent *float64 `json:"bias_current_ma,omitempty"`
	Raw         Instance `json:"raw,omitempty"`
}

// Port is one Ethernet port.
type Port struct {
	Name       string `json:"name"` // LAN1..
	ID         string `json:"id"`   // DEV.ETH.IF1
	Up         bool   `json:"up"`
	SpeedMbps  int    `json:"speed_mbps,omitempty"`
	Duplex     string `json:"duplex,omitempty"`
	RxBytes    uint64 `json:"rx_bytes"`
	TxBytes    uint64 `json:"tx_bytes"`
	RxPackets  uint64 `json:"rx_packets"`
	TxPackets  uint64 `json:"tx_packets"`
	RxErrors   uint64 `json:"rx_errors"`
	TxErrors   uint64 `json:"tx_errors"`
	RxDiscards uint64 `json:"rx_discards"`
	TxDiscards uint64 `json:"tx_discards"`
}

// Counters is a named byte/packet counter set.
type Counters struct {
	Name      string  `json:"name"`
	RxBytes   *uint64 `json:"rx_bytes,omitempty"`
	TxBytes   *uint64 `json:"tx_bytes,omitempty"`
	RxPackets *uint64 `json:"rx_packets,omitempty"`
	TxPackets *uint64 `json:"tx_packets,omitempty"`
}

// Traffic is the per-interface traffic. Rx is what the device received
// on the interface (from the client side for LAN ports/SSIDs). WAN lists
// the connections that report counters, the internet one first.
type Traffic struct {
	Ports []Port     `json:"ports"`
	WAN   []Counters `json:"wan"`
	SSIDs []Counters `json:"ssids"`
}

// Topology is the topology/EasyMesh controller summary.
type Topology struct {
	DeviceName      string   `json:"device_name,omitempty"` // "ZTE:F8648P"
	MAC             string   `json:"mac,omitempty"`
	IP              string   `json:"ip,omitempty"`
	SoftwareVersion string   `json:"software_version,omitempty"`
	Clients         *int     `json:"clients,omitempty"`
	Raw             Instance `json:"raw,omitempty"`
}

// ---------------------------------------------------------------- parsers

func ptrU(in Instance, k string) *uint64 {
	if v, ok := in.Uint(k); ok {
		return &v
	}
	return nil
}

func ptrI(in Instance, k string) *int {
	if v, ok := in.Float(k); ok {
		i := int(v)
		return &i
	}
	return nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func seconds(in Instance, k string) time.Duration {
	v, _ := in.Int(k)
	return time.Duration(v) * time.Second
}

func normMAC(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func ipSet(vals ...string) []string {
	var out []string
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" || v == "0.0.0.0" || v == "::" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ParseDeviceInfo maps devmgr_statusmgr_lua.lua.
func ParseDeviceInfo(r *Response) (DeviceInfo, error) {
	d := r.First("OBJ_DEVINFO_ID")
	if len(d) == 0 {
		return DeviceInfo{}, fmt.Errorf("%w: no OBJ_DEVINFO_ID", ErrUnexpected)
	}
	return DeviceInfo{
		Model:            d.Str("ModelName"),
		Manufacturer:     d.Str("ManuFacturer"),
		ManufacturerOUI:  d.Str("ManuFacturerOui"),
		SoftwareVersion:  d.Str("SoftwareVer"),
		SoftwareExtended: d.Str("SoftwareVerExtent"),
		HardwareVersion:  d.Str("HardwareVer"),
		BootVersion:      d.Str("BootVer"),
		BuildDate:        d.Str("VerDate"),
		SerialNumber:     d.Str("SerialNumber"),
		PONSerial:        r.First("OBJ_SN_INFO_ID").Str("Sn"),
		Alias:            d.Str("OnuAlias"),
		Uptime:           seconds(r.First("OBJ_POWERONTIME_ID"), "PowerOnTime"),
	}, nil
}

// ParseSystem maps devmgr_statusmgr_lua.lua (CpuUsage1..N, MemUsage).
func ParseSystem(r *Response) (System, error) {
	cm := r.First("OBJ_CPUMEMUSAGE_ID")
	if len(cm) == 0 {
		return System{}, fmt.Errorf("%w: no OBJ_CPUMEMUSAGE_ID", ErrUnexpected)
	}
	s := System{CPUUsage: []float64{}, Uptime: seconds(r.First("OBJ_POWERONTIME_ID"), "PowerOnTime")}
	for i := 1; i <= 64; i++ {
		v, ok := cm.Float("CpuUsage" + strconv.Itoa(i))
		if !ok {
			break
		}
		s.CPUUsage = append(s.CPUUsage, v)
	}
	if len(s.CPUUsage) == 0 {
		if v, ok := cm.Float("CpuUsage"); ok {
			s.CPUUsage = append(s.CPUUsage, v)
		}
	}
	for _, v := range s.CPUUsage {
		s.CPUAverage += v
	}
	if n := len(s.CPUUsage); n > 0 {
		s.CPUAverage /= float64(n)
	}
	if v, ok := cm.Float("MemUsage"); ok {
		s.MemUsage = &v
	}
	return s, nil
}

func wanProtocol(in Instance) string {
	if strings.EqualFold(in.Str("TransType"), "PPPoE") || strings.EqualFold(in.Str("wantype"), "pppoe") || strings.EqualFold(in.Str("linkMode"), "PPP") {
		return "pppoe"
	}
	switch strings.ToLower(in.Str("Addressingtype")) {
	case "dhcp":
		return "dhcp"
	case "static":
		return "static"
	}
	if t := in.Str("wantype"); t != "" {
		return strings.ToLower(t)
	}
	return strings.ToLower(in.Str("linkMode"))
}

// ParseWANStatus maps wan_internetstatus_lua.lua (ID_WAN_COMFIG).
func ParseWANStatus(r *Response) (WANStatus, error) {
	insts, ok := r.Objects["ID_WAN_COMFIG"]
	if !ok {
		return WANStatus{}, fmt.Errorf("%w: no ID_WAN_COMFIG", ErrUnexpected)
	}
	w := WANStatus{Connections: []WANConnection{}}
	for _, in := range insts {
		c := WANConnection{
			ID:        in.Str("_InstID"),
			Name:      in.Str("WANCName"),
			Enabled:   in.Bool("Enable"),
			Status:    in.Str("ConnStatus"),
			Status6:   in.Str("ConnStatus6"),
			Protocol:  wanProtocol(in),
			LinkMode:  in.Str("linkMode"),
			IPMode:    in.Str("IpMode"),
			Services:  in.Str("StrServList"),
			DefaultGW: in.Bool("IsDefGW"),
			NAT:       in.Bool("IsNAT"),
			IP:        firstNonEmpty(ipSet(in.Str("IPAddress"))...),
			Netmask:   in.Str("SubnetMask"),
			Gateway:   firstNonEmpty(ipSet(in.Str("GateWay"))...),
			DNS:       ipSet(in.Str("DNS1"), in.Str("DNS2"), in.Str("DNS3")),
			DNS6:      ipSet(in.Str("Dns1v6"), in.Str("Dns2v6"), in.Str("Dns3v6")),
			Gateway6:  firstNonEmpty(ipSet(in.Str("Gateway6"))...),
			MAC:       normMAC(in.Str("WorkIFMac")),
			VLAN:      atoi(in.Str("VLANID")),
			MTU:       atoi(in.Str("MTU")),
			Uptime:    seconds(in, "UpTime"),
			Uptime6:   seconds(in, "UpTimeV6"),
			RxBytes:   ptrU(in, "RxBytes"),
			TxBytes:   ptrU(in, "TxBytes"),
			RxPackets: ptrU(in, "RxPackets"),
			TxPackets: ptrU(in, "TxPackets"),
		}
		if e := in.Str("ConnError"); e != "" && e != "ERROR_NONE" {
			c.Error = e
		}
		c.Connected = strings.EqualFold(c.Status, "Connected")
		for i := 1; i <= 3; i++ {
			if a := firstNonEmpty(ipSet(in.Str(fmt.Sprintf("Gua%d", i)))...); a != "" {
				c.IPv6 = a
				break
			}
		}
		if p := firstNonEmpty(ipSet(in.Str("Pd"))...); p != "" {
			if l := in.Str("PdLen"); l != "" && l != "0" {
				p += "/" + l
			}
			c.IPv6Prefix = p
		}
		w.Connections = append(w.Connections, c)
	}
	return w, nil
}

// apBand maps an AP instance (DEV.WIFI.APn) to its radio band using the
// WLANViewName / OBJ_WLANSETTING_ID relation when present.
type apIndex struct {
	byAlias map[string]Instance // SSID5 -> AP instance
	byID    map[string]Instance // DEV.WIFI.AP5 -> AP instance
	band    map[string]string   // DEV.WIFI.RD1 -> 2.4GHz
}

func newAPIndex(rs ...*Response) apIndex {
	ix := apIndex{byAlias: map[string]Instance{}, byID: map[string]Instance{}, band: map[string]string{}}
	for _, r := range rs {
		if r == nil {
			continue
		}
		for _, rd := range r.Object("OBJ_WLANSETTING_ID") {
			if b := rd.Str("Band"); b != "" {
				ix.band[rd.Str("_InstID")] = b
			}
		}
		for _, ap := range r.Object("OBJ_WLANAP_ID") {
			id := ap.Str("_InstID")
			merged := ix.byID[id]
			if merged == nil {
				merged = Instance{}
				ix.byID[id] = merged
			}
			for k, v := range ap {
				if _, ok := merged[k]; !ok || merged[k] == "" {
					merged[k] = v
				}
			}
			if a := ap.Str("Alias"); a != "" {
				ix.byAlias[a] = merged
			}
		}
	}
	return ix
}

// bandOf returns the band of an AP. Without radio data it falls back to
// the F86xx layout: SSID1-4 on 2.4 GHz, SSID5-8 on 5 GHz.
func (ix apIndex) bandOf(ap Instance, alias string) string {
	if ap != nil {
		if b := ix.band[ap.Str("WLANViewName")]; b != "" {
			return b
		}
	}
	n := atoi(strings.TrimPrefix(strings.ToUpper(alias), "SSID"))
	switch {
	case n >= 1 && n <= 4:
		return "2.4GHz"
	case n >= 5 && n <= 8:
		return "5GHz"
	}
	return ""
}

// parseRate parses a link rate into Mbit/s. Values with a unit are
// honored; bare numbers above 20000 are taken as kbit/s.
func parseRate(s string) *float64 {
	v, ok := leadingFloat(s)
	if !ok {
		return nil
	}
	ls := strings.ToLower(s)
	switch {
	case strings.Contains(ls, "kbps") || strings.Contains(ls, "kbit"):
		v /= 1000
	case strings.Contains(ls, "gbps") || strings.Contains(ls, "gbit"):
		v *= 1000
	case strings.Contains(ls, "mbps") || strings.Contains(ls, "mbit"):
	default:
		if v > 20000 {
			v /= 1000
		}
	}
	return &v
}

// ParseHosts maps accessdev_landevs_lua.lua (wired) and
// wlan_client_stat_lua.lua (WiFi). Either may be nil. wlanStatus (from
// wlan_wlanstatus_lua.lua) is optional and only improves band mapping.
func ParseHosts(lan, wlan, wlanStatus *Response) []Host {
	out := []Host{}
	seen := map[string]int{}
	add := func(h Host) {
		if h.MAC == "" {
			return
		}
		if i, ok := seen[h.MAC]; ok {
			if h.Interface == "wlan" { // WiFi detail wins
				prev := out[i]
				if h.IP == "" {
					h.IP = prev.IP
				}
				if h.HostName == "" {
					h.HostName = prev.HostName
				}
				if h.IPv6 == "" {
					h.IPv6 = prev.IPv6
				}
				out[i] = h
			}
			return
		}
		seen[h.MAC] = len(out)
		out = append(out, h)
	}
	ix := newAPIndex(wlan, wlanStatus)
	for _, in := range wlan.Object("OBJ_WLAN_AD_ID") {
		alias := in.Str("AliasName")
		ap := ix.byAlias[alias]
		h := Host{
			MAC:       normMAC(in.Str("MACAddress")),
			IP:        in.Str("IPAddress"),
			IPv6:      in.Str("IPV6Address"),
			HostName:  in.Str("HostName"),
			Interface: "wlan",
			Port:      alias,
			Band:      ix.bandOf(ap, alias),
			RSSI:      ptrI(in, "RSSI"),
			SNR:       ptrI(in, "SNR"),
			Noise:     ptrI(in, "NOISE"),
			TxRate:    parseRate(in.Str("TxRate")),
			RxRate:    parseRate(in.Str("RxRate")),
			Mode:      in.Str("CurrentMode"),
			Bandwidth: in.Str("BAND"),
			RxBytes:   ptrU(in, "RXBytes"),
			TxBytes:   ptrU(in, "TXBytes"),
		}
		if ap != nil {
			h.SSID = ap.Str("ESSID")
		}
		if v, ok := in.Int("LinkTime"); ok {
			d := time.Duration(v) * time.Second
			h.LinkTime = &d
		}
		add(h)
	}
	for _, in := range lan.Object("OBJ_ACCESSDEV_ID") {
		add(Host{
			MAC:       normMAC(in.Str("MACAddress")),
			IP:        in.Str("IPAddress"),
			IPv6:      in.Str("IPV6Address"),
			HostName:  in.Str("HostName"),
			Interface: "lan",
			Port:      in.Str("AliasName"),
			RxBytes:   ptrU(in, "InBytes"),
			TxBytes:   ptrU(in, "OutBytes"),
		})
	}
	return out
}

func security(beacon string) string {
	switch strings.ToLower(beacon) {
	case "none":
		return "open"
	case "basic":
		return "wep"
	case "wpa":
		return "wpa"
	case "11i":
		return "wpa2"
	case "wpaand11i":
		return "wpa/wpa2"
	case "wpa3":
		return "wpa3"
	case "11iandwpa3":
		return "wpa2/wpa3"
	}
	return beacon
}

// ParseWiFi maps wlan_wlanbasicadconf_lua.lua (radios), and optionally
// wlan_wlansssidconf_lua.lua (SSID settings) and wlan_wlanstatus_lua.lua
// (BSSID, channel in use, counters). Any argument may be nil.
func ParseWiFi(radios, ssids, status *Response) WiFi {
	w := WiFi{Radios: []Radio{}}
	byID := map[string]int{}
	addRadio := func(in Instance) {
		id := in.Str("_InstID")
		if _, ok := byID[id]; ok || id == "" {
			return
		}
		byID[id] = len(w.Radios)
		r := Radio{
			ID:          id,
			Band:        in.Str("Band"),
			Enabled:     in.Bool("RadioStatus"),
			Channel:     atoi(in.Str("Channel")),
			AutoChannel: in.Bool("AutoChannelEnabled"),
			Bandwidth:   in.Str("BandWidth"),
			Standard:    in.Str("Standard"),
			TxPower:     in.Str("TxPower"),
			Country:     in.Str("CountryCode"),
			SSIDs:       []SSID{},
		}
		w.Radios = append(w.Radios, r)
	}
	for _, r := range []*Response{radios, ssids, status} {
		for _, in := range r.Object("OBJ_WLANSETTING_ID") {
			addRadio(in)
		}
	}
	ix := newAPIndex(ssids, status)
	drv := map[string]Instance{}
	for _, in := range status.Object("OBJ_WLANCONFIGDRV_ID") {
		drv[in.Str("_InstID")] = in
	}
	ids := make([]string, 0, len(ix.byID))
	for id := range ix.byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { // DEV.WIFI.AP2 < DEV.WIFI.AP10
		a, b := ids[i], ids[j]
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})
	for _, id := range ids {
		ap := ix.byID[id]
		d := drv[id]
		if d == nil {
			d = Instance{}
		}
		s := SSID{
			ID:             id,
			Alias:          ap.Str("Alias"),
			Name:           ap.Str("ESSID"),
			Enabled:        ap.Bool("Enable"),
			Hidden:         ap.Bool("ESSIDHideEnable"),
			Security:       security(ap.Str("BeaconType")),
			BSSID:          normMAC(d.Str("Bssid")),
			ChannelInUse:   atoi(d.Str("ChannelInUsed")),
			BandwidthInUse: d.Str("BandWidthInUsed"),
			MaxClients:     atoi(ap.Str("MaxUserNum")),
			RxBytes:        ptrU(d, "TotalBytesReceived"),
			TxBytes:        ptrU(d, "TotalBytesSent"),
			RxPackets:      ptrU(d, "TotalPacketsReceived"),
			TxPackets:      ptrU(d, "TotalPacketsSent"),
		}
		rd := firstNonEmpty(ap.Str("WLANViewName"), d.Str("WLANViewName"))
		i, ok := byID[rd]
		if !ok {
			band := ix.bandOf(nil, s.Alias)
			for j, r := range w.Radios {
				if r.Band == band {
					i, ok = j, true
					break
				}
			}
		}
		if !ok {
			addRadio(Instance{"_InstID": firstNonEmpty(rd, "unknown"), "Band": ix.bandOf(nil, s.Alias)})
			i = len(w.Radios) - 1
		}
		w.Radios[i].SSIDs = append(w.Radios[i].SSIDs, s)
	}
	return w
}

// ParsePON maps optical_info_lua.lua. The parameter names are matched
// loosely (RxPower/RXPower, Temp/Temperature, Volt/Voltage,
// Current/BiasCurrent) because this endpoint could not be verified.
func ParsePON(r *Response) (PON, error) {
	pick := func(in Instance, names ...string) *float64 {
		for k, v := range in {
			for _, n := range names {
				if strings.EqualFold(k, n) {
					if f, ok := leadingFloat(v); ok {
						return &f
					}
				}
			}
		}
		return nil
	}
	for _, name := range r.ObjectNames() {
		for _, in := range r.Objects[name] {
			p := PON{
				RxPower:     pick(in, "RxPower", "RxOpticalPower", "RxPwr"),
				TxPower:     pick(in, "TxPower", "TxOpticalPower", "TxPwr"),
				Temperature: pick(in, "Temp", "Temperature", "TransceiverTemperature"),
				Voltage:     pick(in, "Volt", "Voltage", "SupplyVoltage", "SupplyVottage"),
				BiasCurrent: pick(in, "Current", "BiasCurrent"),
				Raw:         in,
			}
			if p.RxPower != nil || p.TxPower != nil || p.Temperature != nil {
				return p, nil
			}
		}
	}
	return PON{}, fmt.Errorf("%w: no optical parameters", ErrNotAvailable)
}

// ParseTraffic maps status_lan_info_lua.lua (ports), and optionally
// wan_internetstatus_lua.lua and wlan_wlanstatus_lua.lua. Any may be nil.
func ParseTraffic(lanInfo, wan, wlanStatus *Response) Traffic {
	t := Traffic{Ports: []Port{}, WAN: []Counters{}, SSIDs: []Counters{}}
	for i, in := range lanInfo.Object("OBJ_PON_PORT_BASIC_STATUS_ID") {
		u := func(k string) uint64 { v, _ := in.Uint(k); return v }
		t.Ports = append(t.Ports, Port{
			Name:       "LAN" + strconv.Itoa(i+1), // as the web UI names them
			ID:         in.Str("_InstID"),
			Up:         in.Str("Status") == "0",
			SpeedMbps:  atoi(in.Str("Speed")),
			Duplex:     in.Str("Duplex"),
			RxBytes:    u("InBytes"),
			TxBytes:    u("OutBytes"),
			RxPackets:  u("InPkts"),
			TxPackets:  u("OutPkts"),
			RxErrors:   u("InError"),
			TxErrors:   u("OutError"),
			RxDiscards: u("InDiscard"),
			TxDiscards: u("OutDiscard"),
		})
	}
	if wan != nil {
		if ws, err := ParseWANStatus(wan); err == nil {
			conns := ws.Connections
			if inet, ok := ws.Internet(); ok { // internet connection first
				conns = []WANConnection{inet}
				for _, c := range ws.Connections {
					if c.ID != inet.ID {
						conns = append(conns, c)
					}
				}
			}
			for _, c := range conns {
				if c.RxBytes == nil && c.TxBytes == nil {
					continue
				}
				t.WAN = append(t.WAN, Counters{Name: c.Name, RxBytes: c.RxBytes, TxBytes: c.TxBytes, RxPackets: c.RxPackets, TxPackets: c.TxPackets})
			}
		}
	}
	if wlanStatus != nil {
		w := ParseWiFi(nil, nil, wlanStatus)
		for _, r := range w.Radios {
			for _, s := range r.SSIDs {
				if !s.Enabled {
					continue
				}
				t.SSIDs = append(t.SSIDs, Counters{Name: s.Alias, RxBytes: s.RxBytes, TxBytes: s.TxBytes, RxPackets: s.RxPackets, TxPackets: s.TxPackets})
			}
		}
	}
	return t
}

// ParseTopology maps topo_lua.lua (JSON, controllerData).
func ParseTopology(r *Response) (Topology, error) {
	in := r.First("controllerData")
	if len(in) == 0 {
		return Topology{}, fmt.Errorf("%w: no controllerData", ErrUnexpected)
	}
	t := Topology{
		DeviceName:      in.Str("DeviceName"),
		MAC:             normMAC(in.Str("MacAddr")),
		IP:              in.Str("IpAddr"),
		SoftwareVersion: in.Str("SoftwareVer"),
		Raw:             in,
	}
	if n, ok := in.Int("accdevCount"); ok {
		i := int(n)
		t.Clients = &i
	}
	return t, nil
}

// ------------------------------------------------------------ typed reads

// DeviceInfo reads the device identity.
func (c *Client) DeviceInfo(ctx context.Context) (DeviceInfo, error) {
	rs, err := c.fetch(ctx, viewStatusMgr, tagStatusMgr)
	if err != nil {
		return DeviceInfo{}, err
	}
	return ParseDeviceInfo(rs[0])
}

// System reads CPU/memory usage and uptime.
func (c *Client) System(ctx context.Context) (System, error) {
	rs, err := c.fetch(ctx, viewStatusMgr, tagStatusMgr)
	if err != nil {
		return System{}, err
	}
	return ParseSystem(rs[0])
}

// WANStatus reads the WAN connections.
func (c *Client) WANStatus(ctx context.Context) (WANStatus, error) {
	rs, err := c.fetch(ctx, viewWAN, tagWAN)
	if err != nil {
		return WANStatus{}, err
	}
	return ParseWANStatus(rs[0])
}

// Clients reads the wired and WiFi clients currently known to the device
// (the device lists only connected clients).
func (c *Client) Clients(ctx context.Context) ([]Host, error) {
	rs, err := c.fetch(ctx, viewLocalNet, tagLANDevs, tagWLANClients, tagWLANStatus)
	if err != nil {
		return nil, err
	}
	return ParseHosts(rs[0], rs[1], rs[2]), nil
}

// WiFi reads the WLAN radios and SSIDs.
func (c *Client) WiFi(ctx context.Context) (WiFi, error) {
	cfg, err := c.fetch(ctx, viewWLANBasic, tagWLANRadio, tagWLANSSID)
	if err != nil {
		return WiFi{}, err
	}
	var status *Response
	if st, err := c.fetch(ctx, viewLocalNet, tagWLANStatus); err == nil {
		status = st[0]
	} else if ctx.Err() != nil {
		return WiFi{}, ctx.Err()
	}
	return ParseWiFi(cfg[0], cfg[1], status), nil
}

// PON reads the optical module state. On the F8648P this page is only
// visible to admin accounts; with others it returns ErrNotAvailable
// (remembered for the life of the client, so it costs requests once).
func (c *Client) PON(ctx context.Context) (PON, error) {
	rs, err := c.fetch(ctx, viewPONOptical, tagPONOptical)
	if err != nil {
		return PON{}, err
	}
	return ParsePON(rs[0])
}

// Traffic reads the Ethernet port, WAN and SSID counters.
func (c *Client) Traffic(ctx context.Context) (Traffic, error) {
	lan, err := c.fetch(ctx, viewLocalNet, tagLANInfo, tagWLANStatus)
	if err != nil {
		return Traffic{}, err
	}
	var wan *Response
	if w, err := c.fetch(ctx, viewWAN, tagWAN); err == nil {
		wan = w[0]
	} else if ctx.Err() != nil || !errors.Is(err, ErrNotAvailable) {
		return Traffic{}, err
	}
	return ParseTraffic(lan[0], wan, lan[1]), nil
}

// Topology reads the topology controller summary (LAN MAC, IP).
func (c *Client) Topology(ctx context.Context) (Topology, error) {
	rs, err := c.fetch(ctx, viewTopology, tagTopology)
	if err != nil {
		return Topology{}, err
	}
	return ParseTopology(rs[0])
}

// SessionTimeout reads the web session idle timeout.
func (c *Client) SessionTimeout(ctx context.Context) (time.Duration, error) {
	rs, err := c.fetch(ctx, viewAccount, tagLoginTimeout)
	if err != nil {
		return 0, err
	}
	v, ok := rs[0].First("OBJ_USERIF_ID").Int("Timeout")
	if !ok {
		return 0, fmt.Errorf("%w: no Timeout", ErrUnexpected)
	}
	return time.Duration(v) * time.Minute, nil
}
