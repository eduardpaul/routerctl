// Package fake is a deterministic in-memory driver ("fake", see Factory),
// for tests and demos. Its state only changes through the exported
// setters (tests) or, with the "demo" option, on a fixed poll schedule.
package fake

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

// Marker is the home page marker Detect looks for.
const Marker = "<!-- routerctl-fake -->"

// Info is the static driver metadata.
var Info = routerkit.DriverInfo{
	Name:      "fake",
	Vendor:    "Fake",
	Help:      "deterministic in-memory device for tests and demos",
	Stability: routerkit.Stable,
	Contract:  routerkit.ContractVersion,
}

// OptionSpecs are the driver options.
var OptionSpecs = []routerkit.OptionSpec{
	{Name: "demo", Help: "change the state on a fixed poll schedule (also with host \"demo\")", Type: routerkit.ArgBool},
}

// Factory returns the "fake" driver factory.
func Factory() routerkit.Factory {
	return routerkit.Factory{
		Info:    Info,
		Options: OptionSpecs,
		New: func(cfg routerkit.Config) (routerkit.Driver, error) {
			o, err := routerkit.BindOptions(OptionSpecs, cfg.Options)
			if err != nil {
				return nil, fmt.Errorf("fake: %w", err)
			}
			f := New()
			f.demo = o.Bool("demo") || cfg.Host == "demo"
			if cfg.Host != "" {
				f.identity.Hostname = cfg.Host
			}
			return f, nil
		},
		Fingerprints: []routerkit.Fingerprint{{Match: func(ev *routerkit.Evidence) (routerkit.Match, bool) {
			if strings.Contains(ev.Home().Text(), Marker) {
				return routerkit.Match{Score: routerkit.ScoreDefinitive, Model: "FAKE-1000", Reason: "routerctl-fake marker"}, true
			}
			return routerkit.Match{}, false
		}}},
	}
}

// Call is one recorded Do.
type Call struct {
	Name string
	Args map[string]string
}

// Fake is the driver.
type Fake struct {
	routerkit.Base

	mu        sync.Mutex
	connected bool
	closed    bool
	demo      bool
	polls     int
	// FailPoll makes Poll fail (device unreachable) while set
	failPoll error
	partErr  map[routerkit.Part]string
	// latched makes Poll fail until the next Connect (like a latched
	// credentials refusal that only an explicit reconnect clears)
	latched  error
	connects int

	identity routerkit.Identity
	clients  []routerkit.Client
	wan      routerkit.WAN
	system   routerkit.System
	traffic  []routerkit.Interface
	nodes    []routerkit.Node
	firmware routerkit.Firmware
	led      bool
	calls    []Call

	unsupported   map[routerkit.Part]bool
	requested     map[routerkit.Part]int
	minInterval   time.Duration
	singleSession bool
}

var _ routerkit.Driver = (*Fake)(nil)

// New returns a fake with the default dataset.
func New() *Fake {
	return &Fake{
		identity: routerkit.Identity{
			Vendor:   "Fake",
			Model:    "FAKE-1000",
			Firmware: "1.0.0",
			Serial:   "FAKE0001",
			MAC:      "02:00:00:00:00:01",
			Hostname: "fake-router",
		},
		clients: []routerkit.Client{
			{MAC: "02:00:00:00:10:01", IP: "192.168.50.10", Name: "laptop", Vendor: "Acme", Online: true, Connection: "5ghz", Node: "02:00:00:00:00:01", RSSI: routerkit.Ptr(-48)},
			{MAC: "02:00:00:00:10:02", IP: "192.168.50.11", Name: "phone", Vendor: "Acme", Online: true, Connection: "2.4ghz", Node: "02:00:00:00:00:02", RSSI: routerkit.Ptr(-67)},
			{MAC: "02:00:00:00:10:03", IP: "192.168.50.12", Name: "nas", Vendor: "Synology", Online: true, Connection: "wired", Node: "02:00:00:00:00:01"},
		},
		wan: routerkit.WAN{
			Connected: routerkit.Ptr(true),
			IP:        "203.0.113.7",
			Gateway:   "203.0.113.1",
			DNS:       []string{"1.1.1.1", "9.9.9.9"},
			Protocol:  "dhcp",
			Uptime:    routerkit.Ptr(int64(86400)),
		},
		system: routerkit.System{
			CPUUsage:     routerkit.Ptr(12.5),
			RAMUsage:     routerkit.Ptr(41.0),
			Temperatures: map[string]float64{"cpu": 55.5, "2.4ghz": 44, "5ghz": 47},
			Uptime:       routerkit.Ptr(int64(172800)),
		},
		traffic: []routerkit.Interface{
			{Name: "wan", RxBytes: routerkit.Ptr(uint64(1_000_000)), TxBytes: routerkit.Ptr(uint64(250_000)), RxRate: routerkit.Ptr(12500.0), TxRate: routerkit.Ptr(2500.0)},
			{Name: "lan", RxBytes: routerkit.Ptr(uint64(400_000)), TxBytes: routerkit.Ptr(uint64(900_000)), RxRate: routerkit.Ptr(1000.0), TxRate: routerkit.Ptr(8000.0)},
		},
		nodes: []routerkit.Node{
			{MAC: "02:00:00:00:00:01", Name: "Living room", Model: "FAKE-1000", IP: "192.168.50.1", Online: true, Clients: routerkit.Ptr(2)},
			{MAC: "02:00:00:00:00:02", Name: "Office", Model: "FAKE-500", IP: "192.168.50.2", Online: true, Clients: routerkit.Ptr(1)},
		},
		firmware: routerkit.Firmware{Current: "1.0.0", UpdateAvailable: routerkit.Ptr(false)},
		led:      true,
		partErr:  map[routerkit.Part]string{},

		unsupported: map[routerkit.Part]bool{},
		requested:   map[routerkit.Part]int{},
	}
}

