package mqtt

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

var update = flag.Bool("update", false, "rewrite golden files")

type pub struct {
	Topic    string
	Retained bool
	Payload  string
}

type fakeClient struct {
	mu   sync.Mutex
	pubs []pub
	subs map[string]func(string, []byte)
}

func (c *fakeClient) Connect(onConnect func()) error {
	onConnect()
	return nil
}

func (c *fakeClient) Publish(topic string, _ byte, retained bool, payload []byte) error {
	c.mu.Lock()
	c.pubs = append(c.pubs, pub{topic, retained, string(payload)})
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Subscribe(topic string, _ byte, h func(string, []byte)) error {
	c.mu.Lock()
	if c.subs == nil {
		c.subs = map[string]func(string, []byte){}
	}
	c.subs[topic] = h
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Disconnect() {}

func (c *fakeClient) last(topic string) (pub, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.pubs) - 1; i >= 0; i-- {
		if c.pubs[i].Topic == topic {
			return c.pubs[i], true
		}
	}
	return pub{}, false
}

func (c *fakeClient) deliver(topic string, payload string) {
	c.mu.Lock()
	var h func(string, []byte)
	for pattern, fn := range c.subs {
		if match(pattern, topic) {
			h = fn
		}
	}
	c.mu.Unlock()
	if h != nil {
		h(topic, []byte(payload))
	}
}

func match(pattern, topic string) bool {
	p, t := strings.Split(pattern, "/"), strings.Split(topic, "/")
	if len(p) != len(t) {
		return false
	}
	for i := range p {
		if p[i] != "+" && p[i] != t[i] {
			return false
		}
	}
	return true
}

func testState(t *testing.T) poller.DeviceState {
	t.Helper()
	f := fake.New()
	snap, err := f.Poll(context.Background(), routerkit.Poll(routerkit.AllParts...))
	if err != nil {
		t.Fatal(err)
	}
	snap.Time = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	id, _ := f.Identity(context.Background())
	caps, _ := f.Capabilities(context.Background())
	return poller.DeviceState{
		Name: "home", Driver: "fake", Identity: id, Online: true,
		LastPoll: snap.Time, Snapshot: snap, Capabilities: caps,
		Presence: map[string]bool{"02:00:00:00:10:01": true, "02:00:00:00:10:02": false},
		Inventory: inventory.Map{
			"02:00:00:00:10:01": {MAC: "02:00:00:00:10:01", Name: "laptop", Label: "Work laptop", Known: true},
			"02:00:00:00:10:02": {MAC: "02:00:00:00:10:02", Name: "phone"},
			"02:00:00:00:10:03": {MAC: "02:00:00:00:10:03", Name: "nas", Known: true},
		},
	}
}

func testOptions() Options {
	return Options{
		Topics: Topics{Base: "routerctl"}, DiscoveryPrefix: "homeassistant",
		Trackers: "known", Commands: true, BlockSwitches: true, Version: "test",
		Policy: commands.DefaultPolicy,
	}
}

func TestDiscoveryGolden(t *testing.T) {
	got := Discovery(testOptions(), testState(t))
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	golden := filepath.Join("testdata", "discovery.golden.json")
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update)", err)
	}
	if string(want) != string(data) {
		t.Errorf("discovery payloads differ from %s (run go test -update and review the diff)", golden)
	}
}

