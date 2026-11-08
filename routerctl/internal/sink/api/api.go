// Package api is the local HTTP API of the daemon (docs/http-api.md):
// REST for reads, actions and inventory edits, and Server-Sent
// Events for the event stream. It is a transport like MQTT: commands go
// through service/commands, payloads are the service/view views and the
// events.Event envelope. The Node-RED nodes (node-red/) are its client.
//
// The app mounts Handler on the shared HTTP server, under /api/v1.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
	"github.com/eduardpaul/routerctl/routerkit"
)

// Version is the API version (the /api/v<Version> prefix).
const Version = 1

// Prefix is where the API is mounted.
const Prefix = "/api/v1"

// Options configure the API.
type Options struct {
	// Token: required as "Authorization: Bearer <token>" when set
	Token string
	// Policy decides which actions POST .../actions/{action} may run
	Policy commands.Policy
	// Buffer: events kept for Last-Event-ID replay (default 500)
	Buffer int
	// Heartbeat of the event stream (default 15s)
	Heartbeat time.Duration
	// Version of routerctl (GET /info)
	Version string
}

// Sink is the API transport.
type Sink struct {
	opts   Options
	ctl    poller.Control
	ctx    context.Context
	cancel context.CancelFunc

	mu   sync.Mutex
	ring []events.Event
	subs map[*subscriber]struct{}
}

// New builds the API.
func New(o Options) *Sink {
	if o.Buffer <= 0 {
		o.Buffer = 500
	}
	if o.Heartbeat <= 0 {
		o.Heartbeat = 15 * time.Second
	}
	s := &Sink{opts: o, subs: map[*subscriber]struct{}{}}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

// Name implements poller.Sink.
func (s *Sink) Name() string { return "api" }

// Start implements poller.Sink.
func (s *Sink) Start(_ context.Context, ctl poller.Control) error {
	s.mu.Lock()
	s.ctl = ctl
	s.mu.Unlock()
	return nil
}

// Close implements poller.Sink: ends every event stream.
func (s *Sink) Close(context.Context) error {
	s.cancel()
	return nil
}

func (s *Sink) control() poller.Control {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctl
}

// ---- event fan-out ----

type frame struct {
	event, id string
	data      []byte
}

type subscriber struct {
	sel   events.Selector
	state bool
	ch    chan frame
	// dropped: the client was too slow; its stream ends (it reconnects
	// with Last-Event-ID and replays from the ring)
	dropped bool
}

func (s *Sink) send(sub *subscriber, f frame) {
	if sub.dropped {
		return
	}
	select {
	case sub.ch <- f:
	default:
		sub.dropped = true
		close(sub.ch)
		slog.Warn("api: event stream too slow, closing it")
	}
}

// Event implements poller.Sink.
func (s *Sink) Event(e events.Event) {
	data, err := marshal(e)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring = append(s.ring, e)
	if over := len(s.ring) - s.opts.Buffer; over > 0 {
		s.ring = append(s.ring[:0:0], s.ring[over:]...)
	}
	for sub := range s.subs {
		if sub.sel.Match(e) {
			s.send(sub, frame{event: string(e.Type), id: e.ID, data: data})
		}
	}
}

// StateFrame is the "state" frame of the event stream (?state=1).
type StateFrame struct {
	Device   string     `json:"device"`
	Online   bool       `json:"online"`
	LastPoll string     `json:"last_poll,omitempty"`
	State    view.State `json:"state"`
}

// State implements poller.Sink.
func (s *Sink) State(st poller.DeviceState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data []byte
	for sub := range s.subs {
		if !sub.state || (len(sub.sel.Devices) > 0 && !has(sub.sel.Devices, st.Name)) {
			continue
		}
		if data == nil {
			f := StateFrame{Device: st.Name, Online: st.Online, State: view.BuildState(st)}
			if !st.LastPoll.IsZero() {
				f.LastPoll = st.LastPoll.UTC().Format(time.RFC3339)
			}
			var err error
			if data, err = marshal(f); err != nil {
				return
			}
		}
		s.send(sub, frame{event: "state", data: data})
	}
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---- HTTP ----

// Handler serves the API (paths start with Prefix).
func (s *Sink) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+Prefix+"/info", s.info)
	mux.HandleFunc("GET "+Prefix+"/devices", s.devices)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}", s.device)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}/clients", s.clients)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}/nodes", s.nodes)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}/actions", s.actions)
	mux.HandleFunc("POST "+Prefix+"/devices/{dev}/actions/{action}", s.do)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}/sources", s.sources)
	mux.HandleFunc("GET "+Prefix+"/devices/{dev}/sources/{name}", s.source)
	mux.HandleFunc("GET "+Prefix+"/inventory", s.inventoryList)
	mux.HandleFunc("GET "+Prefix+"/inventory/{mac}", s.inventoryGet)
	mux.HandleFunc("PATCH "+Prefix+"/inventory/{mac}", s.inventoryPatch)
	mux.HandleFunc("DELETE "+Prefix+"/inventory/{mac}", s.inventoryDelete)
	mux.HandleFunc("GET "+Prefix+"/events", s.events)
	mux.HandleFunc(Prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, commands.CodeNotFound, "no such endpoint")
	})
	return s.auth(mux)
}

