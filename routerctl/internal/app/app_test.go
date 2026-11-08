package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/config"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/fake"
)

func TestPollerDevice(t *testing.T) {
	d := config.Device{
		Name: "home", Timeout: config.Duration(7 * time.Second),
		Intervals: map[string]config.Duration{"wan": config.Duration(time.Minute), "nodes": 0},
	}
	f := fake.New()
	got := PollerDevice(d, f)
	if got.Name != "home" || got.Driver != routerkit.Driver(f) || got.Timeout != 7*time.Second {
		t.Errorf("PollerDevice = %+v", got)
	}
	for _, p := range routerkit.AllParts {
		if want := d.Interval(p); got.Intervals[p] != want {
			t.Errorf("interval %s = %s, want %s", p, got.Intervals[p], want)
		}
	}
	if _, ok := got.Intervals[routerkit.PartNodes]; ok {
		t.Error("a disabled part must not be polled")
	}
}

func TestMQTT(t *testing.T) {
	off := false
	c := config.MQTT{
		Broker: "tcp://mqtt:1883", Username: "u", Password: "pw", BaseTopic: "rc",
		DiscoveryPrefix: "homeassistant", Trackers: "all", QoS: 1, BlockSwitches: true,
	}
	o, b := MQTT(c, commands.DefaultPolicy, "v1")
	if !o.Commands || o.DiscoveryPrefix != "homeassistant" || o.Topics.Base != "rc" || o.QoS != 1 ||
		o.Trackers != "all" || !o.BlockSwitches || o.Version != "v1" {
		t.Errorf("options = %+v", o)
	}
	if b.URL != c.Broker || b.Password != "pw" || b.WillTopic != "rc/status" {
		t.Errorf("broker = %+v", b)
	}
	if s := fmt.Sprintf("%v %+v %#v", b, b, b); strings.Contains(s, "pw") {
		t.Errorf("broker formatting leaks the password: %s", s)
	}
	c.Discovery, c.Commands = &off, &off
	if o, _ = MQTT(c, commands.DefaultPolicy, "v1"); o.DiscoveryPrefix != "" || o.Commands {
		t.Errorf("discovery/commands off: %+v", o)
	}
}

func TestNotify(t *testing.T) {
	n := Notify(config.Notify{Type: "ntfy", URL: "https://ntfy.sh", Topic: "t", Token: "tok", Timeout: config.Duration(3 * time.Second), Events: []string{"device_new"}})
	if n.Token != "tok" || n.Timeout != 3*time.Second || n.Topic != "t" || len(n.Events) != 1 {
		t.Errorf("Notify = %+v", n)
	}
	if s := fmt.Sprintf("%v %+v %#v", n, n, n); strings.Contains(s, "tok") {
		t.Errorf("formatting leaks the token: %s", s)
	}
}

func TestOpenDevice(t *testing.T) {
	reg, err := Registry(fake.Factory())
	if err != nil {
		t.Fatal(err)
	}
	drv, err := OpenDevice(reg, config.Device{Name: "lab", Driver: "fake"})
	if err != nil || drv.Info().Name != "fake" {
		t.Fatalf("fake: %v, %v", drv, err)
	}
	if _, err := OpenDevice(reg, config.Device{Name: "lab", Driver: "nope"}); err == nil || !strings.HasPrefix(err.Error(), `lab: unknown driver "nope"`) {
		t.Errorf("unknown driver: %v", err)
	}
	drv, err = OpenDevice(reg, config.Device{Name: "lab", Driver: "auto"})
	if err != nil || drv.Info().Name != "auto" {
		t.Errorf("auto: %v, %v", drv, err)
	}
	if _, err := Registry(fake.Factory(), fake.Factory()); err == nil {
		t.Error("duplicate factories accepted")
	}
}

func TestCheckDevice(t *testing.T) {
	reg, err := Registry(fake.Factory(), routerkit.Factory{
		Info:    routerkit.DriverInfo{Name: "zz"},
		Options: []routerkit.OptionSpec{{Name: "cache", Type: routerkit.ArgDuration}},
		New:     fake.Factory().New,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dev  config.Device
		want string // "" = valid
	}{
		{config.Device{Name: "a", Driver: "fake", Options: map[string]string{"demo": "true"}}, ""},
		{config.Device{Name: "a", Driver: "fake", Options: map[string]string{"demo": "maybe"}}, `device a: driver fake: option demo="maybe" is not on/off`},
		{config.Device{Name: "a", Driver: "fake", Options: map[string]string{"cache": "1s"}}, "device a: driver fake: unknown option(s) cache (accepted: demo)"},
		{config.Device{Name: "a", Driver: "nope"}, `device a: unknown driver "nope"`},
		{config.Device{Name: "a", Driver: "auto", Options: map[string]string{"cache": "1s", "demo": "on"}}, ""},
		{config.Device{Name: "a", Driver: "auto", Options: map[string]string{"cache": "soon"}}, `device a: zz: option cache="soon" is not a duration`},
		{config.Device{Name: "a", Driver: "auto", Options: map[string]string{"typo": "1"}}, `device a: unknown option "typo" (no driver accepts it)`},
	} {
		err := CheckDevice(reg, tc.dev)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%+v: %v", tc.dev, err)
		case tc.want != "" && (err == nil || !strings.HasPrefix(err.Error(), tc.want)):
			t.Errorf("%+v: got %v, want %s", tc.dev, err, tc.want)
		}
	}
}

func TestPolicy(t *testing.T) {
	p := Policy(config.Commands{MaxDanger: "destructive", Deny: []string{"led"}}, "safe")
	if p.MaxDanger != routerkit.Safe || len(p.Deny) != 1 {
		t.Errorf("stricter transport: %+v", p)
	}
	if p := Policy(config.Commands{MaxDanger: "safe"}, "destructive"); p.MaxDanger != routerkit.Safe {
		t.Errorf("a transport cannot loosen the shared policy: %+v", p)
	}
	if p := Policy(config.Commands{}, ""); p.MaxDanger != routerkit.Disruptive {
		t.Errorf("default: %+v", p)
	}
}

func TestSinksHTTP(t *testing.T) {
	cfg := &config.Config{API: config.API{Enabled: true}, HTTP: config.HTTP{Listen: "127.0.0.1:0", Token: "t"}}
	sinks, err := Sinks(cfg, "v", nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range sinks {
		names = append(names, s.Name())
	}
	// metrics (health), the API, then the server last
	if strings.Join(names, ",") != "metrics,api,http" {
		t.Errorf("sinks: %v", names)
	}
}
