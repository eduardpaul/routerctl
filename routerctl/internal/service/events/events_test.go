package events

import (
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerkit"
)

var t0 = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

const (
	phone = "aa:bb:cc:00:00:01"
	tv    = "aa:bb:cc:00:00:02"
	guest = "aa:bb:cc:00:00:99"
)

func cl(mac, ip string, online bool) routerkit.Client {
	return routerkit.Client{MAC: mac, IP: ip, Online: online, Name: "n-" + mac[len(mac)-2:]}
}

func types(evs []Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, string(e.Type))
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type tstep struct {
	at      time.Duration
	clients []routerkit.Client
	want    []string
}

func TestPresenceDebounce(t *testing.T) {
	inv := inventory.Map{
		phone: {MAC: phone, IP: "10.0.0.5", Known: true},
		tv:    {MAC: tv, IP: "10.0.0.6", Known: true, Label: "TV"},
	}
	cfg := Config{OfflineGrace: 3 * time.Minute, OnlineDebounce: 0}
	steps := []tstep{
		{0, []routerkit.Client{cl(phone, "10.0.0.5", true), cl(tv, "10.0.0.6", true)}, nil},
		// Phone sleeps: offline, but within the grace
		{30 * time.Second, []routerkit.Client{cl(phone, "10.0.0.5", false), cl(tv, "10.0.0.6", true)}, nil},
		{2 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", false), cl(tv, "10.0.0.6", true)}, nil},
		// Back before the grace expired: no event at all
		{3 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", true), cl(tv, "10.0.0.6", true)}, nil},
		// TV vanishes from the list (counts as offline)
		{4 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", true)}, nil},
		{6*time.Minute + 59*time.Second, []routerkit.Client{cl(phone, "10.0.0.5", true)}, nil},
		{7 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", true)}, []string{"device_offline"}},
		// Stays offline: fires once
		{8 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", true)}, nil},
		// TV comes back: online debounce 0 fires at once
		{9 * time.Minute, []routerkit.Client{cl(phone, "10.0.0.5", true), cl(tv, "10.0.0.6", true)}, []string{"device_online"}},
	}
	st := State{}
	for i, s := range steps {
		var evs []Event
		st, evs = Diff(st, routerkit.Snapshot{Clients: s.clients}, inv, t0.Add(s.at), cfg, "asus")
		if !eq(types(evs), s.want...) {
			t.Fatalf("step %d (%v): got %v, want %v", i, s.at, types(evs), s.want)
		}
		for _, e := range evs {
			if e.Device != "asus" || e.Data["mac"] != tv || e.Data["name"] != "TV" {
				t.Errorf("step %d: event %+v", i, e)
			}
		}
		if i == 6 {
			if evs[0].Data["since"] != t0.Add(4*time.Minute).Format(time.RFC3339) {
				t.Errorf("offline since = %v", evs[0].Data["since"])
			}
			if p, _ := st.Present(tv); p {
				t.Error("tv should be reported offline")
			}
		}
	}
}

func TestOnlineDebounce(t *testing.T) {
	inv := inventory.Map{phone: {MAC: phone, Known: true}}
	cfg := Config{OfflineGrace: time.Minute, OnlineDebounce: time.Minute}
	st, _ := Diff(State{}, routerkit.Snapshot{Clients: []routerkit.Client{cl(phone, "", false)}}, inv, t0, cfg, "")
	st, evs := Diff(st, routerkit.Snapshot{Clients: []routerkit.Client{cl(phone, "", true)}}, inv, t0.Add(10*time.Second), cfg, "")
	if len(evs) != 0 {
		t.Fatalf("debounced online fired early: %v", types(evs))
	}
	_, evs = Diff(st, routerkit.Snapshot{Clients: []routerkit.Client{cl(phone, "", true)}}, inv, t0.Add(80*time.Second), cfg, "")
	if !eq(types(evs), "device_online") {
		t.Fatalf("got %v", types(evs))
	}
}

func TestDeviceNewAndIPChange(t *testing.T) {
	inv := inventory.Map{phone: {MAC: phone, IP: "10.0.0.5", Known: true, Name: "phone"}}
	snap := routerkit.Snapshot{Clients: []routerkit.Client{
		{MAC: "AA:BB:CC:00:00:99", IP: "10.0.0.50", Name: "visitor", Vendor: "Acme", Connection: "2.4ghz", Online: true},
		{MAC: phone, IP: "10.0.0.7", Name: "phone", Online: true},
	}}
	st, evs := Diff(State{}, snap, inv, t0, Config{}, "asus")
	if !eq(types(evs), "device_ip_changed", "device_new") {
		t.Fatalf("got %v", types(evs))
	}
	ipc, nw := evs[0], evs[1]
	if ipc.Data["old_ip"] != "10.0.0.5" || ipc.Data["new_ip"] != "10.0.0.7" {
		t.Errorf("ip change: %+v", ipc.Data)
	}
	if nw.Data["mac"] != guest || nw.Data["vendor"] != "Acme" || nw.Data["known"] != false || nw.Data["ip"] != "10.0.0.50" {
		t.Errorf("new: %+v", nw.Data)
	}
	if !strings.Contains(nw.Describe(), "New device on asus: visitor (aa:bb:cc:00:00:99)") {
		t.Errorf("summary: %s", nw.Describe())
	}
	if _, ok := st.Clients[guest]; !ok {
		t.Error("new client not tracked")
	}

	// Baseline: no device_new
	_, evs = Diff(State{}, snap, inventory.Map{}, t0, Config{Baseline: true}, "asus")
	if len(evs) != 0 {
		t.Errorf("baseline fired %v", types(evs))
	}
}

func TestWAN(t *testing.T) {
	w := func(conn bool, ip string) routerkit.Snapshot {
		return routerkit.Snapshot{WAN: &routerkit.WAN{Connected: routerkit.Ptr(conn), IP: ip}}
	}
	seq := []struct {
		snap routerkit.Snapshot
		want []string
	}{
		{w(true, "203.0.113.7"), nil},
		{w(true, "203.0.113.7"), nil},
		{w(false, ""), []string{"wan_down"}},
		{w(false, "0.0.0.0"), nil},
		{w(true, "203.0.113.8"), []string{"wan_up", "wan_ip_changed"}},
		// Failed part: untouched
		{routerkit.Snapshot{WAN: &routerkit.WAN{Connected: routerkit.Ptr(false)}, Errors: map[routerkit.Part]routerkit.PartError{routerkit.PartWAN: {Message: "boom"}}}, nil},
		// Missing part: untouched
		{routerkit.Snapshot{}, nil},
		// Public IP from a double-NAT router (Extra wins over the WAN address)
		{routerkit.Snapshot{WAN: &routerkit.WAN{Connected: routerkit.Ptr(true), IP: "192.168.1.2", PublicIP: "203.0.113.9"}}, []string{"wan_ip_changed"}},
	}
	st := State{}
	for i, s := range seq {
		var evs []Event
		st, evs = Diff(st, s.snap, inventory.Map{}, t0.Add(time.Duration(i)*time.Minute), Config{}, "asus")
		if !eq(types(evs), s.want...) {
			t.Fatalf("step %d: got %v, want %v", i, types(evs), s.want)
		}
		if i == 4 && (evs[1].Data["old_ip"] != "203.0.113.7" || evs[1].Data["new_ip"] != "203.0.113.8") {
			t.Errorf("ip change data %+v", evs[1].Data)
		}
	}
	if st.WANIP != "203.0.113.9" {
		t.Errorf("WANIP = %q", st.WANIP)
	}
}

func TestFirmware(t *testing.T) {
	fw := func(avail string) routerkit.Snapshot {
		return routerkit.Snapshot{Firmware: &routerkit.Firmware{Current: "1.0", Available: avail, UpdateAvailable: routerkit.Ptr(avail != "")}}
	}
	seq := []struct {
		snap routerkit.Snapshot
		want []string
	}{
		{fw(""), nil},
		{fw("1.1"), []string{"firmware_update"}},
		{fw("1.1"), nil},
		{fw("1.2"), []string{"firmware_update"}},
		{fw(""), nil},
	}
	st := State{}
	for i, s := range seq {
		var evs []Event
		st, evs = Diff(st, s.snap, inventory.Map{}, t0, Config{}, "asus")
		if !eq(types(evs), s.want...) {
			t.Fatalf("step %d: got %v, want %v", i, types(evs), s.want)
		}
	}
	// First sight of a pending update (no stored state) fires once
	_, evs := Diff(State{}, fw("2.0"), inventory.Map{}, t0, Config{}, "asus")
	if !eq(types(evs), "firmware_update") || evs[0].Data["available"] != "2.0" {
		t.Errorf("got %v %+v", types(evs), evs)
	}
}

func TestNodes(t *testing.T) {
	n := func(nodes ...routerkit.Node) routerkit.Snapshot { return routerkit.Snapshot{Nodes: nodes} }
	a := routerkit.Node{MAC: "c8:00:00:00:00:01", Name: "Office", Online: true}
	b := routerkit.Node{MAC: "c8:00:00:00:00:02", Name: "Attic", Online: true}
	bOff := b
	bOff.Online = false
	seq := []struct {
		snap routerkit.Snapshot
		want []string
	}{
		{n(a, b), nil},
		{n(a, bOff), []string{"node_offline"}},
		{n(a, b), []string{"node_online"}},
		{n(b), []string{"node_offline"}}, // a vanished
		{n(b), nil},
		{n(a, b), []string{"node_online"}},
	}
	st := State{}
	for i, s := range seq {
		var evs []Event
		st, evs = Diff(st, s.snap, inventory.Map{}, t0, Config{}, "asus")
		if !eq(types(evs), s.want...) {
			t.Fatalf("step %d: got %v, want %v", i, types(evs), s.want)
		}
	}
}

func TestDiffIsPure(t *testing.T) {
	prev := State{Clients: map[string]ClientState{phone: {Reported: true, Observed: true, Since: t0}}, Nodes: map[string]bool{}}
	inv := inventory.Map{phone: {MAC: phone, Known: true}}
	snap := routerkit.Snapshot{Clients: []routerkit.Client{cl(phone, "", false)}}
	next, _ := Diff(prev, snap, inv, t0.Add(time.Hour), Config{}, "")
	if !prev.Clients[phone].Observed {
		t.Error("Diff mutated the previous state")
	}
	if next.Clients[phone].Reported {
		t.Error("next state should be offline (grace 0)")
	}
	// Same input, same output
	a, ea := Diff(prev, snap, inv, t0.Add(time.Hour), Config{}, "")
	b, eb := Diff(prev, snap, inv, t0.Add(time.Hour), Config{}, "")
	if len(ea) != len(eb) || a.Clients[phone] != b.Clients[phone] {
		t.Error("Diff is not deterministic")
	}
}

func TestSummaries(t *testing.T) {
	for _, e := range []Event{
		{Type: WANDown, Device: "asus"},
		{Type: WANIPChanged, Device: "asus", Data: map[string]any{"old_ip": "1.1.1.1", "new_ip": "2.2.2.2"}},
		{Type: FirmwareUpdate, Data: map[string]any{"current": "1", "available": "2"}},
		{Type: NodeOffline, Data: map[string]any{"mac": "m", "name": "Office"}},
	} {
		if s := e.Describe(); s == "" || strings.Contains(s, "<nil>") {
			t.Errorf("%s: %q", e.Type, s)
		}
	}
	sel := func(types ...string) Selector { return NewSelector(types, nil, nil, false) }
	if !sel("wan_down").Match(Event{Type: WANDown}) || sel("wan_down").Match(Event{Type: WANUp}) || !sel().Match(Event{Type: WANUp}) {
		t.Error("filter")
	}
}

func TestSelector(t *testing.T) {
	newDev := Event{Type: DeviceNew, Device: "asus", Data: map[string]any{"mac": "aa:bb:cc:00:00:01", "known": false}}
	known := Event{Type: DeviceOnline, Device: "asus", Data: map[string]any{"mac": "aa:bb:cc:00:00:02", "known": true}}
	wan := Event{Type: WANDown, Device: "zte", Data: map[string]any{}}
	cases := []struct {
		name string
		sel  Selector
		want []bool // newDev, known, wan
	}{
		{"empty", Selector{}, []bool{true, true, true}},
		{"devices", NewSelector(nil, []string{"zte"}, nil, false), []bool{false, false, true}},
		{"macs normalized", NewSelector(nil, nil, []string{"AA-BB-CC-00-00-02"}, false), []bool{false, true, false}},
		{"unknown only", NewSelector(nil, nil, nil, true), []bool{true, false, false}},
		{"types and device", NewSelector([]string{"device_new", " wan_down "}, []string{"asus"}, nil, false), []bool{true, false, false}},
	}
	for _, c := range cases {
		for i, e := range []Event{newDev, known, wan} {
			if got := c.sel.Match(e); got != c.want[i] {
				t.Errorf("%s: event %s: got %v", c.name, e.Type, got)
			}
		}
	}
	if err := NewSelector([]string{"nope"}, nil, nil, false).Validate(); err == nil {
		t.Error("unknown type accepted")
	}
	if err := NewSelector([]string{"action_done"}, nil, nil, false).Validate(); err != nil {
		t.Error(err)
	}
}