func (s *Sink) auth(next http.Handler) http.Handler {
	want := []byte("Bearer " + s.opts.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.opts.Token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="routerctl"`)
			writeError(w, http.StatusUnauthorized, commands.CodeAuth, "missing or wrong bearer token")
			return
		}
		if s.control() == nil {
			writeError(w, http.StatusServiceUnavailable, commands.CodeNotReady, "the daemon is starting")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func marshal(v any) ([]byte, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(b.String(), "\n")), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, commands.CodeDevice, "cannot encode the response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// ErrorBody is every error response.
type ErrorBody struct {
	Error commands.Error `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code commands.Code, msg string) {
	writeJSON(w, status, ErrorBody{Error: commands.Error{Code: code, Message: msg}})
}

// StatusOf maps an error code to an HTTP status.
func StatusOf(code commands.Code) int {
	switch code {
	case "":
		return http.StatusOK
	case commands.CodeInvalidArgs:
		return http.StatusBadRequest
	case commands.CodeAuth:
		return http.StatusBadGateway // the device refused our login
	case commands.CodeForbidden:
		return http.StatusForbidden
	case commands.CodeUnknownDevice, commands.CodeNotFound, commands.CodeUnsupported:
		return http.StatusNotFound
	case commands.CodeNotReady:
		return http.StatusConflict
	case commands.CodeBusy:
		return http.StatusServiceUnavailable
	case commands.CodeTimeout, commands.CodeUnreachable:
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func writeErr(w http.ResponseWriter, err error) {
	e := commands.NewError(err)
	writeError(w, StatusOf(e.Code), e.Code, e.Message)
}

// state returns the state of {dev} or writes a 404.
func (s *Sink) state(w http.ResponseWriter, r *http.Request) (poller.DeviceState, bool) {
	dev := r.PathValue("dev")
	st, ok := s.control().State(dev)
	if !ok {
		writeError(w, http.StatusNotFound, commands.CodeUnknownDevice, fmt.Sprintf("unknown device %q", dev))
	}
	return st, ok
}

func ready(st poller.DeviceState) bool {
	return len(st.Capabilities.Actions) > 0 || len(st.Capabilities.Parts) > 0
}

// Info is GET /info.
type Info struct {
	API        int           `json:"api"`
	Version    string        `json:"version"`
	Devices    []string      `json:"devices"`
	EventTypes []events.Type `json:"event_types"`
	Policy     PolicyView    `json:"policy"`
	Auth       bool          `json:"auth"`
	Time       time.Time     `json:"time"`
}

// PolicyView is the command policy as JSON.
type PolicyView struct {
	MaxDanger string   `json:"max_danger"`
	Allow     []string `json:"allow,omitempty"`
	Deny      []string `json:"deny,omitempty"`
}

func (s *Sink) info(w http.ResponseWriter, r *http.Request) {
	p := s.opts.Policy
	writeJSON(w, http.StatusOK, Info{
		API: Version, Version: s.opts.Version, Devices: s.control().Devices(), EventTypes: events.AllTypes,
		Policy: PolicyView{MaxDanger: p.MaxDanger.String(), Allow: p.Allow, Deny: p.Deny},
		Auth:   s.opts.Token != "", Time: time.Now().UTC(),
	})
}

func (s *Sink) devices(w http.ResponseWriter, r *http.Request) {
	ctl := s.control()
	out := []view.Device{}
	for _, d := range ctl.Devices() {
		if st, ok := ctl.State(d); ok {
			out = append(out, view.BuildDevice(st, false))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Sink) device(w http.ResponseWriter, r *http.Request) {
	if st, ok := s.state(w, r); ok {
		writeJSON(w, http.StatusOK, view.BuildDetail(st))
	}
}

// boolQuery reads ?name=true|false (nil when absent).
func boolQuery(r *http.Request, name string) (*bool, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	b, err := routerkit.ParseOnOff(v)
	if err != nil {
		return nil, fmt.Errorf("%s=%q is not a boolean", name, v)
	}
	return &b, nil
}

func (s *Sink) clients(w http.ResponseWriter, r *http.Request) {
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	online, err1 := boolQuery(r, "online")
	known, err2 := boolQuery(r, "known")
	if err := errors.Join(err1, err2); err != nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, err.Error())
		return
	}
	out := []view.Client{}
	for _, c := range view.Clients(st) {
		if (online == nil || c.Present == *online) && (known == nil || c.Known == *known) {
			out = append(out, c)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Sink) nodes(w http.ResponseWriter, r *http.Request) {
	if st, ok := s.state(w, r); ok {
		out := st.Snapshot.Nodes
		if out == nil {
			out = []view.Node{}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Sink) actions(w http.ResponseWriter, r *http.Request) {
	st, ok := s.state(w, r)
	if !ok {
		return
	}
	if !ready(st) {
		writeError(w, http.StatusConflict, commands.CodeNotReady, st.Name+": not connected yet")
		return
	}
	writeJSON(w, http.StatusOK, s.opts.Policy.Actions(st.Capabilities.Actions))
}

// maxBody bounds request bodies.
const maxBody = 64 << 10

func (s *Sink) do(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, commands.CodeInvalidArgs, "request body too large")
		return
	}
	res := commands.Run(r.Context(), commands.FromControl(s.control()), s.opts.Policy, commands.Request{
		ID:      r.Header.Get("X-Request-Id"),
		Device:  r.PathValue("dev"),
		Action:  r.PathValue("action"),
		Payload: body,
		Source:  "api",
	})
	status := http.StatusOK
	if res.Error != nil {
		status = StatusOf(res.Error.Code)
	}
	writeJSON(w, status, res)
}

func (s *Sink) sources(w http.ResponseWriter, r *http.Request) {
	if st, ok := s.state(w, r); ok {
		out := st.Capabilities.Sources
		if out == nil {
			out = []routerkit.SourceSpec{}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// SourceBody is GET .../sources/{name}.
type SourceBody struct {
	Device string    `json:"device"`
	Source string    `json:"source"`
	Time   time.Time `json:"time"`
	Data   any       `json:"data"`
}

func (s *Sink) source(w http.ResponseWriter, r *http.Request) {
	dev, name := r.PathValue("dev"), r.PathValue("name")
	data, err := s.control().Get(r.Context(), dev, name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SourceBody{Device: dev, Source: name, Time: time.Now().UTC(), Data: data})
}

func (s *Sink) inventoryList(w http.ResponseWriter, r *http.Request) {
	known, err1 := boolQuery(r, "known")
	online, err2 := boolQuery(r, "online")
	if err := errors.Join(err1, err2); err != nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, err.Error())
		return
	}
	dev := r.URL.Query().Get("device")
	out := []inventory.Entry{}
	for _, e := range s.control().Inventory().List() {
		if (known == nil || e.Known == *known) && (online == nil || e.Online == *online) && (dev == "" || e.Router == dev) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	writeJSON(w, http.StatusOK, out)
}

func (s *Sink) entry(w http.ResponseWriter, mac string) {
	e, ok := s.control().Inventory().Get(mac)
	if !ok {
		writeError(w, http.StatusNotFound, commands.CodeNotFound, fmt.Sprintf("%s is not in the inventory", mac))
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Sink) inventoryGet(w http.ResponseWriter, r *http.Request) {
	if mac, ok := macParam(w, r); ok {
		s.entry(w, mac)
	}
}

// InventoryPatch is the body of PATCH /inventory/{mac}.
type InventoryPatch struct {
	Label *string `json:"label"`
	Known *bool   `json:"known"`
}

func inventoryErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		writeError(w, http.StatusNotFound, commands.CodeNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, commands.CodeDevice, err.Error())
	}
}

// macParam reads {mac} or writes a 400.
func macParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	mac := routerkit.NormalizeMAC(r.PathValue("mac"))
	if mac == "" {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, fmt.Sprintf("%q is not a MAC address", r.PathValue("mac")))
	}
	return mac, mac != ""
}

func (s *Sink) inventoryPatch(w http.ResponseWriter, r *http.Request) {
	mac, ok := macParam(w, r)
	if !ok {
		return
	}
	var p InventoryPatch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, "body: "+err.Error())
		return
	}
	if p.Label == nil && p.Known == nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, `body needs "label" and/or "known"`)
		return
	}
	inv := s.control().Inventory()
	if p.Label != nil {
		if err := inv.Label(mac, *p.Label); err != nil {
			inventoryErr(w, err)
			return
		}
	}
	if p.Known != nil {
		if err := inv.SetKnown(mac, *p.Known); err != nil {
			inventoryErr(w, err)
			return
		}
	}
	s.entry(w, mac)
}

