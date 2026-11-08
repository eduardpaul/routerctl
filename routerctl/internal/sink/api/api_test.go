package api

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/view"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

const token = "s3cret"

type env struct {
	t   *testing.T
	f   *fake.Fake
	d   *poller.Daemon
	api *Sink
	srv *httptest.Server
}

func start(t *testing.T) *env {
	t.Helper()
	f := fake.New()
	store, _ := inventory.Open("")
	a := New(Options{Token: token, Policy: commands.DefaultPolicy, Version: "test", Heartbeat: 50 * time.Millisecond})
	iv := map[routerkit.Part]time.Duration{}
	for _, p := range routerkit.AllParts {
		iv[p] = 20 * time.Millisecond
	}
	d, err := poller.New([]poller.Device{{Name: "home", Driver: f, Intervals: iv}}, poller.Options{
		Store: store, Sinks: []poller.Sink{a}, Baseline: true,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), ActionRepoll: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = d.Run(ctx); close(done) }()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
		cancel()
		<-done
	})
	e := &env{t: t, f: f, d: d, api: a, srv: srv}
	// Wait for the first poll (inventory baseline recorded)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := d.State("home"); ok && st.Polls > 0 && len(st.Inventory) > 0 {
			return e
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("daemon did not poll")
	return nil
}

func (e *env) req(method, path, body string, out any) int {
	e.t.Helper()
	r, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, data)
		}
	}
	return resp.StatusCode
}

func TestAuth(t *testing.T) {
	e := start(t)
	resp, err := http.Get(e.srv.URL + Prefix + "/info")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 || resp.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("no token: %d", resp.StatusCode)
	}
	r, _ := http.NewRequest("GET", e.srv.URL+Prefix+"/info", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(r)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("wrong token: %d", resp.StatusCode)
	}
	var info Info
	if c := e.req("GET", Prefix+"/info", "", &info); c != 200 || info.API != 1 || !info.Auth || info.Policy.MaxDanger != "disruptive" || len(info.Devices) != 1 {
		t.Errorf("info: %d %+v", c, info)
	}
	var eb ErrorBody
	if c := e.req("GET", Prefix+"/nope", "", &eb); c != 404 || eb.Error.Code != commands.CodeNotFound {
		t.Errorf("unknown endpoint: %d %+v", c, eb)
	}
}

func TestReads(t *testing.T) {
	e := start(t)
	var devs []view.Device
	if c := e.req("GET", Prefix+"/devices", "", &devs); c != 200 || len(devs) != 1 || devs[0].Name != "home" || !devs[0].Online || devs[0].Capabilities != nil {
		t.Errorf("devices: %d %+v", c, devs)
	}
	var det view.DeviceDetail
	if c := e.req("GET", Prefix+"/devices/home", "", &det); c != 200 || det.Capabilities == nil || det.State.WANIP == "" || len(det.Nodes) != 2 {
		t.Errorf("device: %d %+v", c, det)
	}
	var eb ErrorBody
	if c := e.req("GET", Prefix+"/devices/nope", "", &eb); c != 404 || eb.Error.Code != commands.CodeUnknownDevice {
		t.Errorf("unknown device: %d %+v", c, eb)
	}
	var cs []view.Client
	if c := e.req("GET", Prefix+"/devices/home/clients", "", &cs); c != 200 || len(cs) != 3 || !cs[0].Known {
		t.Errorf("clients: %d %+v", c, cs)
	}
	if c := e.req("GET", Prefix+"/devices/home/clients?known=false", "", &cs); c != 200 || len(cs) != 0 {
		t.Errorf("unknown clients: %d %+v", c, cs)
	}
	if c := e.req("GET", Prefix+"/devices/home/clients?online=maybe", "", &eb); c != 400 {
		t.Errorf("bad bool: %d", c)
	}
	var nodes []view.Node
	if c := e.req("GET", Prefix+"/devices/home/nodes", "", &nodes); c != 200 || len(nodes) != 2 {
		t.Errorf("nodes: %d %+v", c, nodes)
	}
	var srcs []routerkit.SourceSpec
	if c := e.req("GET", Prefix+"/devices/home/sources", "", &srcs); c != 200 || len(srcs) == 0 {
		t.Errorf("sources: %d %+v", c, srcs)
	}
	var sb SourceBody
	if c := e.req("GET", Prefix+"/devices/home/sources/fake_status", "", &sb); c != 200 || sb.Source != "fake_status" || sb.Data == nil {
		t.Errorf("source: %d %+v", c, sb)
	}
	if c := e.req("GET", Prefix+"/devices/home/sources/nope", "", &eb); c != 404 || eb.Error.Code != commands.CodeNotFound {
		t.Errorf("unknown source: %d %+v", c, eb)
	}
}