// Info implements routerkit.Driver.
func (f *Fake) Info() routerkit.DriverInfo { return Info }

// Capabilities implements routerkit.Driver: every part (minus the ones
// SetUnsupported removed), the actions and the sources.
func (f *Fake) Capabilities(context.Context) (routerkit.Capabilities, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return routerkit.Capabilities{}, routerkit.ErrClosed
	}
	return f.capsLocked(), nil
}

func (f *Fake) capsLocked() routerkit.Capabilities {
	parts := routerkit.AllSupported()
	parts[routerkit.PartClients] = routerkit.PartialSupport("rx_rate", "tx_rate", "since")
	parts[routerkit.PartWAN] = routerkit.PartialSupport("ipv6", "public_ip")
	for p := range f.unsupported {
		parts[p] = routerkit.PartSupport{Level: routerkit.Unsupported}
	}
	return routerkit.Capabilities{
		Parts:         parts,
		Features:      []routerkit.Feature{routerkit.FeatureMesh},
		Actions:       f.actions(),
		Sources:       sources,
		MinInterval:   f.minInterval,
		SingleSession: f.singleSession,
	}
}

// Connect implements routerkit.Driver.
func (f *Fake) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failPoll != nil {
		return f.failPoll
	}
	f.connected = true
	f.closed = false
	f.latched = nil
	f.connects++
	return nil
}

// Latch makes every Poll fail with err until Connect is called again.
func (f *Fake) Latch(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latched = err
}

// Connects counts successful Connect calls.
func (f *Fake) Connects() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connects
}

// Close implements routerkit.Driver.
func (f *Fake) Close(context.Context) error {
	f.mu.Lock()
	f.connected = false
	f.closed = true
	f.mu.Unlock()
	return nil
}

// Identity implements routerkit.Driver.
func (f *Fake) Identity(context.Context) (routerkit.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return routerkit.Identity{}, routerkit.ErrClosed
	}
	return f.identity, nil
}

