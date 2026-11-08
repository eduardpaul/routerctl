package asus

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	asusrouter "github.com/eduardpaul/routerctl/asusrouter"
	"github.com/eduardpaul/routerctl/asusrouter/parity/replay"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/asus/normalize"
)

func TestFingerprint(t *testing.T) {
	for page, want := range map[string]struct {
		score int
		model string
	}{
		`<script>top.location.href='/Main_Login.asp';</script>`:            {40, ""},
		`<form action="login.cgi" method="post"><title>ASUS Login</title>`: {90, ""},
		`<title>ASUS Wireless Router RT-AX88U Pro - Login</title>`:         {70, "RT-AX88U Pro"},
		`<form action="/login.cgi">`:                                       {routerkit.ScoreWeak, ""},
		`<title>F8648P</title><script src="/js/zte.js"></script>`:          {0, ""},
		``: {0, ""},
	} {
		m, ok := Fingerprint(routerkit.HomeEvidence(page))
		if ok != (want.score > 0) || m.Score != want.score || m.Model != want.model {
			t.Errorf("Fingerprint(%q) = %+v, %v; want %+v", page, m, ok, want)
		}
	}
	// a redirect to the login page counts
	ev := routerkit.HomeEvidence("")
	p := ev.Pages["/"]
	p.Redirects = []string{"http://192.168.50.1/Main_Login.asp"}
	ev.Pages["/"] = p
	if m, ok := Fingerprint(ev); !ok || m.Score != 40 {
		t.Errorf("redirect: %+v %v", m, ok)
	}
	if f := Factory(); f.Info.Name != "asus" || f.New == nil || f.Record == nil || f.Replay == nil {
		t.Fatalf("Factory = %+v", f)
	}
}

func TestOptions(t *testing.T) {
	f := Factory()
	if _, err := f.New(routerkit.Config{Host: "x", Options: map[string]string{"cache": "soon"}}); err == nil || !strings.Contains(err.Error(), `cache="soon" is not a duration`) {
		t.Errorf("bad cache: %v", err)
	}
	if _, err := f.New(routerkit.Config{Host: "x", Options: map[string]string{"cahce": "1s"}}); err == nil {
		t.Error("unknown option accepted")
	}
	d, err := NewWithOptions(routerkit.Config{Host: "x", Options: map[string]string{"cache": "2s"}}, Options{Transport: &asusrouter.ReplayTransport{Session: replay.NewSession(nil)}})
	if err != nil || d.cache != 2*time.Second {
		t.Errorf("cache option: %v %v", d, err)
	}
}

func TestResolveSource(t *testing.T) {
	for in, want := range map[string]string{"ARWanSource": "ARWanSource", "wan": "ARWanSource", "TEMPERATURE": "ARTemperatureSource", "nope": ""} {
		if got := resolveSource(in); got != want {
			t.Errorf("resolveSource(%q) = %q", in, got)
		}
	}
}

func TestTruthy(t *testing.T) {
	for v, want := range map[any]bool{nil: false, true: true, false: false, "": false, "x": true, int64(0): false, int64(2): true} {
		if truthy(v) != want {
			t.Errorf("truthy(%v) != %v", v, want)
		}
	}
	if truthy(map[string]any{"success": false, "x": 1}) || !truthy(map[string]any{"a": 1}) || truthy(map[string]any{}) || truthy([]any{}) {
		t.Error("truthy of maps / lists")
	}
}

func TestActionsSpec(t *testing.T) {
	d, err := NewWithOptions(routerkit.Config{Host: "x"}, Options{Transport: &asusrouter.ReplayTransport{Session: replay.NewSession(nil)}})
	if err != nil {
		t.Fatal(err)
	}
	do := func(name string, raw map[string]string) error {
		_, err := d.Do(context.Background(), routerkit.Invocation{Name: name, Args: routerkit.RawArgs(raw)})
		return err
	}
	if err := do("reboot", map[string]string{"now": "1"}); !errors.Is(err, routerkit.ErrInvalidArgs) {
		t.Errorf("unknown argument: %v", err)
	}
	if err := do("restart-service", map[string]string{"name": "restart_anything"}); !errors.Is(err, routerkit.ErrInvalidArgs) {
		t.Errorf("restart-service outside the allow list: %v", err)
	}
	if err := do("nope", nil); !errors.Is(err, routerkit.ErrUnsupported) {
		t.Errorf("unknown action: %v", err)
	}
	caps, err := d.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := caps.Action("wifi")
	if len(spec.Args[0].Choices) == 0 {
		t.Error("wifi bands missing")
	}
}

// Recorded sessions (parity/local, git-ignored): the normalized model
// must match the Python goldens run through the same mapping.
func TestReplaySessions(t *testing.T) {
	devs := replay.LocalDevices()
	if len(devs) == 0 {
		t.Skip("no recorded sessions")
	}
	for _, dir := range devs {
		golden, err := replay.LoadGolden(filepath.Join(dir, "py.json"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := golden["ARDeviceSource"]; !ok {
			continue // not an ASUS session
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			rt, err := asusrouter.NewReplayTransport(filepath.Join(dir, "session.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewWithOptions(routerkit.Config{Host: "replay"}, Options{Transport: rt})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := d.Connect(ctx); err != nil {
				t.Fatal(err)
			}
			id, err := d.Identity(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want := normalize.Identity(golden["ARDeviceSource"])
			want.Variant = "stock" // both sessions run stock firmware
			if id != want {
				t.Errorf("identity = %+v, want %+v", id, want)
			}
			snap, err := d.Poll(ctx, routerkit.Poll(routerkit.PartWAN))
			if err != nil {
				t.Fatal(err)
			}
			wantWAN := normalize.WAN(golden["ARWanSource"])
			gotJ, _ := json.Marshal(snap.WAN)
			wantJ, _ := json.Marshal(wantWAN)
			if !reflect.DeepEqual(gotJ, wantJ) {
				t.Errorf("wan = %s, want %s", gotJ, wantJ)
			}
			caps, err := d.Capabilities(ctx)
			if err != nil || len(caps.Sources) == 0 || !caps.Supports(routerkit.PartNodes) {
				t.Errorf("capabilities %+v %v", caps, err)
			}
		})
	}
}

// The generic replay hook opens the same sessions as the replay command.
func TestFactoryReplay(t *testing.T) {
	devs := replay.LocalDevices()
	if len(devs) == 0 {
		t.Skip("no recorded sessions")
	}
	drv, err := Factory().Replay(filepath.Join(devs[0], "session.jsonl"), routerkit.Config{Host: "replay"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := drv.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer drv.Close(ctx)
	if v, err := drv.Get(ctx, "ARWanSource"); err != nil || v == nil {
		t.Errorf("Get ARWanSource = %v, %v", v, err)
	}
	if _, err := Factory().Replay(filepath.Join(t.TempDir(), "missing.jsonl"), routerkit.Config{}); err == nil {
		t.Error("replay of a missing file succeeded")
	}
}