func TestActions(t *testing.T) {
	e := start(t)
	var acts []commands.Action
	if c := e.req("GET", Prefix+"/devices/home/actions", "", &acts); c != 200 || len(acts) == 0 {
		t.Fatalf("actions: %d %+v", c, acts)
	}
	for _, a := range acts {
		if (a.Name == "reboot") == a.Allowed {
			t.Errorf("%s allowed=%v", a.Name, a.Allowed)
		}
	}
	var res commands.Result
	if c := e.req("POST", Prefix+"/devices/home/actions/led", `{"state": false, "_id": "n1"}`, &res); c != 200 || !res.OK || res.ID != "n1" || res.Args["state"] != "off" {
		t.Errorf("led: %d %+v", c, res)
	}
	if c := e.req("POST", Prefix+"/devices/home/actions/led", "on", &res); c != 200 || !res.OK {
		t.Errorf("led bare: %d %+v", c, res)
	}
	cases := []struct {
		path, body string
		status     int
		code       commands.Code
	}{
		{"/devices/home/actions/reboot", "", 403, commands.CodeForbidden},
		{"/devices/home/actions/led", "dim", 400, commands.CodeInvalidArgs},
		{"/devices/home/actions/fly", "", 404, commands.CodeUnsupported},
		{"/devices/nope/actions/led", "on", 404, commands.CodeUnknownDevice},
	}
	for _, c := range cases {
		var r commands.Result
		if got := e.req("POST", Prefix+c.path, c.body, &r); got != c.status || r.Error == nil || r.Error.Code != c.code {
			t.Errorf("%s: %d %+v", c.path, got, r)
		}
	}
	if calls := e.f.Calls(); len(calls) != 2 {
		t.Errorf("calls: %+v", calls)
	}
}

func TestInventory(t *testing.T) {
	e := start(t)
	var list []inventory.Entry
	if c := e.req("GET", Prefix+"/inventory", "", &list); c != 200 || len(list) != 3 {
		t.Fatalf("inventory: %d %+v", c, list)
	}
	mac := list[0].MAC
	var ent inventory.Entry
	if c := e.req("PATCH", Prefix+"/inventory/"+strings.ToUpper(mac), `{"known": false}`, &ent); c != 200 || ent.Known {
		t.Errorf("unapprove: %d %+v", c, ent)
	}
	if c := e.req("GET", Prefix+"/inventory?known=false", "", &list); c != 200 || len(list) != 1 {
		t.Errorf("unknown list: %d %+v", c, list)
	}
	if c := e.req("PATCH", Prefix+"/inventory/"+mac, `{"label": "Kitchen tablet"}`, &ent); c != 200 || ent.Label != "Kitchen tablet" || !ent.Known {
		t.Errorf("label: %d %+v", c, ent)
	}
	var eb ErrorBody
	if c := e.req("PATCH", Prefix+"/inventory/"+mac, `{"nope": 1}`, &eb); c != 400 {
		t.Errorf("unknown field: %d", c)
	}
	if c := e.req("PATCH", Prefix+"/inventory/not-a-mac", `{"known": true}`, &eb); c != 400 || eb.Error.Code != commands.CodeInvalidArgs {
		t.Errorf("bad mac: %d %+v", c, eb)
	}
	if c := e.req("DELETE", Prefix+"/inventory/"+mac, "", nil); c != 204 {
		t.Errorf("forget: %d", c)
	}
	if c := e.req("GET", Prefix+"/inventory/"+mac, "", &eb); c != 404 {
		t.Errorf("forgotten: %d", c)
	}
}

type sse struct {
	resp *http.Response
	sc   *bufio.Scanner
}

