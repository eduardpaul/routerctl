package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
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
