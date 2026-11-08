// Package metrics serves the Prometheus text exposition format on
// /metrics (written by hand, no client library) and a health check on
// /healthz.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerkit"
)

// Sink is the metrics sink and HTTP server.
type Sink struct {
	perClient bool
	// StaleAfter: /healthz fails when a device has not been polled
	// successfully for this long (default 5m)
	StaleAfter time.Duration
	// Now is the clock (tests)
	Now func() time.Time

	mu      sync.Mutex
	states  map[string]poller.DeviceState
	events  map[[2]string]uint64 // (device, type) -> count
	started time.Time
	lastOK  map[string]time.Time
}

// New builds the sink.
func New(perClient bool) *Sink {
	return &Sink{
		perClient:  perClient,
		StaleAfter: 5 * time.Minute,
		Now:        time.Now,
		states:     map[string]poller.DeviceState{},
		events:     map[[2]string]uint64{},
		lastOK:     map[string]time.Time{},
	}
}

// Name implements poller.Sink.
func (s *Sink) Name() string { return "metrics" }

// Handler returns both endpoints (/metrics, /healthz).
func (s *Sink) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", s.MetricsHandler())
	mux.Handle("/healthz", s.HealthHandler())
	return mux
}

// MetricsHandler serves the Prometheus exposition.
func (s *Sink) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		s.Write(w)
	})
}

// HealthHandler serves the health check (503 when a device is stale).
func (s *Sink) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, detail := s.Health()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(detail)
	})
}

// Start implements poller.Sink. The sink serves nothing itself: the app
// mounts Handler on the shared HTTP server.
func (s *Sink) Start(context.Context, poller.Control) error {
	s.mu.Lock()
	s.started = s.Now()
	s.mu.Unlock()
	return nil
}

// Close implements poller.Sink.
func (s *Sink) Close(context.Context) error { return nil }

// State implements poller.Sink.
func (s *Sink) State(st poller.DeviceState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[st.Name] = st
	if st.Online {
		s.lastOK[st.Name] = st.LastPoll
	}
}

// Event implements poller.Sink.
func (s *Sink) Event(e events.Event) {
	s.mu.Lock()
	s.events[[2]string{e.Device, string(e.Type)}]++
	s.mu.Unlock()
}

// Health reports whether every device was polled successfully recently.
func (s *Sink) Health() (bool, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	devs := map[string]any{}
	ok := true
	names := make([]string, 0, len(s.states))
	for n := range s.states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		st := s.states[n]
		last, seen := s.lastOK[n]
		healthy := st.Online || (seen && now.Sub(last) < s.StaleAfter)
		// A device that never answered is healthy during a start grace
		if !seen && now.Sub(s.started) < s.StaleAfter {
			healthy = true
		}
		d := map[string]any{"online": st.Online, "healthy": healthy}
		if seen {
			d["last_ok"] = last.UTC().Format(time.RFC3339)
		}
		if st.LastError != "" {
			d["error"] = st.LastError
		}
		devs[n] = d
		ok = ok && healthy
	}
	status := "ok"
	if !ok {
		status = "degraded"
	}
	return ok, map[string]any{"status": status, "devices": devs}
}

// ---- exposition ----

type family struct {
	name, help, typ string
	samples         []sample
}

type sample struct {
	labels [][2]string
	value  float64
}

type writer struct{ fams map[string]*family }

func (w *writer) add(name, typ, help string, value float64, labels ...string) {
	f, ok := w.fams[name]
	if !ok {
		f = &family{name: name, help: help, typ: typ}
		w.fams[name] = f
	}
	var ls [][2]string
	for i := 0; i+1 < len(labels); i += 2 {
		ls = append(ls, [2]string{labels[i], labels[i+1]})
	}
	f.samples = append(f.samples, sample{labels: ls, value: value})
}

func (w *writer) gauge(name, help string, v float64, labels ...string) {
	w.add(name, "gauge", help, v, labels...)
}