func (s *Sink) inventoryDelete(w http.ResponseWriter, r *http.Request) {
	mac, ok := macParam(w, r)
	if !ok {
		return
	}
	if err := s.control().Inventory().Forget(mac); err != nil {
		inventoryErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// list reads a repeatable, comma-separated query parameter.
func list(r *http.Request, name string) []string {
	var out []string
	for _, v := range r.URL.Query()[name] {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Hello is the first frame of every event stream.
type Hello struct {
	API      int      `json:"api"`
	Version  string   `json:"version"`
	Devices  []string `json:"devices"`
	Replayed int      `json:"replayed"`
	// Resumed: the Last-Event-ID was found (false: events may be missing)
	Resumed bool `json:"resumed"`
}

func (s *Sink) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, commands.CodeDevice, "streaming unsupported")
		return
	}
	unknown, err := boolQuery(r, "unknown")
	wantState, err2 := boolQuery(r, "state")
	if err := errors.Join(err, err2); err != nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, err.Error())
		return
	}
	sel := events.NewSelector(list(r, "types"), list(r, "devices"), list(r, "macs"), unknown != nil && *unknown)
	if err := sel.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, commands.CodeInvalidArgs, err.Error())
		return
	}
	last := r.Header.Get("Last-Event-ID")
	if last == "" {
		last = r.URL.Query().Get("last_event_id")
	}
	sub := &subscriber{sel: sel, state: wantState != nil && *wantState, ch: make(chan frame, 256)}

	// Register and collect the replay under one lock: no gap, no dupes
	s.mu.Lock()
	var replay []events.Event
	resumed := false
	if last != "" {
		for i, e := range s.ring {
			if e.ID == last {
				replay, resumed = append(replay, s.ring[i+1:]...), true
				break
			}
		}
	}
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
	}()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(f frame) bool {
		var b strings.Builder
		if f.id != "" {
			b.WriteString("id: " + f.id + "\n")
		}
		b.WriteString("event: " + f.event + "\n")
		b.WriteString("data: ")
		b.Write(f.data)
		b.WriteString("\n\n")
		if _, err := io.WriteString(w, b.String()); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	matched := replay[:0:0]
	for _, e := range replay {
		if sel.Match(e) {
			matched = append(matched, e)
		}
	}
	hello, _ := marshal(Hello{API: Version, Version: s.opts.Version, Devices: s.control().Devices(), Replayed: len(matched), Resumed: resumed})
	if _, err := io.WriteString(w, "retry: 3000\n\n"); err != nil {
		return
	}
	if !write(frame{event: "hello", data: hello}) {
		return
	}
	for _, e := range matched {
		data, _ := marshal(e)
		if !write(frame{event: string(e.Type), id: e.ID, data: data}) {
			return
		}
	}
	tick := time.NewTicker(s.opts.Heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-tick.C:
			if _, err := io.WriteString(w, ": ping "+strconv.FormatInt(time.Now().Unix(), 10)+"\n\n"); err != nil {
				return
			}
			fl.Flush()
		case f, ok := <-sub.ch:
			if !ok || !write(f) {
				return
			}
		}
	}
}
