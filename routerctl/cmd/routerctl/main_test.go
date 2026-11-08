package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o := fs.String("o", "table", "")
	y := fs.Bool("yes", false, "")
	pos, err := parse(fs, []string{"led", "-o", "json", "state=off", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pos, " ") != "led state=off" || *o != "json" || !*y {
		t.Errorf("pos=%v o=%s yes=%v", pos, *o, *y)
	}
}

func TestAdHocHostWithEnvPassword(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ROUTERCTL_CONFIG", "")
	t.Setenv("ROUTERCTL_PASSWORD", "envpw")
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	df := addDevFlags(fs, true)
	if _, err := parse(fs, []string{"--host", "10.0.0.1", "--driver", "fake"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := df.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	d, err := df.selected(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if d.Host != "10.0.0.1" || d.Password.Value() != "envpw" || d.Driver != "fake" || d.Username != "admin" || len(cfg.Devices) != 1 {
		t.Errorf("device: %+v (%d devices)", d, len(cfg.Devices))
	}
}

func TestDeviceFromConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	os.WriteFile(p, []byte("devices:\n  - {name: a, driver: fake, host: x}\n  - {name: b, driver: fake, host: y}\n"), 0o600)
	t.Setenv("ROUTERCTL_CONFIG", p)
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	df := addDevFlags(fs, true)
	parse(fs, []string{"-d", "b", "--user", "u"})
	cfg, err := df.loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	d, err := df.selected(cfg)
	if err != nil || d.Host != "y" || d.Username != "u" {
		t.Errorf("selected %+v, %v", d, err)
	}
}

func TestVersionFrom(t *testing.T) {
	rev := []debug.BuildSetting{{Key: "vcs.revision", Value: "d754f44abcdef"}}
	cases := []struct {
		name, ldflags string
		bi            *debug.BuildInfo
		want          string
	}{
		{"release build", "v0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "v0.1.0"},
		{"go install @version", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"source build", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: rev}, "dev-d754f44"},
		{"no build info", "dev", nil, "dev"},
	}
	for _, c := range cases {
		if got := versionFrom(c.ldflags, c.bi); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
