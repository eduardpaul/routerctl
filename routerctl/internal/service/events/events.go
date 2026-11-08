// Package events turns successive snapshots of a device into events
// (docs/watch.md, "Events"). Diff is a pure function: the
// previous engine state, the new snapshot, a read-only view of the
// inventory and the current time in; the next state and the events out.
package events

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerkit"
)

// Type is an event type.
type Type string

// Event types.
const (
	DeviceNew       Type = "device_new"
	DeviceOnline    Type = "device_online"
	DeviceOffline   Type = "device_offline"
	DeviceIPChanged Type = "device_ip_changed"
	WANDown         Type = "wan_down"
	WANUp           Type = "wan_up"
	WANIPChanged    Type = "wan_ip_changed"
	FirmwareUpdate  Type = "firmware_update"
	NodeOffline     Type = "node_offline"
	NodeOnline      Type = "node_online"
	// ActionDone: an action ran (from any transport), successfully or not
	ActionDone Type = "action_done"
)

// AllTypes lists every event type.
var AllTypes = []Type{DeviceNew, DeviceOnline, DeviceOffline, DeviceIPChanged, WANDown, WANUp, WANIPChanged, FirmwareUpdate, NodeOffline, NodeOnline, ActionDone}

// ValidType reports whether s names an event type.
func ValidType(s string) bool {
	for _, t := range AllTypes {
		if string(t) == s {
			return true
		}
	}
	return false
}

// Event is one event: `{"id", "type", "time", "device"?, "summary",
// "data"}`, the envelope every transport (MQTT, HTTP API, notify,
// stdout) sends as is. Device is the configured router name. ID and
// Summary are set by the daemon when the event is emitted (Diff leaves
// them empty): ID is "<boot>-<seq>", unique across restarts and ordered
// within one run; Summary is Describe().
type Event struct {
	ID      string         `json:"id,omitempty"`
	Type    Type           `json:"type"`
	Time    time.Time      `json:"time"`
	Device  string         `json:"device,omitempty"`
	Summary string         `json:"summary,omitempty"`
	Data    map[string]any `json:"data"`
}

