package asus

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	asusrouter "github.com/eduardpaul/routerctl/asusrouter"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivertest"
)

// sessionDir is the recorded RT-AX88U Pro session the conformance suite
// replays. It is not committed: it lives in the git-ignored parity/local
// (or, once a fully synthetic one exists, parity/fixtures), with the
// golden snapshot next to it.
const sessionDir = "rt-ax88u-pro_stock_3.0.0.6.102"

// findSession returns the session directory under root, or "".
func findSession(root string) string {
	for _, base := range []string{"parity/local", "parity/fixtures"} {
		dir := filepath.Join(root, base, sessionDir)
		if _, err := os.Stat(filepath.Join(dir, "session.jsonl")); err == nil {
			return dir
		}
	}
	return ""
}

// counting counts the queries a replayed driver sends.
type counting struct {
	*asusrouter.ReplayTransport
	n *atomic.Int64
}

func (c counting) Query(ctx context.Context, ep endpoint.AREndpoint, payload *string, rt consts.RequestType) (asusrouter.Response, error) {
	c.n.Add(1)
	return c.ReplayTransport.Query(ctx, ep, payload, rt)
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// repoRoot is the checkout this test runs in: the nearest directory above
// the working directory (the package directory) that holds
// parity/check.sh, or "" outside a checkout (the module cache, where the
// fixtures are not shipped). It does not use replay.RepoRoot, which looks
// above the asusrouter sources: those are in the module cache once
// routerkit builds against a released asusrouter.
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "parity", "check.sh")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

func TestConformance(t *testing.T) {
	root := repoRoot()
	if root == "" {
		t.Skip("not in a routerctl checkout: the parity fixtures are not available")
	}
	dir := findSession(root)
	if dir == "" {
		t.Skipf("no recorded %s session under parity/local or parity/fixtures", sessionDir)
	}
	path := filepath.Join(dir, "session.jsonl")
	var n atomic.Int64
	drivertest.Run(t, drivertest.Harness{
		Factory: Factory(),
		New: func(t *testing.T) routerkit.Driver {
			rt, err := asusrouter.NewReplayTransport(path)
			if err != nil {
				t.Fatal(err)
			}
			n.Store(0)
			d, err := NewWithOptions(routerkit.Config{Host: "replay"}, Options{Transport: counting{rt, &n}})
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		Unreachable: func(t *testing.T) routerkit.Driver {
			d, err := New(routerkit.Config{Host: "127.0.0.1", Port: closedPort(t), Username: "admin", Password: "pw", Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		BadAuth: func(t *testing.T, password string) routerkit.Driver {
			// An ASUSWRT web server refusing every login (error_status 3)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"error_status":"3"}`))
			}))
			t.Cleanup(srv.Close)
			host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
			p, _ := strconv.Atoi(port)
			d, err := New(routerkit.Config{Host: host, Port: p, Username: "admin", Password: password, Timeout: 2 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			return d
		},
		Requests:  func() int { return int(n.Load()) },
		Homepages: os.DirFS("testdata/homepages"),
		Expect: drivertest.Expect{
			NonEmpty: []routerkit.Part{routerkit.PartClients, routerkit.PartWAN, routerkit.PartSystem, routerkit.PartTraffic, routerkit.PartNodes, routerkit.PartFirmware},
			// no firmware update was pending when the session was recorded
			Unset:  []string{"firmware.available"},
			Golden: filepath.Join(dir, "snapshot.golden.json"),
		},
	})
}
