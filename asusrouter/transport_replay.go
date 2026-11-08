package asusrouter

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
)

// ReplayTransport answers queries from a recorded session, like
// `_install_replay` in parity/py_harness.py.
type ReplayTransport struct {
	Session   *replay.Session
	mu        sync.Mutex
	connected bool
}

// NewReplayTransport loads a session file.
func NewReplayTransport(path string) (*ReplayTransport, error) {
	s, err := replay.Load(path)
	if err != nil {
		return nil, err
	}
	return &ReplayTransport{Session: s}, nil
}

// Connect always succeeds.
func (t *ReplayTransport) Connect(context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = true
	return true, nil
}

// Disconnect always succeeds.
func (t *ReplayTransport) Disconnect(context.Context) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	return true, nil
}

// Connected reports the fake connection state.
func (t *ReplayTransport) Connected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// ResetAuth drops the fake connection.
func (t *ReplayTransport) ResetAuth() {}

// Close is a no-op.
func (t *ReplayTransport) Close() error { return nil }

// Query answers from the session; unknown queries are a 404.
func (t *ReplayTransport) Query(_ context.Context, ep endpoint.AREndpoint, payload *string, _ consts.RequestType) (Response, error) {
	rec := t.Session.Answer(string(ep), payload)
	if rec.Status == 404 {
		return Response{}, arerror.New404Error(fmt.Sprintf("Endpoint %s not found", ep))
	}
	return Response{Status: rec.Status, Content: rec.Content}, nil
}

// RecordingTransport wraps a transport and records every query, like
// `_install_recorder` in parity/py_harness.py (same session format).
type RecordingTransport struct {
	Transport
	mu      sync.Mutex
	records []replay.Record
}

// NewRecordingTransport wraps a transport.
func NewRecordingTransport(t Transport) *RecordingTransport {
	return &RecordingTransport{Transport: t}
}

// Query forwards and records the reply (404s are recorded as status 404).
func (t *RecordingTransport) Query(ctx context.Context, ep endpoint.AREndpoint, payload *string, rt consts.RequestType) (Response, error) {
	resp, err := t.Transport.Query(ctx, ep, payload, rt)
	var p *string
	if payload != nil {
		v := *payload
		p = &v
	}
	switch {
	case err == nil:
		t.add(replay.Record{Endpoint: string(ep), Payload: p, Status: resp.Status, Content: resp.Content})
	case errors.Is(err, arerror.Err404):
		t.add(replay.Record{Endpoint: string(ep), Payload: p, Status: 404})
	}
	return resp, err
}

func (t *RecordingTransport) add(r replay.Record) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, r)
}

// Records returns what was recorded so far.
func (t *RecordingTransport) Records() []replay.Record {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]replay.Record(nil), t.records...)
}

// Save writes the recorded session file.
func (t *RecordingTransport) Save(path string) error {
	return replay.WriteRecords(path, t.Records())
}

// NewHTTPTransport builds the HTTP connection on its own (for wrapping it
// in a RecordingTransport and passing it as Options.Transport).
func NewHTTPTransport(hostname, username, password string, opts ConnectionOptions) (*Connection, error) {
	return NewConnection(hostname, username, password, opts)
}