func (w *writer) counter(name, help string, v float64, labels ...string) {
	w.add(name, "counter", help, v, labels...)
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
var helpEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`)

func formatValue(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (w *writer) flush(out io.Writer) {
	names := make([]string, 0, len(w.fams))
	for n := range w.fams {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		f := w.fams[n]
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", f.name, helpEscaper.Replace(f.help), f.name, f.typ)
		for _, s := range f.samples {
			b.WriteString(f.name)
			if len(s.labels) > 0 {
				b.WriteByte('{')
				for i, l := range s.labels {
					if i > 0 {
						b.WriteByte(',')
					}
					fmt.Fprintf(&b, `%s="%s"`, l[0], labelEscaper.Replace(l[1]))
				}
				b.WriteByte('}')
			}
			b.WriteByte(' ')
			b.WriteString(formatValue(s.value))
			b.WriteByte('\n')
		}
	}
	_, _ = io.WriteString(out, b.String())
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Write renders the exposition.
func (s *Sink) Write(out io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := &writer{fams: map[string]*family{}}

	names := make([]string, 0, len(s.states))
	for n := range s.states {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		st := s.states[n]
		dev := []string{"device", n}
		with := func(kv ...string) []string { return append(append([]string{}, dev...), kv...) }
		id := st.Identity
		w.gauge("routerctl_up", "Whether the last poll reached the device.", b2f(st.Online), dev...)
		w.gauge("routerctl_info", "Device identity.", 1, with("driver", st.Driver, "vendor", id.Vendor, "model", id.Model, "firmware", id.Firmware)...)
		w.counter("routerctl_polls_total", "Polls run.", float64(st.Polls), dev...)
		w.counter("routerctl_poll_failures_total", "Polls that could not reach the device.", float64(st.PollFailures), dev...)
		parts := make([]string, 0, len(st.PartErrors))
		for p := range st.PartErrors {
			parts = append(parts, string(p))
		}
		sort.Strings(parts)
		for _, p := range parts {
			w.counter("routerctl_part_errors_total", "Per-part read errors.", float64(st.PartErrors[routerkit.Part(p)]), with("part", p)...)
		}
		w.gauge("routerctl_poll_duration_seconds", "Duration of the last poll.", st.PollDuration.Seconds(), dev...)
		if !st.LastPoll.IsZero() {
			w.gauge("routerctl_last_poll_timestamp_seconds", "Unix time of the last poll.", float64(st.LastPoll.Unix()), dev...)
		}

		snap := st.Snapshot
		if sys := snap.System; sys != nil {
			if sys.CPUUsage != nil {
				w.gauge("routerctl_cpu_usage_percent", "CPU usage.", *sys.CPUUsage, dev...)
			}
			if sys.RAMUsage != nil {
				w.gauge("routerctl_ram_usage_percent", "RAM usage.", *sys.RAMUsage, dev...)
			}
			if sys.Uptime != nil {
				w.gauge("routerctl_uptime_seconds", "Device uptime.", float64(*sys.Uptime), dev...)
			}
			sensors := make([]string, 0, len(sys.Temperatures))
			for k := range sys.Temperatures {
				sensors = append(sensors, k)
			}
			sort.Strings(sensors)
			for _, k := range sensors {
				w.gauge("routerctl_temperature_celsius", "Temperature by sensor.", sys.Temperatures[k], with("sensor", k)...)
			}
		}
		if wan := snap.WAN; wan != nil {
			if wan.Connected != nil {
				w.gauge("routerctl_wan_connected", "Whether the WAN is connected.", b2f(*wan.Connected), dev...)
			}
			if wan.IP != "" {
				w.gauge("routerctl_wan_info", "WAN addressing.", 1, with("ip", wan.IP, "protocol", wan.Protocol)...)
			}
			if wan.Uptime != nil {
				w.gauge("routerctl_wan_uptime_seconds", "WAN link uptime.", float64(*wan.Uptime), dev...)
			}
		}
		for _, it := range snap.Traffic {
			l := with("interface", it.Name)
			if it.RxBytes != nil {
				w.counter("routerctl_interface_receive_bytes_total", "Bytes received.", float64(*it.RxBytes), l...)
			}
			if it.TxBytes != nil {
				w.counter("routerctl_interface_transmit_bytes_total", "Bytes transmitted.", float64(*it.TxBytes), l...)
			}
			if it.RxRate != nil {
				w.gauge("routerctl_interface_receive_bytes_per_second", "Receive rate.", *it.RxRate, l...)
			}
			if it.TxRate != nil {
				w.gauge("routerctl_interface_transmit_bytes_per_second", "Transmit rate.", *it.TxRate, l...)
			}
		}
		if snap.Clients != nil {
			byConn := map[string]int{}
			online := 0
			for _, c := range snap.Clients {
				if c.Online {
					online++
					conn := c.Connection
					if conn == "" {
						conn = "unknown"
					}
					byConn[conn]++
				}
			}
			w.gauge("routerctl_clients", "Clients known to the device.", float64(len(snap.Clients)), dev...)
			w.gauge("routerctl_clients_online", "Online clients.", float64(online), dev...)
			conns := make([]string, 0, len(byConn))
			for k := range byConn {
				conns = append(conns, k)
			}
			sort.Strings(conns)
			for _, k := range conns {
				w.gauge("routerctl_clients_online_by_connection", "Online clients by connection type.", float64(byConn[k]), with("connection", k)...)
			}
			if s.perClient {
				cs := append([]routerkit.Client(nil), snap.Clients...)
				sort.Slice(cs, func(i, j int) bool { return cs[i].MAC < cs[j].MAC })
				for _, c := range cs {
					name := c.Name
					known := false
					if e, ok := st.Inventory[c.MAC]; ok {
						name = e.DisplayName()
						known = e.Known
					}
					present := c.Online
					if p, ok := st.Presence[c.MAC]; ok {
						present = p
					}
					l := with("mac", c.MAC, "name", name, "known", strconv.FormatBool(known))
					w.gauge("routerctl_client_online", "Debounced client presence.", b2f(present), l...)
					if c.RSSI != nil {
						w.gauge("routerctl_client_rssi_dbm", "Client signal strength.", float64(*c.RSSI), with("mac", c.MAC, "name", name)...)
					}
				}
			}
		}
		for _, nd := range snap.Nodes {
			l := with("mac", nd.MAC, "name", nd.Name, "model", nd.Model)
			w.gauge("routerctl_node_online", "Mesh node online state.", b2f(nd.Online), l...)
			if nd.Clients != nil {
				w.gauge("routerctl_node_clients", "Clients attached to a mesh node.", float64(*nd.Clients), with("mac", nd.MAC, "name", nd.Name)...)
			}
		}
		if fw := snap.Firmware; fw != nil && fw.UpdateAvailable != nil {
			w.gauge("routerctl_firmware_update_available", "Whether a firmware update is available.", b2f(*fw.UpdateAvailable), with("current", fw.Current, "available", fw.Available)...)
		}
	}

	keys := make([][2]string, 0, len(s.events))
	for k := range s.events {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		w.counter("routerctl_events_total", "Events emitted.", float64(s.events[k]), "device", k[0], "type", k[1])
	}
	w.flush(out)
}