// str reads a string field of Data.
func (e Event) str(k string) string {
	if v, ok := e.Data[k]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// Describe is a one-line human description (the Summary field, and the
// default notification message).
func (e Event) Describe() string {
	who := e.str("name")
	if mac := e.str("mac"); mac != "" && who != mac {
		if who == "" {
			who = mac
		} else {
			who += " (" + mac + ")"
		}
	}
	on := ""
	if e.Device != "" {
		on = " on " + e.Device
	}
	switch e.Type {
	case DeviceNew:
		parts := []string{"New device" + on + ": " + who}
		if ip := e.str("ip"); ip != "" {
			parts = append(parts, "IP "+ip)
		}
		if v := e.str("vendor"); v != "" {
			parts = append(parts, "vendor "+v)
		}
		if c := e.str("connection"); c != "" {
			parts = append(parts, c)
		}
		return strings.Join(parts, ", ")
	case DeviceOnline:
		return who + " is online" + on
	case DeviceOffline:
		return who + " went offline" + on
	case DeviceIPChanged:
		return fmt.Sprintf("%s changed IP%s: %s -> %s", who, on, e.str("old_ip"), e.str("new_ip"))
	case WANDown:
		return "Internet is down" + on
	case WANUp:
		return "Internet is back up" + on
	case WANIPChanged:
		return fmt.Sprintf("Public IP changed%s: %s -> %s", on, e.str("old_ip"), e.str("new_ip"))
	case FirmwareUpdate:
		if a := e.str("available"); a != "" {
			return fmt.Sprintf("Firmware update available%s: %s -> %s", on, e.str("current"), a)
		}
		return "Firmware update available" + on
	case NodeOffline:
		return "Mesh node " + who + " went offline" + on
	case NodeOnline:
		return "Mesh node " + who + " is back online" + on
	case ActionDone:
		act := e.str("action")
		if ok, _ := e.Data["ok"].(bool); ok {
			if p, _ := e.Data["pending"].(bool); p {
				return "Action " + act + " started" + on
			}
			return "Action " + act + " done" + on
		}
		return "Action " + act + " failed" + on + ": " + e.str("error")
	}
	return string(e.Type) + on
}

// Config tunes the diff.
type Config struct {
	// OfflineGrace: how long a client must stay offline before
	// device_offline fires
	OfflineGrace time.Duration
	// OnlineDebounce: how long a client must stay online before
	// device_online fires
	OnlineDebounce time.Duration
	// Baseline: record unseen clients without device_new (first run)
	Baseline bool
}

// ClientState is the per-client presence state.
type ClientState struct {
	// Reported is the debounced state (what the events / MQTT say)
	Reported bool `json:"reported"`
	// Observed is the raw state of the last poll, Since when it changed
	Observed bool      `json:"observed"`
	Since    time.Time `json:"since"`
}

// State is the engine state of one router (persisted between runs).
type State struct {
	Clients           map[string]ClientState `json:"clients,omitempty"`
	WANConnected      *bool                  `json:"wan_connected,omitempty"`
	WANIP             string                 `json:"wan_ip,omitempty"`
	FirmwareUpdate    *bool                  `json:"firmware_update,omitempty"`
	FirmwareAvailable string                 `json:"firmware_available,omitempty"`
	Nodes             map[string]bool        `json:"nodes,omitempty"`
}

// Clone deep-copies the state.
func (s State) Clone() State {
	out := s
	out.Clients = make(map[string]ClientState, len(s.Clients))
	for k, v := range s.Clients {
		out.Clients[k] = v
	}
	out.Nodes = make(map[string]bool, len(s.Nodes))
	for k, v := range s.Nodes {
		out.Nodes[k] = v
	}
	if s.WANConnected != nil {
		out.WANConnected = routerkit.Ptr(*s.WANConnected)
	}
	if s.FirmwareUpdate != nil {
		out.FirmwareUpdate = routerkit.Ptr(*s.FirmwareUpdate)
	}
	return out
}

// Present reports the debounced presence of a client.
func (s State) Present(mac string) (bool, bool) {
	c, ok := s.Clients[mac]
	return c.Reported, ok
}

func clientData(c routerkit.Client, e inventory.Entry, inInv bool) map[string]any {
	name := c.Name
	if inInv {
		name = e.DisplayName()
		if e.Label == "" && c.Name != "" {
			name = c.Name
		}
	}
	if name == "" {
		name = c.MAC
	}
	d := map[string]any{"mac": c.MAC, "name": name, "known": inInv && e.Known}
	if c.IP != "" {
		d["ip"] = c.IP
	}
	if c.Vendor != "" {
		d["vendor"] = c.Vendor
	}
	if c.Connection != "" {
		d["connection"] = c.Connection
	}
	if c.Node != "" {
		d["node"] = c.Node
	}
	if inInv && e.Label != "" {
		d["label"] = e.Label
	}
	return d
}

// PublicIP is the public address of a WAN: WAN.PublicIP when the driver
// knows it (a router behind another NAT), else the WAN address.
func PublicIP(w *routerkit.WAN) string {
	if w == nil {
		return ""
	}
	if validIP(w.PublicIP) {
		return w.PublicIP
	}
	return w.IP
}

func validIP(ip string) bool {
	return ip != "" && ip != "0.0.0.0" && ip != "::"
}

// Diff compares a snapshot with the previous state. Parts missing from
// the snapshot (nil, or listed in snap.Errors) are left untouched.
func Diff(prev State, snap routerkit.Snapshot, inv inventory.Lookup, now time.Time, cfg Config, device string) (State, []Event) {
	next := prev.Clone()
	var out []Event
	emit := func(t Type, data map[string]any) {
		out = append(out, Event{Type: t, Time: now, Device: device, Data: data})
	}
	_, clientsFailed := snap.Errors[routerkit.PartClients]

	// Clients
	if snap.Clients != nil && !clientsFailed {
		seen := map[string]bool{}
		clients := make([]routerkit.Client, 0, len(snap.Clients))
		for _, c := range snap.Clients {
			if c.MAC = routerkit.NormalizeMAC(c.MAC); c.MAC != "" {
				clients = append(clients, c)
			}
		}
		sort.SliceStable(clients, func(i, j int) bool { return clients[i].MAC < clients[j].MAC })
		for _, c := range clients {
			mac := c.MAC
			if seen[mac] {
				continue
			}
			seen[mac] = true
			entry, inInv := inv.Get(mac)
			st, tracked := next.Clients[mac]

			if !inInv && !cfg.Baseline {
				emit(DeviceNew, clientData(c, entry, false))
			}
			if inInv && c.Online && validIP(c.IP) && validIP(entry.IP) && c.IP != entry.IP {
				d := clientData(c, entry, true)
				d["old_ip"], d["new_ip"] = entry.IP, c.IP
				delete(d, "ip")
				emit(DeviceIPChanged, d)
			}

			if !tracked {
				// First sight: adopt the state, no transition event
				next.Clients[mac] = ClientState{Reported: c.Online, Observed: c.Online, Since: now}
				continue
			}
			next.Clients[mac] = step(st, c.Online, now, cfg, func(online bool, since time.Time) {
				t := DeviceOffline
				if online {
					t = DeviceOnline
				}
				d := clientData(c, entry, inInv)
				d["since"] = since.UTC().Format(time.RFC3339)
				emit(t, d)
			})
		}
		// Clients that vanished from the list count as offline
		var gone []string
		for mac := range next.Clients {
			if !seen[mac] {
				gone = append(gone, mac)
			}
		}
		sort.Strings(gone)
		for _, mac := range gone {
			st := next.Clients[mac]
			entry, inInv := inv.Get(mac)
			c := routerkit.Client{MAC: mac, Name: entry.Name, IP: entry.IP, Vendor: entry.Vendor}
			next.Clients[mac] = step(st, false, now, cfg, func(_ bool, since time.Time) {
				d := clientData(c, entry, inInv)
				d["since"] = since.UTC().Format(time.RFC3339)
				emit(DeviceOffline, d)
			})
		}
	}

	// WAN
	if _, failed := snap.Errors[routerkit.PartWAN]; snap.WAN != nil && !failed {
		w := snap.WAN
		if w.Connected != nil {
			if next.WANConnected != nil && *next.WANConnected != *w.Connected {
				d := map[string]any{}
				if validIP(w.IP) {
					d["ip"] = w.IP
				}
				if *w.Connected {
					emit(WANUp, d)
				} else {
					emit(WANDown, d)
				}
			}
			next.WANConnected = routerkit.Ptr(*w.Connected)
		}
		connected := w.Connected == nil || *w.Connected
		if ip := PublicIP(w); validIP(ip) && connected {
			if next.WANIP != "" && next.WANIP != ip {
				emit(WANIPChanged, map[string]any{"old_ip": next.WANIP, "new_ip": ip})
			}
			next.WANIP = ip
		}
	}

	// Firmware
	if _, failed := snap.Errors[routerkit.PartFirmware]; snap.Firmware != nil && !failed && snap.Firmware.UpdateAvailable != nil {
		fw := snap.Firmware
		if *fw.UpdateAvailable {
			fresh := next.FirmwareUpdate == nil || !*next.FirmwareUpdate || next.FirmwareAvailable != fw.Available
			if fresh {
				d := map[string]any{"current": fw.Current}
				if fw.Available != "" {
					d["available"] = fw.Available
				}
				emit(FirmwareUpdate, d)
			}
		}
		next.FirmwareUpdate = routerkit.Ptr(*fw.UpdateAvailable)
		next.FirmwareAvailable = fw.Available
	}

	// Mesh nodes
	if _, failed := snap.Errors[routerkit.PartNodes]; snap.Nodes != nil && !failed {
		seen := map[string]bool{}
		nodes := append([]routerkit.Node(nil), snap.Nodes...)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].MAC < nodes[j].MAC })
		nodeData := func(n routerkit.Node) map[string]any {
			d := map[string]any{"mac": n.MAC, "name": n.Name}
			if n.Name == "" {
				d["name"] = n.MAC
			}
			if n.Model != "" {
				d["model"] = n.Model
			}
			if n.IP != "" {
				d["ip"] = n.IP
			}
			return d
		}
		for _, n := range nodes {
			mac := routerkit.NormalizeMAC(n.MAC)
			if mac == "" {
				mac = n.MAC
			}
			n.MAC = mac
			seen[mac] = true
			was, ok := next.Nodes[mac]
			if ok && was != n.Online {
				if n.Online {
					emit(NodeOnline, nodeData(n))
				} else {
					emit(NodeOffline, nodeData(n))
				}
			}
			next.Nodes[mac] = n.Online
		}
		var gone []string
		for mac, online := range next.Nodes {
			if !seen[mac] && online {
				gone = append(gone, mac)
			}
		}
		sort.Strings(gone)
		for _, mac := range gone {
			emit(NodeOffline, nodeData(routerkit.Node{MAC: mac}))
			next.Nodes[mac] = false
		}
	}
	return next, out
}