// Poll implements routerkit.Driver.
func (f *Fake) Poll(_ context.Context, req routerkit.PollRequest) (routerkit.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return routerkit.Snapshot{}, routerkit.ErrClosed
	}
	if f.failPoll != nil {
		return routerkit.Snapshot{}, f.failPoll
	}
	if f.latched != nil {
		return routerkit.Snapshot{}, f.latched
	}
	f.polls++
	if f.demo {
		f.demoStep()
	}
	// Counters grow deterministically with every poll
	for i := range f.traffic {
		it := &f.traffic[i]
		if it.RxBytes != nil && it.RxRate != nil {
			*it.RxBytes += uint64(*it.RxRate)
		}
		if it.TxBytes != nil && it.TxRate != nil {
			*it.TxBytes += uint64(*it.TxRate)
		}
	}
	snap := routerkit.Snapshot{Time: time.Now()}
	failed, tried, first := 0, 0, ""
	for _, p := range req.Parts {
		f.requested[p]++
		if f.unsupported[p] {
			continue
		}
		tried++
		if msg, ok := f.partErr[p]; ok {
			snap.SetError(p, errors.New(msg))
			if failed++; failed == 1 {
				first = string(p) + ": " + msg
			}
			continue
		}
		switch p {
		case routerkit.PartClients:
			snap.Clients = cloneClients(f.clients)
		case routerkit.PartWAN:
			w := f.wan
			w.DNS = append([]string(nil), f.wan.DNS...)
			if f.wan.Connected != nil {
				w.Connected = routerkit.Ptr(*f.wan.Connected)
			}
			snap.WAN = &w
		case routerkit.PartSystem:
			s := f.system
			s.Temperatures = map[string]float64{}
			for k, v := range f.system.Temperatures {
				s.Temperatures[k] = v
			}
			snap.System = &s
		case routerkit.PartTraffic:
			for _, it := range f.traffic {
				c := routerkit.Interface{Name: it.Name}
				if it.RxBytes != nil {
					c.RxBytes = routerkit.Ptr(*it.RxBytes)
				}
				if it.TxBytes != nil {
					c.TxBytes = routerkit.Ptr(*it.TxBytes)
				}
				if it.RxRate != nil {
					c.RxRate = routerkit.Ptr(*it.RxRate)
				}
				if it.TxRate != nil {
					c.TxRate = routerkit.Ptr(*it.TxRate)
				}
				snap.Traffic = append(snap.Traffic, c)
			}
		case routerkit.PartNodes:
			snap.Nodes = append([]routerkit.Node(nil), f.nodes...)
		case routerkit.PartFirmware:
			fw := f.firmware
			if f.firmware.UpdateAvailable != nil {
				fw.UpdateAvailable = routerkit.Ptr(*f.firmware.UpdateAvailable)
			}
			snap.Firmware = &fw
		}
	}
	if tried > 0 && failed == tried {
		return routerkit.Snapshot{}, fmt.Errorf("fake: every part failed (%s)", first)
	}
	return snap, nil
}

// demoStep changes the state on a fixed schedule (every few polls) so a
// demo `watch` shows events without a real device.
func (f *Fake) demoStep() {
	switch f.polls % 12 {
	case 4:
		f.setOnline("02:00:00:00:10:02", false)
	case 8:
		f.setOnline("02:00:00:00:10:02", true)
	case 6:
		f.clients = append(f.clients, routerkit.Client{MAC: fmt.Sprintf("02:00:00:00:20:%02x", f.polls%256), IP: "192.168.50.99", Name: "visitor", Online: true, Connection: "2.4ghz"})
	case 10:
		f.wan.Connected = routerkit.Ptr(false)
	case 11:
		f.wan.Connected = routerkit.Ptr(true)
	}
}

func cloneClients(in []routerkit.Client) []routerkit.Client {
	out := make([]routerkit.Client, len(in))
	copy(out, in)
	return out
}

func (f *Fake) setOnline(mac string, online bool) {
	for i := range f.clients {
		if f.clients[i].MAC == mac {
			f.clients[i].Online = online
		}
	}
}

var sources = []routerkit.SourceSpec{
	{Name: "fake_clients", Help: "the raw client table"},
	{Name: "fake_led", Help: "LED state"},
	{Name: "fake_status", Help: "WAN and system state"},
}

// Get implements routerkit.Driver.
func (f *Fake) Get(_ context.Context, name string) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, routerkit.ErrClosed
	}
	switch name {
	case "fake_clients":
		out := map[string]any{}
		for _, c := range f.clients {
			out[c.MAC] = map[string]any{"ip": c.IP, "name": c.Name, "online": c.Online}
		}
		return out, nil
	case "fake_led":
		return map[string]any{"state": f.led}, nil
	case "fake_status":
		return map[string]any{"wan_ip": f.wan.IP, "wan_connected": f.wan.Connected != nil && *f.wan.Connected, "cpu": *f.system.CPUUsage}, nil
	}
	return nil, routerkit.Classify(fmt.Errorf("unknown source %q", name), routerkit.ErrNotFound)
}

func (f *Fake) actions() []routerkit.ActionSpec {
	return []routerkit.ActionSpec{
		routerkit.Std(routerkit.ActBlockClient),
		routerkit.Std(routerkit.ActLED),
		routerkit.Std(routerkit.ActReboot),
		{Name: "restart-service", Help: "restart a service", Danger: routerkit.Disruptive, Args: []routerkit.Arg{{Name: "name", Type: routerkit.ArgString, Required: true}}},
		routerkit.Std(routerkit.ActSpeedtest),
		routerkit.Std(routerkit.ActWiFi, routerkit.WithChoices("band", "2g1", "5g1")),
	}
}

