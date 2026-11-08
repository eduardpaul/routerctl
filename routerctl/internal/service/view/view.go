// Package view holds the JSON views of the daemon state that every
// transport publishes (MQTT topics, the HTTP API, stdout snapshots, and so
// Node-RED). They are a public payload contract: the golden schema in
// testdata/views.schema.golden.json makes changes deliberate.
package view

import (
	"sort"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerkit"
)

// State is the device state summary: MQTT <base>/<dev>/state, HTTP
// GET /api/v1/devices/{dev} "state".
type State struct {
	Online            bool                    `json:"online"`
	LastPoll          string                  `json:"last_poll,omitempty"`
	WANConnected      *bool                   `json:"wan_connected,omitempty"`
	WANIP             string                  `json:"wan_ip,omitempty"`
	PublicIP          string                  `json:"public_ip,omitempty"`
	CPUUsage          *float64                `json:"cpu_usage,omitempty"`
	RAMUsage          *float64                `json:"ram_usage,omitempty"`
	Temperature       map[string]float64      `json:"temperature,omitempty"`
	Uptime            *int64                  `json:"uptime,omitempty"`
	ClientsOnline     *int                    `json:"clients_online,omitempty"`
	ClientsTotal      *int                    `json:"clients_total,omitempty"`
	RxRate            *float64                `json:"rx_rate,omitempty"`
	TxRate            *float64                `json:"tx_rate,omitempty"`
	RxBytes           *uint64                 `json:"rx_bytes,omitempty"`
	TxBytes           *uint64                 `json:"tx_bytes,omitempty"`
	Traffic           map[string]trafficValue `json:"traffic,omitempty"`
	FirmwareCurrent   string                  `json:"firmware_current,omitempty"`
	FirmwareAvailable string                  `json:"firmware_available,omitempty"`
	FirmwareUpdate    *bool                   `json:"firmware_update,omitempty"`
}

type trafficValue struct {
	RxRate  *float64 `json:"rx_rate,omitempty"`
	TxRate  *float64 `json:"tx_rate,omitempty"`
	RxBytes *uint64  `json:"rx_bytes,omitempty"`
	TxBytes *uint64  `json:"tx_bytes,omitempty"`
}

// WANInterface picks the uplink interface of the traffic list.
func WANInterface(ifs []routerkit.Interface) *routerkit.Interface {
	for i := range ifs {
		if strings.EqualFold(ifs[i].Name, "wan") || strings.EqualFold(ifs[i].Name, "internet") {
			return &ifs[i]
		}
	}
	if len(ifs) > 0 {
		return &ifs[0]
	}
	return nil
}

// BuildState renders the state summary.
func BuildState(st poller.DeviceState) State {
	s := st.Snapshot
	p := State{Online: st.Online}
	if !st.LastPoll.IsZero() {
		p.LastPoll = st.LastPoll.UTC().Format(time.RFC3339)
	}
	if s.WAN != nil {
		p.WANConnected = s.WAN.Connected
		p.WANIP = s.WAN.IP
		p.PublicIP = s.WAN.PublicIP
	}
	if s.System != nil {
		p.CPUUsage, p.RAMUsage, p.Uptime = s.System.CPUUsage, s.System.RAMUsage, s.System.Uptime
		if len(s.System.Temperatures) > 0 {
			p.Temperature = s.System.Temperatures
		}
	}
	if s.Clients != nil {
		on := 0
		for _, c := range s.Clients {
			if c.Online {
				on++
			}
		}
		p.ClientsOnline, p.ClientsTotal = routerkit.Ptr(on), routerkit.Ptr(len(s.Clients))
	}
	if w := WANInterface(s.Traffic); w != nil {
		p.RxRate, p.TxRate, p.RxBytes, p.TxBytes = w.RxRate, w.TxRate, w.RxBytes, w.TxBytes
	}
	if len(s.Traffic) > 0 {
		p.Traffic = map[string]trafficValue{}
		for _, it := range s.Traffic {
			p.Traffic[it.Name] = trafficValue{it.RxRate, it.TxRate, it.RxBytes, it.TxBytes}
		}
	}
	if s.Firmware != nil {
		p.FirmwareCurrent, p.FirmwareAvailable, p.FirmwareUpdate = s.Firmware.Current, s.Firmware.Available, s.Firmware.UpdateAvailable
	}
	return p
}

