package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
)

type hit struct {
	Path    string
	Query   string
	Header  http.Header
	Body    string
	Payload map[string]any
}

type recorder struct {
	mu   sync.Mutex
	hits []hit
	fail int // fail the first n requests with 503
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	b, _ := io.ReadAll(req.Body)
	h := hit{Path: req.URL.Path, Query: req.URL.RawQuery, Header: req.Header.Clone(), Body: string(b)}
	_ = json.Unmarshal(b, &h.Payload)
	r.mu.Lock()
	r.hits = append(r.hits, h)
	fail := r.fail > 0
	if fail {
		r.fail--
	}
	r.mu.Unlock()
	if fail {
		w.WriteHeader(503)
		return
	}
	w.WriteHeader(200)
}

func (r *recorder) all() []hit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]hit(nil), r.hits...)
}

var ev = events.Event{
	Type: events.DeviceNew, Device: "asus", Time: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC),
	Data: map[string]any{"mac": "aa:bb:cc:00:00:99", "name": "visitor", "ip": "10.0.0.50", "vendor": "Acme"},
}

func TestNtfy(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s, err := New([]Options{{
		Type: "ntfy", URL: srv.URL, Topic: "home", Token: "tk", Priority: 4, Tags: []string{"house"},
		Events: []string{"device_new"},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Event(ev)
	s.Event(events.Event{Type: events.WANDown, Device: "asus"}) // filtered out
	hits := rec.all()
	if len(hits) != 1 {
		t.Fatalf("hits = %d", len(hits))
	}
	h := hits[0]
	if h.Path != "/home" || h.Header.Get("Title") != "routerctl: New device" || h.Header.Get("Priority") != "4" ||
		h.Header.Get("Tags") != "house,device_new" || h.Header.Get("Authorization") != "Bearer tk" {
		t.Errorf("ntfy request: %+v", h)
	}
	if h.Body != "New device on asus: visitor (aa:bb:cc:00:00:99), IP 10.0.0.50, vendor Acme" {
		t.Errorf("ntfy body: %q", h.Body)
	}
}

func TestGotifyTemplates(t *testing.T) {
	rec := &recorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s, err := New([]Options{{
		Type: "gotify", URL: srv.URL + "/", Token: "gt", Priority: 7,
		Title:   "{{ .Device | upper }}: {{ .Title }}",
		Message: `{{ index .Data "name" }} at {{ index .Data "ip" }} ({{ default "?" (index .Data "label") }}) {{ .Time.Format "15:04" }}`,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Event(ev)
	hits := rec.all()
	if len(hits) != 1 {
		t.Fatalf("hits = %d", len(hits))
	}
	h := hits[0]
	if h.Path != "/message" || h.Query != "" || h.Header.Get("X-Gotify-Key") != "gt" {
		t.Errorf("gotify request: %+v", h)
	}
	if h.Payload["title"] != "ASUS: New device" || h.Payload["message"] != "visitor at 10.0.0.50 (?) 10:00" || h.Payload["priority"] != float64(7) {
		t.Errorf("gotify payload: %+v", h.Payload)
	}
}

func TestWebhookRetryAndHeaders(t *testing.T) {
	rec := &recorder{fail: 1}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	s, err := New([]Options{{
		Type: "webhook", URL: srv.URL + "/hook", Headers: map[string]string{"X-Source": "test"}, Devices: []string{"asus"},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Event(ev)
	other := ev
	other.Device = "ont" // device filter
	s.Event(other)
	hits := rec.all()
	if len(hits) != 2 { // one 503, one retry
		t.Fatalf("hits = %d", len(hits))
	}
	h := hits[1]
	if h.Header.Get("X-Source") != "test" || h.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers: %v", h.Header)
	}
	if h.Payload["type"] != "device_new" || h.Payload["device"] != "asus" || h.Payload["message"] == "" {
		t.Errorf("webhook payload: %+v", h.Payload)
	}
	data := h.Payload["data"].(map[string]any)
	if data["mac"] != "aa:bb:cc:00:00:99" {
		t.Errorf("data: %+v", data)
	}
}

func TestBadConfig(t *testing.T) {
	if _, err := New([]Options{{Type: "ntfy", URL: "http://x", Topic: "t", Events: []string{"device_new", "bogus"}}}, nil); err == nil {
		t.Error("unknown event accepted")
	}
	if _, err := New([]Options{{Type: "ntfy", URL: "http://x", Topic: "t", Message: "{{ .Nope"}}, nil); err == nil {
		t.Error("broken template accepted")
	}
}

func TestErrorsDoNotLeakTokens(t *testing.T) {
	for _, c := range []Options{
		{Type: "gotify", URL: "http://127.0.0.1:1", Token: "supersecret"},
		{Type: "webhook", URL: "http://127.0.0.1:1/hooks/supersecret"},
	} {
		c.Timeout = time.Second
		tg, err := NewTarget(c, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = tg.Send(t.Context(), ev)
		if err == nil || strings.Contains(err.Error(), "supersecret") {
			t.Errorf("%s: err = %v", c.Type, err)
		}
	}
}

func TestNonASCIITitle(t *testing.T) {
	if got := headerSafe("Nuevo dispositivo: Teléfono\n"); !strings.HasPrefix(got, "=?utf-8?b?") {
		t.Errorf("headerSafe = %q", got)
	}
	if got := headerSafe("plain\r\ntitle"); got != "plain  title" {
		t.Errorf("headerSafe = %q", got)
	}
}
