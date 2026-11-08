package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

var now = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

func state(t *testing.T) poller.DeviceState {
	f := fake.New()
	snap, err := f.Poll(context.Background(), routerkit.Poll(routerkit.AllParts...))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := f.Identity(context.Background())
	return poller.DeviceState{
		Name: "home", Driver: "fake", Identity: id, Online: true, LastPoll: now,
		Snapshot: snap, Polls: 3, PartErrors: map[routerkit.Part]uint64{routerkit.PartFirmware: 1},
		Presence:  map[string]bool{"02:00:00:00:10:02": false},
		Inventory: inventory.Map{"02:00:00:00:10:01": {MAC: "02:00:00:00:10:01", Label: `Say "hi"`, Known: true}},
	}
}

func TestExposition(t *testing.T) {
	s := New(true)
	s.Now = func() time.Time { return now }
	s.State(state(t))
	s.Event(events.Event{Type: events.DeviceNew, Device: "home"})
	s.Event(events.Event{Type: events.DeviceNew, Device: "home"})
	var b strings.Builder
	s.Write(&b)
	out := b.String()

	for _, want := range []string{
		`routerctl_up{device="home"} 1`,
		`routerctl_info{device="home",driver="fake",vendor="Fake",model="FAKE-1000",firmware="1.0.0"} 1`,
		`routerctl_cpu_usage_percent{device="home"} 12.5`,
		`routerctl_temperature_celsius{device="home",sensor="cpu"} 55.5`,
		`routerctl_wan_connected{device="home"} 1`,
		`routerctl_clients_online{device="home"} 3`,
		`routerctl_client_online{device="home",mac="02:00:00:00:10:01",name="Say \"hi\"",known="true"} 1`,
		// Debounced presence wins over the raw online flag
		`routerctl_client_online{device="home",mac="02:00:00:00:10:02",name="phone",known="false"} 0`,
		`routerctl_node_online{device="home",mac="02:00:00:00:00:02",name="Office",model="FAKE-500"} 1`,
		`routerctl_events_total{device="home",type="device_new"} 2`,
		`routerctl_part_errors_total{device="home",part="firmware"} 1`,
		`routerctl_last_poll_timestamp_seconds{device="home"} 1794045600`,
		"# TYPE routerctl_interface_receive_bytes_total counter",
		"# HELP routerctl_up Whether the last poll reached the device.",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %q", want)
		}
	}

	// Every line is a comment or a valid sample; each family has one
	// HELP and one TYPE, before its samples
	sample := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{([a-zA-Z_][a-zA-Z0-9_]*="([^"\\]|\\.)*",?)*\})? [-+0-9.eE]+|NaN|[+-]Inf$`)
	seenType := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			name := strings.Fields(line)[2]
			if seenType[name] {
				t.Errorf("duplicate TYPE for %s", name)
			}
			seenType[name] = true
			continue
		}
		if strings.HasPrefix(line, "# HELP ") {
			continue
		}
		if !sample.MatchString(line) {
			t.Errorf("invalid sample line %q", line)
		}
		name := line[:strings.IndexAny(line, "{ ")]
		if !seenType[name] {
			t.Errorf("sample before TYPE: %q", line)
		}
	}
}

func TestPerClientOff(t *testing.T) {
	s := New(false)
	s.State(state(t))
	var b strings.Builder
	s.Write(&b)
	if strings.Contains(b.String(), "routerctl_client_online") {
		t.Error("per-client series exported with per_client off")
	}
}

func TestHTTP(t *testing.T) {
	s := New(true)
	clock := now
	s.Now = func() time.Time { return clock }
	s.StaleAfter = time.Minute
	if err := s.Start(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	st := state(t)
	s.State(st)
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain; version=0.0.4") || !strings.Contains(string(body), "routerctl_up") {
		t.Fatalf("/metrics: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	get := func() int {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := get(); c != 200 {
		t.Errorf("healthy: %d", c)
	}
	// Device unreachable but within StaleAfter: still healthy
	st.Online = false
	s.State(st)
	clock = now.Add(30 * time.Second)
	if c := get(); c != 200 {
		t.Errorf("recently ok: %d", c)
	}
	clock = now.Add(2 * time.Minute)
	if c := get(); c != 503 {
		t.Errorf("stale: %d", c)
	}
}