// Do implements routerkit.Driver. Every action is accepted; speedtest
// answers at once.
func (f *Fake) Do(_ context.Context, inv routerkit.Invocation) (routerkit.Result, error) {
	_, args, err := routerkit.BindInvocation(f.actions(), inv)
	if err != nil {
		return routerkit.Result{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return routerkit.Result{}, routerkit.ErrClosed
	}
	f.calls = append(f.calls, Call{Name: inv.Name, Args: args.Strings()})
	switch inv.Name {
	case routerkit.ActLED:
		f.led = args.Bool("state")
	case routerkit.ActSpeedtest:
		return routerkit.Result{Accepted: true, Data: map[string]any{"download": 940.5, "upload": 41.2, "latency": 3.1}}, nil
	case routerkit.ActReboot:
		return routerkit.Result{Accepted: true, Data: map[string]any{"ok": true}, ExpectDisconnect: true}, nil
	}
	return routerkit.Result{Accepted: true, Data: map[string]any{"ok": true}}, nil
}

// ---- test controls ----

// Calls returns the recorded Do calls.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// Polls returns how many successful polls ran.
func (f *Fake) Polls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

// SetUnreachable makes Connect / Poll fail with err (nil restores).
func (f *Fake) SetUnreachable(err error) {
	f.mu.Lock()
	f.failPoll = err
	f.mu.Unlock()
}

// SetPartError makes a part fail (empty msg clears it).
func (f *Fake) SetPartError(p routerkit.Part, msg string) {
	f.mu.Lock()
	if msg == "" {
		delete(f.partErr, p)
	} else {
		f.partErr[p] = msg
	}
	f.mu.Unlock()
}

// SetClients replaces the client table.
func (f *Fake) SetClients(cs []routerkit.Client) {
	f.mu.Lock()
	f.clients = cloneClients(cs)
	f.mu.Unlock()
}

// Clients returns the client table.
func (f *Fake) Clients() []routerkit.Client {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneClients(f.clients)
}

// UpsertClient adds or replaces a client.
func (f *Fake) UpsertClient(c routerkit.Client) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.clients {
		if f.clients[i].MAC == c.MAC {
			f.clients[i] = c
			return
		}
	}
	f.clients = append(f.clients, c)
	sort.Slice(f.clients, func(i, j int) bool { return f.clients[i].MAC < f.clients[j].MAC })
}

// SetClientOnline changes a client's online state.
func (f *Fake) SetClientOnline(mac string, online bool) {
	f.mu.Lock()
	f.setOnline(mac, online)
	f.mu.Unlock()
}

// SetWAN changes the WAN state.
func (f *Fake) SetWAN(connected bool, ip string) {
	f.mu.Lock()
	f.wan.Connected = routerkit.Ptr(connected)
	f.wan.IP = ip
	f.mu.Unlock()
}

// SetFirmwareUpdate changes the firmware update state.
func (f *Fake) SetFirmwareUpdate(available string) {
	f.mu.Lock()
	f.firmware.Available = available
	f.firmware.UpdateAvailable = routerkit.Ptr(available != "")
	f.mu.Unlock()
}

// SetNodeOnline changes a node's online state.
func (f *Fake) SetNodeOnline(mac string, online bool) {
	f.mu.Lock()
	for i := range f.nodes {
		if f.nodes[i].MAC == mac {
			f.nodes[i].Online = online
		}
	}
	f.mu.Unlock()
}

// Requested counts how often each part was requested from Poll.
func (f *Fake) Requested() map[routerkit.Part]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[routerkit.Part]int, len(f.requested))
	for p, n := range f.requested {
		out[p] = n
	}
	return out
}

// SetUnsupported marks parts unsupported (Capabilities and Poll).
func (f *Fake) SetUnsupported(parts ...routerkit.Part) {
	f.mu.Lock()
	for _, p := range parts {
		f.unsupported[p] = true
	}
	f.mu.Unlock()
}

// SetSchedulingHints sets Capabilities.MinInterval and SingleSession.
func (f *Fake) SetSchedulingHints(minInterval time.Duration, singleSession bool) {
	f.mu.Lock()
	f.minInterval, f.singleSession = minInterval, singleSession
	f.mu.Unlock()
}

// ErrUnreachable is a convenient error for SetUnreachable (it is a
// routerkit.ErrUnreachable).
var ErrUnreachable = fmt.Errorf("fake: %w", routerkit.ErrUnreachable)
