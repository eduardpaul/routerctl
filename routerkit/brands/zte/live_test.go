package zte

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

// TestLive reads every typed method once from a real device, then logs
// out. Read-only. Enable with:
//
//	ROUTERCTL_LIVE=zte ROUTERCTL_LIVE_HOST=192.168.1.1 ROUTERCTL_LIVE_USER=user ROUTERCTL_LIVE_PASS=... \
//	  go test ./routerkit/brands/zte -run TestLive -v
//
// ROUTERCTL_LIVE_OUT=<file> also writes the results as JSON (it contains MACs,
// serials and addresses: keep it out of git).
func TestLive(t *testing.T) {
	env := func(k string) string { return os.Getenv("ROUTERCTL_LIVE_" + k) }
	if os.Getenv("ROUTERCTL_LIVE") != "zte" {
		t.Skip("set ROUTERCTL_LIVE=zte and the device credentials to run against a device")
	}
	host := env("HOST")
	if host == "" {
		host = "192.168.1.1"
	}
	c, err := New(host, env("USER"), env("PASS"), &Options{
		MinInterval:        500 * time.Millisecond,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := c.Login(ctx); err != nil {
		t.Fatalf("login: %v", err) // stop: no retry, to avoid a lockout
	}
	defer func() {
		if err := c.Logout(context.Background()); err != nil {
			t.Errorf("logout: %v", err)
		} else {
			t.Log("logged out")
		}
	}()
	out := map[string]any{}
	check := func(name string, v any, err error) {
		t.Helper()
		if err != nil {
			if errors.Is(err, ErrNotAvailable) {
				t.Logf("%s: not available (%v)", name, err)
				out[name] = err.Error()
				return
			}
			t.Errorf("%s: %v", name, err)
			return
		}
		out[name] = v
	}

	d, err := c.DeviceInfo(ctx)
	check("device_info", d, err)
	t.Logf("device: %s %s hw %s, up %s", d.Model, d.SoftwareVersion, d.HardwareVersion, d.Uptime)
	s, err := c.System(ctx)
	check("system", s, err)
	t.Logf("system: cpu %v avg %.1f%%, mem %v", s.CPUUsage, s.CPUAverage, deref(s.MemUsage))
	w, err := c.WANStatus(ctx)
	check("wan", w, err)
	for _, x := range w.Connections {
		t.Logf("wan %-20s %-6s %-9s defgw=%v up=%s", x.Name, x.Protocol, x.Status, x.DefaultGW, x.Uptime)
	}
	hs, err := c.Clients(ctx)
	check("clients", hs, err)
	t.Logf("clients: %d", len(hs))
	wf, err := c.WiFi(ctx)
	check("wifi", wf, err)
	for _, r := range wf.Radios {
		on := 0
		for _, s := range r.SSIDs {
			if s.Enabled {
				on++
			}
		}
		t.Logf("radio %s enabled=%v channel=%d auto=%v ssids=%d (enabled %d)", r.Band, r.Enabled, r.Channel, r.AutoChannel, len(r.SSIDs), on)
	}
	p, err := c.PON(ctx)
	check("pon", p, err)
	tr, err := c.Traffic(ctx)
	check("traffic", tr, err)
	t.Logf("traffic: %d ports, %d wan, %d ssids", len(tr.Ports), len(tr.WAN), len(tr.SSIDs))
	tp, err := c.Topology(ctx)
	check("topology", tp, err)
	to, err := c.SessionTimeout(ctx)
	check("session_timeout", to.String(), err)
	t.Logf("session idle timeout: %s", to)
	for _, src := range Sources {
		r, err := c.Get(ctx, src.Name)
		if err != nil {
			if errors.Is(err, ErrNotAvailable) {
				t.Logf("get %-22s not available", src.Name)
				continue
			}
			t.Errorf("get %s: %v", src.Name, err)
			continue
		}
		t.Logf("get %-22s ok, objects %v", src.Name, r.ObjectNames())
	}
	if f := env("OUT"); f != "" {
		b, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(f, b, 0o600); err != nil {
			t.Error(err)
		}
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
