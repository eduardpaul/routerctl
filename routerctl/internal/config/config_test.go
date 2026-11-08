package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestExampleParses(t *testing.T) {
	c, err := Parse([]byte(Example))
	if err != nil {
		t.Fatal(err)
	}
	// Secret files do not exist here: drop them before Finish
	for i := range c.Devices {
		c.Devices[i].PasswordFile = ""
	}
	c.MQTT.PasswordFile = ""
	for i := range c.Notify {
		c.Notify[i].TokenFile = ""
		if c.Notify[i].Type == "gotify" {
			c.Notify[i].Token = "x"
		}
	}
	if err := c.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(c.Devices) != 2 || c.Devices[0].Name != "asus" || c.Devices[1].Driver != "zte" {
		t.Fatalf("devices: %+v", c.Devices)
	}
	if got := c.Devices[0].Interval(routerkit.PartNodes); got != 2*time.Minute {
		t.Errorf("nodes interval = %v", got)
	}
	if got := c.Devices[1].Interval(routerkit.PartTraffic); got != 0 {
		t.Errorf("disabled traffic interval = %v", got)
	}
	if got := c.Devices[1].Interval(routerkit.PartWAN); got != DefaultIntervals[routerkit.PartWAN] {
		t.Errorf("default wan interval = %v", got)
	}
	if c.Presence.OfflineGrace.D() != 3*time.Minute {
		t.Errorf("grace = %v", c.Presence.OfflineGrace.D())
	}
	if !c.MQTT.Enabled || c.MQTT.Trackers != "known" || c.MQTT.MaxDanger != "" {
		t.Errorf("mqtt: %+v", c.MQTT)
	}
	if len(c.Notify) != 3 || c.Notify[0].Events[0] != "device_new" {
		t.Errorf("notify: %+v", c.Notify)
	}
}

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte("devices:\n  - host: 10.0.0.1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Finish(); err != nil {
		t.Fatal(err)
	}
	d := c.Devices[0]
	if d.Name != "router" || d.Driver != "auto" || d.Timeout.D() != DefaultTimeout {
		t.Errorf("device defaults: %+v", d)
	}
	if c.StateFile != DefaultStateFile || c.MQTT.BaseTopic != "routerctl" || c.MQTT.DiscoveryPrefix != "homeassistant" || c.HTTP.Listen != "127.0.0.1:9100" || c.Commands.MaxDanger != "disruptive" {
		t.Errorf("defaults: %+v", c)
	}
	if !c.MQTT.DiscoveryEnabled() || !c.MQTT.CommandsEnabled() || !c.Inventory.BaselineEnabled() || !c.Metrics.PerClientEnabled() {
		t.Error("bool defaults should be true")
	}
}

func TestDurations(t *testing.T) {
	c, err := Parse([]byte("presence:\n  offline_grace: 90\n  online_debounce: 1m30s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Presence.OfflineGrace.D() != 90*time.Second || c.Presence.OnlineDebounce.D() != 90*time.Second {
		t.Errorf("got %v / %v", c.Presence.OfflineGrace.D(), c.Presence.OnlineDebounce.D())
	}
	if _, err := Parse([]byte("presence:\n  offline_grace: soon\n")); err == nil {
		t.Error("want an error for a bad duration")
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	if _, err := Parse([]byte("devices:\n  - host: x\n    pasword: typo\n")); err == nil {
		t.Fatal("want an error for an unknown field")
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]string{
		"no host":         "devices:\n  - name: a\n",
		"dup names":       "devices:\n  - {name: a, host: x}\n  - {name: a, host: y}\n",
		"bad name":        "devices:\n  - {name: 'a b', host: x}\n",
		"bad part":        "devices:\n  - host: x\n    intervals: {cpu: 1s}\n",
		"mqtt no broker":  "mqtt:\n  enabled: true\n",
		"bad trackers":    "mqtt:\n  trackers: some\n",
		"bad notify type": "notify:\n  - {type: slack, url: http://x}\n",
		"gotify no token": "notify:\n  - {type: gotify, url: http://x}\n",
		"bad log level":   "log:\n  level: loud\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			c, err := Parse([]byte(doc))
			if err != nil {
				return // a parse error is fine too
			}
			if err := c.Finish(); err == nil {
				t.Fatalf("want a validation error")
			}
		})
	}
}

