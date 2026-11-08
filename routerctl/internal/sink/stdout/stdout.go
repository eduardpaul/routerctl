// Package stdout is the JSON-lines sink: one event per line, optionally
// one line per poll result.
package stdout

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
)

// Sink writes JSON lines.
type Sink struct {
	mu        sync.Mutex
	w         io.Writer
	enc       *json.Encoder
	snapshots bool
	sel       events.Selector
}

// New writes to w (os.Stdout when nil); snapshots also prints poll
// results; sel picks the events (the zero Selector: every event).
func New(w io.Writer, snapshots bool, sel events.Selector) *Sink {
	if w == nil {
		w = os.Stdout
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &Sink{w: w, enc: enc, snapshots: snapshots, sel: sel}
}

// Name implements poller.Sink.
func (s *Sink) Name() string { return "stdout" }

// Start implements poller.Sink.
func (s *Sink) Start(context.Context, poller.Control) error { return nil }

// snapshotLine uses the same views as MQTT and the HTTP API.
type snapshotLine struct {
	Type    string        `json:"type"`
	Time    time.Time     `json:"time"`
	Device  string        `json:"device"`
	Online  bool          `json:"online"`
	Error   string        `json:"error,omitempty"`
	State   view.State    `json:"state"`
	Clients []view.Client `json:"clients,omitempty"`
	Nodes   []view.Node   `json:"nodes,omitempty"`
}

// State implements poller.Sink.
func (s *Sink) State(st poller.DeviceState) {
	if !s.snapshots {
		return
	}
	s.write(snapshotLine{Type: "snapshot", Time: st.LastPoll, Device: st.Name, Online: st.Online, Error: st.LastError, State: view.BuildState(st), Clients: view.Clients(st), Nodes: st.Snapshot.Nodes})
}

// Event implements poller.Sink.
func (s *Sink) Event(e events.Event) {
	if s.sel.Match(e) {
		s.write(e)
	}
}

func (s *Sink) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(v)
}

// Close implements poller.Sink.
func (s *Sink) Close(context.Context) error { return nil }