func (e *env) stream(query, lastID string) *sse {
	e.t.Helper()
	r, _ := http.NewRequest("GET", e.srv.URL+Prefix+"/events"+query, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if lastID != "" {
		r.Header.Set("Last-Event-ID", lastID)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		e.t.Fatalf("events: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	e.t.Cleanup(func() { resp.Body.Close() })
	return &sse{resp: resp, sc: bufio.NewScanner(resp.Body)}
}

type msg struct{ event, id, data string }

// next returns the next frame of type typ (skipping others).
func (s *sse) next(t *testing.T, typ string) msg {
	t.Helper()
	got := make(chan msg, 1)
	go func() {
		var m msg
		for s.sc.Scan() {
			line := s.sc.Text()
			switch {
			case line == "":
				if m.event == typ {
					got <- m
					return
				}
				m = msg{}
			case strings.HasPrefix(line, "event: "):
				m.event = line[7:]
			case strings.HasPrefix(line, "id: "):
				m.id = line[4:]
			case strings.HasPrefix(line, "data: "):
				m.data = line[6:]
			}
		}
		close(got)
	}()
	select {
	case m, ok := <-got:
		if !ok {
			t.Fatalf("stream ended before %s", typ)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s frame", typ)
	}
	return msg{}
}

func TestEventStream(t *testing.T) {
	e := start(t)
	s := e.stream("?types=device_new,action_done&devices=home", "")
	var hello Hello
	if m := s.next(t, "hello"); json.Unmarshal([]byte(m.data), &hello) != nil || hello.API != 1 || hello.Resumed {
		t.Fatalf("hello: %+v", m)
	}
	e.f.UpsertClient(routerkit.Client{MAC: "02:00:00:00:30:01", Name: "intruder", Online: true})
	m := s.next(t, "device_new")
	var ev events.Event
	if err := json.Unmarshal([]byte(m.data), &ev); err != nil || ev.ID != m.id || ev.Data["mac"] != "02:00:00:00:30:01" || ev.Summary == "" {
		t.Fatalf("device_new: %+v", m)
	}
	var res commands.Result
	e.req("POST", Prefix+"/devices/home/actions/led", "off", &res)
	done := s.next(t, "action_done")
	if !strings.Contains(done.data, `"source":"api"`) {
		t.Errorf("action_done: %+v", done)
	}

	// Resume from the device_new id: the action_done is replayed
	r := e.stream("?types=device_new,action_done", m.id)
	if m := r.next(t, "hello"); !strings.Contains(m.data, `"replayed":1`) || !strings.Contains(m.data, `"resumed":true`) {
		t.Errorf("resume hello: %+v", m)
	}
	if again := r.next(t, "action_done"); again.id != done.id {
		t.Errorf("replayed %s, want %s", again.id, done.id)
	}

	// State frames on request
	st := e.stream("?state=1&types=wan_down", "")
	var sf StateFrame
	if m := st.next(t, "state"); json.Unmarshal([]byte(m.data), &sf) != nil || sf.Device != "home" || !sf.Online {
		t.Errorf("state frame: %+v", m)
	}

	// Bad selector
	var eb ErrorBody
	if c := e.req("GET", Prefix+"/events?types=bogus", "", &eb); c != 400 {
		t.Errorf("bad type: %d", c)
	}
}

// ---- contract golden ----

var update = flag.Bool("update", false, "rewrite testdata/contract.golden.json")

// schemaOf describes the JSON shape of a type (the routerkit
// model.schema.golden.json format, with embedded structs flattened).
func schemaOf(t reflect.Type, seen map[reflect.Type]bool) any {
	switch t.Kind() {
	case reflect.Pointer:
		return map[string]any{"ptr": schemaOf(t.Elem(), seen)}
	case reflect.Slice:
		return map[string]any{"list": schemaOf(t.Elem(), seen)}
	case reflect.Map:
		return map[string]any{"map": t.Key().Name(), "of": schemaOf(t.Elem(), seen)}
	case reflect.Struct:
		if !strings.HasPrefix(t.PkgPath(), "github.com/eduardpaul/routerctl/") || seen[t] || t == reflect.TypeOf(time.Time{}) {
			return t.String()
		}
		seen[t] = true
		defer delete(seen, t)
		out := map[string]any{}
		fields(t, seen, out)
		return out
	}
	return t.String()
}

func fields(t reflect.Type, seen map[reflect.Type]bool, out map[string]any) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if f.Anonymous && tag == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			fields(ft, seen, out)
			continue
		}
		if !f.IsExported() || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = f.Name
		}
		entry := map[string]any{"type": schemaOf(f.Type, seen)}
		if strings.Contains(tag, "omitempty") {
			entry["omitempty"] = true
		}
		out[name] = entry
	}
}

// TestContract: the JSON of every API payload is a public contract (the
// Node-RED package tests against testdata/contract.golden.json). Review
// the diff, then run go test -update.
func TestContract(t *testing.T) {
	types := map[string]any{}
	for name, v := range map[string]any{
		"Info": Info{}, "Hello": Hello{}, "Device": view.Device{}, "DeviceDetail": view.DeviceDetail{},
		"Client": view.Client{}, "Node": view.Node{}, "Action": commands.Action{}, "Result": commands.Result{},
		"Error": ErrorBody{}, "Event": events.Event{}, "StateFrame": StateFrame{}, "Source": SourceBody{},
		"InventoryEntry": inventory.Entry{}, "InventoryPatch": InventoryPatch{}, "SourceSpec": routerkit.SourceSpec{},
	} {
		types[name] = schemaOf(reflect.TypeOf(v), map[reflect.Type]bool{})
	}
	doc := map[string]any{
		"api": Version, "types": types,
		"event_types": events.AllTypes,
		"error_codes": []commands.Code{
			commands.CodeInvalidArgs, commands.CodeForbidden, commands.CodeUnsupported, commands.CodeUnknownDevice,
			commands.CodeNotFound, commands.CodeNotReady, commands.CodeRejected, commands.CodeBusy, commands.CodeAuth,
			commands.CodeUnreachable, commands.CodeTimeout, commands.CodeDevice,
		},
	}
	got, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := "testdata/contract.golden.json"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(want) != string(got) {
		t.Errorf("the API contract changed; review the diff and run go test -update:\n%s", got)
	}
}