func TestEnvSingleDevice(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw")
	if err := os.WriteFile(pw, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load("", false, env(map[string]string{
		"ROUTERCTL_HOST":          "192.168.50.1",
		"ROUTERCTL_USERNAME":      "admin",
		"ROUTERCTL_PASSWORD_FILE": pw,
		"ROUTERCTL_DRIVER":        "asus",
		"ROUTERCTL_SSL":           "true",
		"ROUTERCTL_PORT":          "8443",
		"ROUTERCTL_MQTT_BROKER":   "tcp://mqtt:1883",
		"ROUTERCTL_NTFY_URL":      "https://ntfy.sh",
		"ROUTERCTL_NTFY_TOPIC":    "home",
		"ROUTERCTL_NTFY_EVENTS":   "device_new, wan_down",
		"ROUTERCTL_STATE_FILE":    "/data/s.json",
	}))
	if err != nil {
		t.Fatal(err)
	}
	d := c.Devices[0]
	if d.Host != "192.168.50.1" || d.Password.Value() != "s3cret" || !d.SSL || d.Port != 8443 || d.Driver != "asus" {
		t.Errorf("device: %+v", d)
	}
	if !c.MQTT.Enabled || c.MQTT.Broker != "tcp://mqtt:1883" {
		t.Errorf("mqtt: %+v", c.MQTT)
	}
	if len(c.Notify) != 1 || len(c.Notify[0].Events) != 2 || c.Notify[0].Events[1] != "wan_down" {
		t.Errorf("notify: %+v", c.Notify)
	}
	if c.StateFile != "/data/s.json" {
		t.Errorf("state file %q", c.StateFile)
	}
}

func TestEnvOverridesFileDevice(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte("devices:\n  - {name: asus, host: 10.0.0.1, username: u}\n  - {name: ont, host: 10.0.0.2}\n"), 0o600)
	c, err := Load(p, true, env(map[string]string{"ROUTERCTL_NAME": "ont", "ROUTERCTL_PASSWORD": "pw"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Devices[1].Password.Value() != "pw" || c.Devices[0].Password != "" {
		t.Errorf("env applied to the wrong device: %+v", c.Devices)
	}
	if _, err := c.Device(""); err == nil {
		t.Error("want an error picking among several devices")
	}
	if d, err := c.Device("asus"); err != nil || d.Host != "10.0.0.1" {
		t.Errorf("Device(asus) = %v, %v", d, err)
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), true, env(nil)); err == nil {
		t.Error("want an error for a missing required file")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), false, env(nil)); err != nil {
		t.Errorf("optional missing file: %v", err)
	}
}

func TestSecretsNeverPrinted(t *testing.T) {
	c, _ := Parse([]byte("devices:\n  - {host: x, password: hunter2}\nmqtt:\n  password: mqttpw\nnotify:\n  - {type: ntfy, url: http://x, topic: t, token: tok123}\n"))
	if err := c.Finish(); err != nil {
		t.Fatal(err)
	}
	dump := c.Redacted()
	js, _ := json.Marshal(c)
	for _, s := range []string{"hunter2", "mqttpw", "tok123"} {
		if strings.Contains(dump, s) || strings.Contains(string(js), s) {
			t.Errorf("secret %q leaked:\n%s\n%s", s, dump, js)
		}
	}
	if !strings.Contains(dump, "********") {
		t.Error("expected masked secrets in the dump")
	}
	if c.Devices[0].DriverConfig().Password != "hunter2" {
		t.Error("DriverConfig must carry the real password")
	}
}

func TestSecretFileErrorHidesNothing(t *testing.T) {
	c, _ := Parse([]byte("devices:\n  - {host: x, password_file: /nonexistent/secret}\n"))
	err := c.Finish()
	if err == nil || !strings.Contains(err.Error(), "password_file") {
		t.Fatalf("want a password_file error, got %v", err)
	}
}

func TestHTTPAndPolicyValidation(t *testing.T) {
	finish := func(doc string, env map[string]string) error {
		c, err := Parse([]byte(doc))
		if err != nil {
			return err
		}
		if err := c.ApplyEnv(func(k string) string { return env[k] }); err != nil {
			return err
		}
		return c.Finish()
	}
	cases := []struct {
		name string
		doc  string
		env  map[string]string
		ok   bool
	}{
		{"api on loopback without token", "api: {enabled: true}\n", nil, true},
		{"api on all interfaces without token", "api: {enabled: true}\nhttp: {listen: \":9100\"}\n", nil, false},
		{"api on all interfaces with env token", "api: {enabled: true}\n", map[string]string{"ROUTERCTL_HTTP_LISTEN": ":9100", "ROUTERCTL_HTTP_TOKEN": "t"}, true},
		{"metrics only on all interfaces", "metrics: {enabled: true}\nhttp: {listen: \":9100\"}\n", nil, true},
		{"bad listen", "http: {listen: \"9100\"}\n", nil, false},
		{"protect_metrics needs a token", "http: {protect_metrics: true}\n", nil, false},
		{"bad max_danger", "commands: {max_danger: yolo}\n", nil, false},
		{"bad mqtt max_danger", "mqtt: {max_danger: nope}\n", nil, false},
		{"stricter api", "commands: {max_danger: destructive}\napi: {enabled: true, max_danger: safe}\n", nil, true},
		{"unknown stdout event", "stdout: {events: [bogus]}\n", nil, false},
		{"allow_dangerous is gone", "mqtt: {allow_dangerous: true}\n", nil, false},
	}
	for _, c := range cases {
		if err := finish(c.doc, c.env); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	for addr, want := range map[string]bool{"127.0.0.1:9100": true, "[::1]:1": true, "localhost:9": true, ":9100": false, "0.0.0.0:9100": false, "192.168.1.5:9100": false} {
		if Loopback(addr) != want {
			t.Errorf("Loopback(%q) != %v", addr, want)
		}
	}
}
