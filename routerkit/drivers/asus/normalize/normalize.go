// Package normalize maps the canonical form of ASUS library sources (see
// asusrouter/PARITY.md "Canonical form": JSON-like trees keyed by the Python
// field names) into the vendor-neutral driver model.
//
// It depends only on the canonical shapes, not on the library's Go
// types, so it is testable on its own and robust to the port's internal
// naming.
package normalize

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

// Unknown is the Python UNKNOWN_MEMBER value of int enums.
const Unknown = -999

// ARConnectionStatus.CONNECTED
const statusConnected = 2

// ---- tree helpers ----

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func path(v any, keys ...string) any {
	for _, k := range keys {
		m := obj(v)
		if m == nil {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func num(v any) (float64, bool) {
	switch t := v.(type) {
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	case float64:
		return t, !math.IsNaN(t)
	case uint64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func numPtr(v any) *float64 {
	if f, ok := num(v); ok && f != Unknown {
		return &f
	}
	return nil
}

func intPtr(v any) *int {
	if f, ok := num(v); ok && f != Unknown {
		i := int(f)
		return &i
	}
	return nil
}

func boolOf(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

// ParseTime reads a Python isoformat() datetime.
func ParseTime(v any) *time.Time {
	s := str(v)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

func validIP(s string) string {
	if s == "" || s == "0.0.0.0" || s == "::" {
		return ""
	}
	return s
}

// ---- identity / firmware ----

// FirmwareString renders a canonical ARFirmware as "3.0.0.6.102_37544".
func FirmwareString(v any) string {
	m := obj(v)
	if m == nil {
		return ""
	}
	var major []string
	for _, x := range list(m["major"]) {
		major = append(major, str(x))
	}
	if len(major) == 0 {
		return ""
	}
	s := strings.Join(major, ".")
	if minor := str(m["minor"]); minor != "" {
		s += "." + minor
	}
	if build := str(m["build"]); build != "" {
		s += "_" + build
	}
	if rog, _ := boolOf(m["rog"]); rog {
		s += "_rog"
	}
	return s
}

// Identity maps ARDeviceSource (or the identity) content.
func Identity(dev any) routerkit.Identity {
	id := routerkit.Identity{Vendor: "ASUS"}
	m := obj(dev)
	if m == nil {
		return id
	}
	id.Model = str(m["model"])
	if id.Model == "" {
		id.Model = str(m["model_original"])
	}
	id.Firmware = FirmwareString(m["firmware"])
	id.Serial = str(m["serial"])
	id.MAC = routerkit.NormalizeMAC(str(m["mac"]))
	return id
}

// Firmware maps ARFirmwareSource content.
func Firmware(fw any) *routerkit.Firmware {
	m := obj(fw)
	if m == nil {
		return nil
	}
	out := &routerkit.Firmware{Current: FirmwareString(m["current"])}
	web := obj(m["web"])
	if web != nil {
		out.Available = FirmwareString(web["available"])
		if st, ok := boolOf(web["state"]); ok {
			out.UpdateAvailable = routerkit.Ptr(st || out.Available != "" && out.Available != out.Current)
		}
	}
	if out.UpdateAvailable == nil && out.Available != "" {
		out.UpdateAvailable = routerkit.Ptr(out.Available != out.Current)
	}
	return out
}

// ---- clients ----

func linkConnection(freq string) string {
	switch strings.ToLower(freq) {
	case "2g":
		return "2.4ghz"
	case "5g":
		return "5ghz"
	case "6g":
		return "6ghz"
	}
	return "wifi"
}

// Clients maps ARClientsSource content ({mac: ARClient}).
func Clients(v any) []routerkit.Client {
	m := obj(v)
	out := make([]routerkit.Client, 0, len(m))
	for key, raw := range m {
		c := obj(raw)
		if c == nil {
			continue
		}
		mac := routerkit.NormalizeMAC(str(c["mac"]))
		if mac == "" {
			mac = routerkit.NormalizeMAC(key)
		}
		if mac == "" {
			continue
		}
		cl := routerkit.Client{MAC: mac, Name: str(c["name"]), Vendor: str(c["vendor"])}
		cl.Online, _ = boolOf(c["online"])
		extra := map[string]any{}
		for _, k := range []string{"device_type", "internet_mode", "vendor_class", "os_type"} {
			if x, ok := c[k]; ok && x != nil && x != "unknown" {
				extra[k] = x
			}
		}
		cl.Node = routerkit.NormalizeMAC(str(c["bound_node"]))
		if conn := obj(c["connection"]); conn != nil {
			cl.IP = validIP(str(conn["ip"]))
			if n := routerkit.NormalizeMAC(str(conn["node"])); n != "" {
				cl.Node = n
			}
			switch str(conn["type"]) {
			case "wired":
				cl.Connection = "wired"
			case "wireless":
				cl.Connection = "wifi"
			}
			links := list(conn["links"])
			if len(links) > 0 {
				l := obj(links[0])
				if cl.Connection != "wired" {
					cl.Connection = linkConnection(str(l["frequency"]))
				}
				cl.RSSI = intPtr(l["rssi"])
				if r := numPtr(l["rx_speed"]); r != nil {
					cl.RxRate = routerkit.Ptr(math.Round(*r/1e6*100) / 100)
				}
				if t := numPtr(l["tx_speed"]); t != nil {
					cl.TxRate = routerkit.Ptr(math.Round(*t/1e6*100) / 100)
				}
			}
			cl.Since = ParseTime(conn["connected_since"])
			for _, k := range []string{"ssid", "guest", "sdn", "ip_method", "security", "internet_access", "mlo"} {
				if x, ok := conn[k]; ok && x != nil && x != "unknown" && x != "" {
					extra[k] = x
				}
			}
		}
		if len(extra) > 0 {
			cl.Extra = extra
		}
		out = append(out, cl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// ---- WAN ----

// WAN maps ARWanSource content.
func WAN(v any) *routerkit.WAN {
	m := obj(v)
	if m == nil {
		return nil
	}
	units := obj(m["units"])
	active := str(m["active_unit"])
	unit := obj(units[active])
	if unit == nil {
		// Fall back to the primary unit, else the lowest index
		keys := make([]string, 0, len(units))
		for k := range units {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if p, _ := boolOf(path(units[k], "primary")); p {
				unit = obj(units[k])
				break
			}
		}
		if unit == nil && len(keys) > 0 {
			unit = obj(units[keys[0]])
		}
	}
	w := &routerkit.WAN{Extra: map[string]any{}}
	if st, ok := num(m["internet_status"]); ok && st != Unknown {
		w.Connected = routerkit.Ptr(int(st) == statusConnected)
		w.Extra["internet_status"] = int(st)
	}
	if unit != nil {
		if w.Connected == nil {
			if st, ok := num(unit["status"]); ok && st != Unknown {
				w.Connected = routerkit.Ptr(int(st) == statusConnected)
			}
		}
		main := obj(unit["main"])
		w.IP = validIP(str(main["ip"]))
		w.Gateway = validIP(str(main["gateway"]))
		for _, d := range list(main["dns"]) {
			if s := validIP(str(d)); s != "" {
				w.DNS = append(w.DNS, s)
			}
		}
		if len(w.DNS) == 0 {
			for _, d := range list(unit["dns_servers"]) {
				if s := validIP(str(d)); s != "" {
					w.DNS = append(w.DNS, s)
				}
			}
		}
		if p := str(unit["protocol"]); p != "unknown" {
			w.Protocol = p
		}
		if rip := validIP(str(unit["real_ip"])); rip != "" {
			w.PublicIP = rip
		}
		if idx := str(unit["index"]); idx != "" {
			w.Extra["unit"] = idx
		}
		if l, ok := boolOf(unit["link"]); ok {
			w.Extra["link"] = l
		}
	}
	if dw := obj(m["dualwan"]); dw != nil {
		if st, ok := boolOf(dw["state"]); ok {
			w.Extra["dualwan"] = st
		}
	}
	if len(w.Extra) == 0 {
		w.Extra = nil
	}
	return w
}

// ---- system ----

// System maps ARSystemStatusSource ({node mac: {cpu: {usage}, ram:
// {usage}}}), ARTemperatureSource ({sensor: value}) and ARClockSource
// ({uptime}). routerMAC picks the router's own entry.
func System(status, temps, clock any, routerMAC string) *routerkit.System {
	s := &routerkit.System{}
	found := false
	if m := obj(status); m != nil {
		entry := obj(m[routerMAC])
		if entry == nil {
			// Look for a flat {cpu, ram} or the first node entry
			if _, ok := m["cpu"]; ok {
				entry = m
			} else {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				if len(keys) > 0 {
					entry = obj(m[keys[0]])
				}
			}
		}
		if entry != nil {
			s.CPUUsage = numPtr(path(entry, "cpu", "usage"))
			s.RAMUsage = numPtr(path(entry, "ram", "usage"))
			found = found || s.CPUUsage != nil || s.RAMUsage != nil
		}
	}
	if m := obj(temps); len(m) > 0 {
		s.Temperatures = map[string]float64{}
		for k, v := range m {
			if f, ok := num(v); ok && f != Unknown {
				s.Temperatures[tempName(k)] = math.Round(f*10) / 10
			}
		}
		found = found || len(s.Temperatures) > 0
	}
	if up := numPtr(path(clock, "uptime")); up != nil {
		s.Uptime = routerkit.Ptr(int64(*up))
		found = true
	}
	if !found {
		return nil
	}
	return s
}

func tempName(k string) string {
	switch strings.ToLower(k) {
	case "2g1", "2ghz", "2.4ghz":
		return "2.4ghz"
	case "5g1", "5ghz":
		return "5ghz"
	case "5g2":
		return "5ghz-2"
	case "6g1", "6ghz":
		return "6ghz"
	}
	return k
}

// ---- traffic ----

// Traffic maps ARTrafficSource content ({link: {rx, tx, rx_speed,
// tx_speed}}; counters in bytes, speeds in bits/s) to bytes and bytes/s.
func Traffic(v any) []routerkit.Interface {
	m := obj(v)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []routerkit.Interface
	for _, k := range keys {
		e := obj(m[k])
		if e == nil {
			continue
		}
		it := routerkit.Interface{Name: k}
		if f, ok := num(e["rx"]); ok && f >= 0 {
			it.RxBytes = routerkit.Ptr(uint64(f))
		}
		if f, ok := num(e["tx"]); ok && f >= 0 {
			it.TxBytes = routerkit.Ptr(uint64(f))
		}
		if f, ok := num(e["rx_speed"]); ok && f >= 0 {
			it.RxRate = routerkit.Ptr(math.Round(f/8*100) / 100)
		}
		if f, ok := num(e["tx_speed"]); ok && f >= 0 {
			it.TxRate = routerkit.Ptr(math.Round(f/8*100) / 100)
		}
		out = append(out, it)
	}
	return out
}

// ---- AiMesh ----

// Nodes maps ARAiMeshSource content ({nodes: {mac: node}}) and the
// per-node client counts of ARClientsCountSource ({mac: n}).
func Nodes(aimesh, counts any) []routerkit.Node {
	nodes := obj(path(aimesh, "nodes"))
	cnt := obj(counts)
	out := make([]routerkit.Node, 0, len(nodes))
	for key, raw := range nodes {
		n := obj(raw)
		if n == nil {
			continue
		}
		mac := routerkit.NormalizeMAC(str(n["mac"]))
		if mac == "" {
			mac = routerkit.NormalizeMAC(key)
		}
		if mac == "" {
			continue
		}
		nd := routerkit.Node{MAC: mac, Name: str(n["alias"]), Model: str(n["model"]), IP: validIP(str(n["ip"]))}
		if nd.Name == "" {
			nd.Name = str(n["model_display"])
		}
		nd.Online, _ = boolOf(n["online"])
		for k, v := range cnt {
			if routerkit.NormalizeMAC(k) == mac {
				nd.Clients = intPtr(v)
			}
		}
		out = append(out, nd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// CountByNode counts online clients per node MAC (the shape of
// ARClientsCountSource: {mac: n}).
func CountByNode(clients []routerkit.Client) map[string]any {
	out := map[string]any{}
	for _, c := range clients {
		if c.Online && c.Node != "" {
			n, _ := out[c.Node].(int64)
			out[c.Node] = n + 1
		}
	}
	return out
}