// step advances one client's debounce state with a new observation; fire
// is called when the reported state flips.
func step(st ClientState, online bool, now time.Time, cfg Config, fire func(online bool, since time.Time)) ClientState {
	if online != st.Observed {
		st.Observed = online
		st.Since = now
	}
	if st.Observed == st.Reported {
		return st
	}
	wait := cfg.OfflineGrace
	if st.Observed {
		wait = cfg.OnlineDebounce
	}
	if now.Sub(st.Since) >= wait {
		st.Reported = st.Observed
		fire(st.Reported, st.Since)
	}
	return st
}

// Selector picks events: every non-empty field must match. It is the one
// filter of every transport (notify targets, stdout, HTTP API event
// streams, Node-RED event nodes).
type Selector struct {
	Types   []Type   `json:"types,omitempty"`
	Devices []string `json:"devices,omitempty"`
	// MACs: client / node events about one of these MACs
	MACs []string `json:"macs,omitempty"`
	// UnknownOnly: client events about a client that is not known
	// (approved or labelled); other events never match
	UnknownOnly bool `json:"unknown_only,omitempty"`
}

// NewSelector builds a selector from names (blank entries are ignored;
// MACs are normalized).
func NewSelector(types, devices, macs []string, unknownOnly bool) Selector {
	var s Selector
	for _, t := range types {
		if t = strings.TrimSpace(t); t != "" {
			s.Types = append(s.Types, Type(t))
		}
	}
	for _, d := range devices {
		if d = strings.TrimSpace(d); d != "" {
			s.Devices = append(s.Devices, d)
		}
	}
	for _, m := range macs {
		if n := routerkit.NormalizeMAC(m); n != "" {
			s.MACs = append(s.MACs, n)
		} else if m = strings.TrimSpace(m); m != "" {
			s.MACs = append(s.MACs, strings.ToLower(m))
		}
	}
	s.UnknownOnly = unknownOnly
	return s
}

// Validate reports unknown event types.
func (s Selector) Validate() error {
	var bad []string
	for _, t := range s.Types {
		if !ValidType(string(t)) {
			bad = append(bad, string(t))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("unknown event type(s) %s", strings.Join(bad, ", "))
	}
	return nil
}

// Match reports whether the selector accepts e.
func (s Selector) Match(e Event) bool {
	if len(s.Types) > 0 && !contains(s.Types, e.Type) {
		return false
	}
	if len(s.Devices) > 0 && !contains(s.Devices, e.Device) {
		return false
	}
	if len(s.MACs) > 0 {
		mac := routerkit.NormalizeMAC(e.str("mac"))
		if mac == "" || !contains(s.MACs, mac) {
			return false
		}
	}
	if s.UnknownOnly {
		known, isClient := e.Data["known"].(bool)
		if !isClient || known {
			return false
		}
	}
	return true
}

func contains[T comparable](list []T, v T) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