func TestDiscoveryShape(t *testing.T) {
	st := testState(t)
	got := Discovery(testOptions(), st)
	var trackers, buttons []string
	for topic, p := range got {
		parts := strings.Split(topic, "/")
		if len(parts) != 5 || parts[0] != "homeassistant" || parts[2] != "routerctl_home" || parts[4] != "config" {
			t.Errorf("bad topic %s", topic)
		}
		for _, k := range []string{"unique_id", "availability", "device", "origin"} {
			if _, ok := p[k]; !ok {
				t.Errorf("%s: missing %s", topic, k)
			}
		}
		switch parts[1] {
		case "device_tracker":
			trackers = append(trackers, parts[3])
		case "button":
			buttons = append(buttons, parts[3])
		}
	}
	sort.Strings(trackers)
	// Only known/labelled clients get trackers by default
	if strings.Join(trackers, ",") != "client_020000001001,client_020000001003" {
		t.Errorf("trackers = %v", trackers)
	}
	// reboot is dangerous: no button unless allowed
	if strings.Join(buttons, ",") != "speedtest" {
		t.Errorf("buttons = %v", buttons)
	}
	o := testOptions()
	o.Policy, o.Trackers = commands.Policy{MaxDanger: routerkit.Destructive}, "all"
	got = Discovery(o, st)
	if _, ok := got["homeassistant/button/routerctl_home/reboot/config"]; !ok {
		t.Error("reboot button missing with allow_dangerous")
	}
	if _, ok := got["homeassistant/device_tracker/routerctl_home/client_020000001002/config"]; !ok {
		t.Error("trackers=all should include unknown clients")
	}
	o.Trackers, o.Commands = "none", false
	for topic := range Discovery(o, st) {
		if strings.Contains(topic, "/device_tracker/") || strings.Contains(topic, "/switch/") || strings.Contains(topic, "/button/") {
			t.Errorf("unexpected entity %s", topic)
		}
	}
	sw := got["homeassistant/switch/routerctl_home/wifi_5g1/config"]
	if sw == nil || sw["command_template"] != `{"band":"5g1","state":"{{ value }}"}` || sw["command_topic"] != "routerctl/home/cmd/wifi" {
		t.Errorf("wifi switch: %+v", sw)
	}
}