// Client is one client merged with the inventory (label, known, first
// seen) and the debounced presence: MQTT <base>/<dev>/client/<macid>,
// HTTP GET /api/v1/devices/{dev}/clients.
type Client struct {
	MAC        string   `json:"mac"`
	Name       string   `json:"name"`
	Label      string   `json:"label,omitempty"`
	IP         string   `json:"ip,omitempty"`
	Vendor     string   `json:"vendor,omitempty"`
	Connection string   `json:"connection,omitempty"`
	Node       string   `json:"node,omitempty"`
	RSSI       *int     `json:"rssi,omitempty"`
	RxRate     *float64 `json:"rx_rate,omitempty"`
	TxRate     *float64 `json:"tx_rate,omitempty"`
	Online     bool     `json:"online"`
	Present    bool     `json:"present"`
	Known      bool     `json:"known"`
	FirstSeen  string   `json:"first_seen,omitempty"`
}

// BuildClient renders one client.
func BuildClient(st poller.DeviceState, c routerkit.Client) Client {
	p := Client{MAC: c.MAC, Name: c.Name, IP: c.IP, Vendor: c.Vendor, Connection: c.Connection, Node: c.Node, RSSI: c.RSSI, RxRate: c.RxRate, TxRate: c.TxRate, Online: c.Online, Present: c.Online}
	if pres, ok := st.Presence[c.MAC]; ok {
		p.Present = pres
	}
	if e, ok := st.Inventory[c.MAC]; ok {
		p.Name, p.Label, p.Known = e.DisplayName(), e.Label, e.Known
		if !e.FirstSeen.IsZero() {
			p.FirstSeen = e.FirstSeen.UTC().Format(time.RFC3339)
		}
		if p.IP == "" {
			p.IP = e.IP
		}
	}
	if p.Name == "" {
		p.Name = c.MAC
	}
	return p
}

// Clients renders every client of the latest snapshot, sorted by MAC.
func Clients(st poller.DeviceState) []Client {
	out := make([]Client, 0, len(st.Snapshot.Clients))
	for _, c := range st.Snapshot.Clients {
		out = append(out, BuildClient(st, c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}

// Node is one mesh node (MQTT <base>/<dev>/node/<macid>).
type Node = routerkit.Node

// Device is the summary of one configured device.
type Device struct {
	Name         string                  `json:"name"`
	Driver       string                  `json:"driver"`
	Online       bool                    `json:"online"`
	LastPoll     string                  `json:"last_poll,omitempty"`
	LastError    string                  `json:"last_error,omitempty"`
	Identity     routerkit.Identity      `json:"identity"`
	Capabilities *routerkit.Capabilities `json:"capabilities,omitempty"`
	Polls        uint64                  `json:"polls"`
	PollFailures uint64                  `json:"poll_failures"`
}

// BuildDevice renders a device; withCaps includes its capabilities.
func BuildDevice(st poller.DeviceState, withCaps bool) Device {
	d := Device{Name: st.Name, Driver: st.Driver, Online: st.Online, LastError: st.LastError, Identity: st.Identity, Polls: st.Polls, PollFailures: st.PollFailures}
	if !st.LastPoll.IsZero() {
		d.LastPoll = st.LastPoll.UTC().Format(time.RFC3339)
	}
	if withCaps {
		c := st.Capabilities
		d.Capabilities = &c
	}
	return d
}

// DeviceDetail is a device with its state summary.
type DeviceDetail struct {
	Device
	State State  `json:"state"`
	Nodes []Node `json:"nodes,omitempty"`
}

// BuildDetail renders a device with its state.
func BuildDetail(st poller.DeviceState) DeviceDetail {
	return DeviceDetail{Device: BuildDevice(st, true), State: BuildState(st), Nodes: st.Snapshot.Nodes}
}
