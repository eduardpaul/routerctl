package ztedriver

import (
	"bytes"
	"io/fs"
	"net"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eduardpaul/routerctl/netkit/httprec"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/brands/zte/ztefake"
	"github.com/eduardpaul/routerctl/routerkit/drivertest"
)

// sessionRedact is how ZTE sessions are recorded: the login form carries
// the password digest, the user and the session token.
var sessionRedact = httprec.Redact{Form: []string{"Password", "Username", "_sessionTOKEN"}}

// closedAddr returns a local address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

// homepages are the positive fingerprint fixtures (the F8648P login page).
func homepages(t *testing.T) fs.FS {
	b, err := fs.ReadFile(ztefake.FixtureFS(), "login_page.html")
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{"zte-F8648P.html": {Data: b}}
}

// TestConformance runs the suite against the protocol fake (login,
// lockout, sessions, fault injection).
func TestConformance(t *testing.T) {
	var last *ztefake.Server
	newFake := func(t *testing.T, pass string, mutate func(*ztefake.Server)) routerkit.Driver {
		t.Helper()
		f := ztefake.New("user", "pw", ztefake.FixtureFS())
		f.Files["wlan_client_stat_lua.lua"] = "wlan_client_stat_synthetic.xml"
		if mutate != nil {
			mutate(f)
		}
		srv := httptest.NewServer(f)
		t.Cleanup(srv.Close)
		last = f
		d, err := New(routerkit.Config{Host: srv.URL, Username: "user", Password: pass})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	drivertest.Run(t, drivertest.Harness{
		Factory: Factory(),
		New:     func(t *testing.T) routerkit.Driver { return newFake(t, "pw", nil) },
		Unreachable: func(t *testing.T) routerkit.Driver {
			d, err := New(routerkit.Config{Host: closedAddr(t), Username: "user", Password: "pw", Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		BadAuth: func(t *testing.T, password string) routerkit.Driver { return newFake(t, password, nil) },
		PartFailure: func(t *testing.T) (routerkit.Driver, routerkit.Part, routerkit.Part) {
			d := newFake(t, "pw", func(f *ztefake.Server) { f.FailTag["wan_internetstatus_lua.lua"] = true })
			return d, routerkit.PartWAN, routerkit.PartSystem
		},
		Requests:  func() int { return last.Stats().Requests },
		Homepages: homepages(t),
		Expect: drivertest.Expect{
			Variant:  "lua",
			NonEmpty: []routerkit.Part{routerkit.PartClients, routerkit.PartWAN, routerkit.PartSystem, routerkit.PartTraffic, routerkit.PartFirmware},
		},
	})
}

// TestConformanceReplay runs the suite against the sanitized recorded
// session of an F8648P and compares the snapshot with the golden file.
func TestConformanceReplay(t *testing.T) {
	ex, err := httprec.Read(bytes.NewReader(ztefake.Session()))
	if err != nil {
		t.Fatal(err)
	}
	var last *httprec.Replayer
	drivertest.Run(t, drivertest.Harness{
		Factory: Factory(),
		New: func(t *testing.T) routerkit.Driver {
			last = httprec.NewReplayer(ex, sessionRedact)
			d, err := NewWithOptions(routerkit.Config{Host: "192.0.2.1", Username: "user", Password: "pw"}, Options{Transport: last})
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		Requests:  func() int { return last.Requests() },
		Homepages: homepages(t),
		Expect: drivertest.Expect{
			Variant:  "lua",
			NonEmpty: []routerkit.Part{routerkit.PartClients, routerkit.PartWAN, routerkit.PartSystem, routerkit.PartTraffic, routerkit.PartFirmware},
			// the capture has wired clients only
			Unset:  []string{"clients.rssi", "clients.rx_rate", "clients.tx_rate", "clients.since"},
			Golden: "testdata/F8648P/snapshot.golden.json",
		},
	})
}