func newSink(t *testing.T, mutate func(*Options)) (*Sink, *fakeClient, *recCmd) {
	o := Options{
		Topics: Topics{Base: "routerctl"}, DiscoveryPrefix: "homeassistant",
		Trackers: "known", Commands: true, Version: "test", QoS: 1,
		Policy: commands.DefaultPolicy,
	}
	if mutate != nil {
		mutate(&o)
	}
	fc := &fakeClient{}
	s := New(o, fc)
	cmd := &recCmd{states: map[string]poller.DeviceState{"home": testState(t)}}
	if err := s.Start(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	return s, fc, cmd
}

// recCmd is a poller.Control that records actions.
type recCmd struct {
	mu     sync.Mutex
	calls  []string
	states map[string]poller.DeviceState
}

func (r *recCmd) Devices() []string { return []string{"home"} }
func (r *recCmd) State(dev string) (poller.DeviceState, bool) {
	st, ok := r.states[dev]
	return st, ok
}
func (r *recCmd) Get(context.Context, string, string) (any, error) { return nil, nil }
func (r *recCmd) Inventory() poller.Inventory                      { return nil }

func (r *recCmd) Do(_ context.Context, dev string, inv routerkit.Invocation) (routerkit.Result, error) {
	args := inv.Args.Strings()
	keys := make([]string, 0, len(args))
	for k, v := range args {
		keys = append(keys, k+"="+v)
	}
	sort.Strings(keys)
	r.mu.Lock()
	r.calls = append(r.calls, dev+" "+inv.Name+" "+strings.Join(keys, " "))
	r.mu.Unlock()
	return routerkit.Result{Accepted: true, Data: map[string]any{"ok": true}}, nil
}

func TestStateTopics(t *testing.T) {
	s, fc, _ := newSink(t, nil)
	st := testState(t)
	s.State(st)
	if p, ok := fc.last("routerctl/status"); !ok || p.Payload != "online" || !p.Retained {
		t.Errorf("status: %+v", p)
	}
	if p, _ := fc.last("routerctl/home/availability"); p.Payload != "online" {
		t.Errorf("availability: %+v", p)
	}
	if p, _ := fc.last("routerctl/home/client/020000001002/presence"); p.Payload != NotHome || !p.Retained {
		t.Errorf("presence (debounced offline): %+v", p)
	}
	if p, _ := fc.last("routerctl/home/client/020000001001/presence"); p.Payload != Home {
		t.Errorf("presence: %+v", p)
	}
	p, _ := fc.last("routerctl/home/client/020000001001")
	var cp view.Client
	if err := json.Unmarshal([]byte(p.Payload), &cp); err != nil || cp.Name != "Work laptop" || !cp.Known || cp.IP != "192.168.50.10" {
		t.Errorf("client payload: %s", p.Payload)
	}
	p, _ = fc.last("routerctl/home/state")
	var sp map[string]any
	json.Unmarshal([]byte(p.Payload), &sp)
	if sp["wan_ip"] != "203.0.113.7" || sp["cpu_usage"] != 12.5 || sp["clients_online"] != float64(3) {
		t.Errorf("state payload: %s", p.Payload)
	}

	// A client that disappears is cleared; an unknown -> known flip adds a tracker
	n := len(fc.pubs)
	st2 := testState(t)
	st2.Snapshot.Clients = st2.Snapshot.Clients[:2]
	s.State(st2)
	if p, _ := fc.last("routerctl/home/client/020000001003"); p.Payload != "" {
		t.Errorf("vanished client not cleared: %+v", p)
	}
	if p, _ := fc.last("homeassistant/device_tracker/routerctl_home/client_020000001003/config"); p.Payload != "" {
		t.Errorf("vanished tracker not removed: %+v", p)
	}
	// Unchanged retained payloads are not republished
	for _, p := range fc.pubs[n:] {
		if p.Topic == "routerctl/home/identity" {
			t.Error("identity republished without a change")
		}
	}
}

func TestStaleDiscoveryCleared(t *testing.T) {
	s, fc, _ := newSink(t, nil)
	st := testState(t)
	s.State(st)
	// A retained config from an earlier run shows up
	stale := "homeassistant/device_tracker/routerctl_home/client_deadbeef0001/config"
	fc.deliver(stale, `{"name":"old"}`)
	s.State(st)
	if p, ok := fc.last(stale); !ok || p.Payload != "" || !p.Retained {
		t.Errorf("stale config not cleared: %+v", p)
	}
}

func waitResult(t *testing.T, fc *fakeClient, topic string) commands.Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p, ok := fc.last(topic); ok {
			var r commands.Result
			if err := json.Unmarshal([]byte(p.Payload), &r); err != nil {
				t.Fatal(err)
			}
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no result on %s", topic)
	return commands.Result{}
}

func TestCommands(t *testing.T) {
	s, fc, cmd := newSink(t, func(o *Options) { o.BlockSwitches = true })
	s.State(testState(t))

	fc.deliver("routerctl/home/cmd/led", "OFF")
	if r := waitResult(t, fc, "routerctl/home/cmd/led/result"); !r.OK || r.Args["state"] != "off" {
		t.Errorf("led result: %+v", r)
	}
	if p, _ := fc.last("routerctl/home/switch/led"); p.Payload != "off" || !p.Retained {
		t.Errorf("led switch state: %+v", p)
	}

	fc.deliver("routerctl/home/cmd/wifi", `{"band":"5g1","state":true,"_id":"req-7"}`)
	if r := waitResult(t, fc, "routerctl/home/cmd/wifi/result"); !r.OK || r.ID != "req-7" || r.Device != "home" {
		t.Errorf("wifi result: %+v", r)
	}
	if p, _ := fc.last("routerctl/home/switch/wifi_5g1"); p.Payload != "on" {
		t.Errorf("wifi switch state: %+v", p)
	}

	fc.deliver("routerctl/home/cmd/block-client", "mac=02:00:00:00:10:02 state=on")
	if r := waitResult(t, fc, "routerctl/home/cmd/block-client/result"); !r.OK {
		t.Errorf("block result: %+v", r)
	}
	if p, _ := fc.last("routerctl/home/switch/block_020000001002"); p.Payload != "on" {
		t.Errorf("block switch: %+v", p)
	}

	fc.deliver("routerctl/home/cmd/reboot", "PRESS")
	if r := waitResult(t, fc, "routerctl/home/cmd/reboot/result"); r.OK || r.Error == nil || r.Error.Code != commands.CodeForbidden {
		t.Errorf("reboot should be refused: %+v", r)
	}
	fc.deliver("routerctl/home/cmd/nope", "")
	if r := waitResult(t, fc, "routerctl/home/cmd/nope/result"); r.OK {
		t.Errorf("unknown action accepted: %+v", r)
	}
	fc.deliver("routerctl/home/cmd/wifi", `{"band":"9g","state":"on"}`)
	time.Sleep(50 * time.Millisecond)
	if r := waitResult(t, fc, "routerctl/home/cmd/wifi/result"); r.OK {
		t.Errorf("bad choice accepted: %+v", r)
	}
	s.Close(context.Background())

	cmd.mu.Lock()
	defer cmd.mu.Unlock()
	want := []string{"home led state=off", "home wifi band=5g1 state=on", "home block-client mac=02:00:00:00:10:02 state=on"}
	if strings.Join(cmd.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %q", cmd.calls)
	}
	if p, _ := fc.last("routerctl/status"); p.Payload != "offline" {
		t.Errorf("close should publish offline: %+v", p)
	}
}

// Parts the device does not support get no entities, even with data.
func TestDiscoverySupportedPartsOnly(t *testing.T) {
	st := testState(t)
	st.Capabilities.Parts[routerkit.PartSystem] = routerkit.PartSupport{Level: routerkit.Unsupported}
	st.Capabilities.Parts[routerkit.PartNodes] = routerkit.PartSupport{Level: routerkit.Unsupported}
	got := Discovery(testOptions(), st)
	for topic := range got {
		if strings.Contains(topic, "/cpu_usage/") || strings.Contains(topic, "/temperature_") || strings.Contains(topic, "/node_") {
			t.Errorf("entity of an unsupported part: %s", topic)
		}
	}
	if _, ok := got["homeassistant/binary_sensor/routerctl_home/wan/config"]; !ok {
		t.Error("supported WAN entity missing")
	}
}

// Switches come from ActionSpec.Toggle (any bool arg name), not from an
// argument that happens to be called "state" with on/off choices.
func TestActionEntitiesFromToggle(t *testing.T) {
	specs := []routerkit.ActionSpec{
		{Name: "guest", Toggle: "enabled", Args: []routerkit.Arg{{Name: "enabled", Type: routerkit.ArgBool, Required: true}}},
		{Name: "legacy", Args: []routerkit.Arg{{Name: "state", Type: routerkit.ArgEnum, Required: true, Choices: []string{"on", "off"}}}},
		routerkit.Std(routerkit.ActVPN, routerkit.WithChoices("protocol", "openvpn", "wireguard")),
		routerkit.Std(routerkit.ActReboot),
	}
	got := map[string]switchSpec{}
	for _, e := range actionEntities(specs, commands.DefaultPolicy) {
		got[e.Object] = e
	}
	if e, ok := got["guest"]; !ok || e.Button || e.Toggle != "enabled" {
		t.Errorf("guest switch: %+v", got)
	}
	if _, ok := got["legacy"]; ok {
		t.Error("an untyped on/off arg made a switch")
	}
	if _, ok := got["vpn_wireguard"]; !ok {
		t.Errorf("per-choice switches: %+v", got)
	}
	if _, ok := got["reboot"]; ok {
		t.Error("destructive action exposed without allow_dangerous")
	}
	if tpl := commandTemplate(nil, "enabled"); tpl != `{"enabled":"{{ value }}"}` {
		t.Errorf("template %s", tpl)
	}
}

func TestEventsPublished(t *testing.T) {
	s, fc, _ := newSink(t, nil)
	s.Event(events.Event{Type: events.WANDown, Device: "home", Time: time.Unix(0, 0).UTC(), Data: map[string]any{}})
	p, ok := fc.last("routerctl/home/event/wan_down")
	if !ok || p.Retained || !strings.Contains(p.Payload, `"type":"wan_down"`) {
		t.Errorf("event: %+v", p)
	}
}
