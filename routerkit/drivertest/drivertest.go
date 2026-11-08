// Package drivertest is the conformance suite of the routerkit contract:
// the semantics every driver must have, whatever the brand. Each driver
// package calls Run from its tests with a Harness bound to its fake or to
// a recorded session:
//
//	func TestConformance(t *testing.T) {
//		drivertest.Run(t, drivertest.Harness{Factory: mydriver.Factory(), New: ...})
//	}
//
// Groups (subtests): info, lifecycle, errors, context, identity,
// capabilities, snapshot, partial, concurrency, sources, actions,
// fingerprints. A nil Harness hook skips the checks that need it. With
// Harness.Live the suite runs against a real device (see LiveConfig):
// read-only unless ROUTERCTL_LIVE_WRITE=1 allows the Safe actions; a
// Disruptive or Destructive action never runs, and the golden comparison
// is skipped.
//
// Golden snapshots are rewritten with
//
//	go test ./routerkit/drivers/<x> -run Conformance -drivertest.update
package drivertest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

var update = flag.Bool("drivertest.update", false, "rewrite the golden snapshots of the conformance suite")

// Sentinel is the password the suite hands to Harness.BadAuth; it must
// never show in errors, logs or JSON.
const Sentinel = "drivertest-S3ntinel-pw"

// Harness binds the suite to one driver.
type Harness struct {
	Factory routerkit.Factory
	// New returns a driver bound to a fake or a replay, not connected.
	// Each call returns a fresh driver.
	New func(t *testing.T) routerkit.Driver
	// Unreachable returns a driver whose device does not answer (a
	// closed port)
	Unreachable func(t *testing.T) routerkit.Driver
	// BadAuth returns a driver whose device refuses password
	BadAuth func(t *testing.T, password string) routerkit.Driver
	// PartFailure returns a driver on which part failing fails and part
	// healthy reads fine
	PartFailure func(t *testing.T) (drv routerkit.Driver, failing, healthy routerkit.Part)
	// Requests counts the device requests of the driver New returned
	// last, to prove that invalid calls cause no I/O
	Requests func() int
	Expect   Expect
	// Homepages holds positive fingerprint fixtures: home pages (one
	// file each) that must be recognized as this driver
	Homepages fs.FS
	// Live: a real device; read-only (no action runs), no golden
	Live bool
}

// Expect is what the fixture behind Harness.New holds.
type Expect struct {
	// NonEmpty parts must have data
	NonEmpty []routerkit.Part
	// Unset lists fields the driver supports but this fixture never
	// fills, as "part.field" ("wan.public_ip")
	Unset []string
	// Golden is the path of the golden snapshot ("" skips it)
	Golden string
	// Variant is what Homepages must be recognized as ("" for any)
	Variant string
	// RunSafeActions runs every Safe action and expects it accepted
	// (fakes only)
	RunSafeActions bool
}

const timeout = 30 * time.Second

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

// noPanic runs fn and reports a panic as a test error.
func noPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", what, r)
		}
	}()
	fn()
}

func connected(t *testing.T, h Harness) routerkit.Driver {
	t.Helper()
	d := h.New(t)
	if err := d.Connect(ctxT(t)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close(context.Background()) })
	return d
}

func capsOf(t *testing.T, d routerkit.Driver) routerkit.Capabilities {
	t.Helper()
	c, err := d.Capabilities(ctxT(t))
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	return c
}

// Run runs the conformance suite.
func Run(t *testing.T, h Harness) {
	t.Helper()
	if h.New == nil {
		t.Fatal("drivertest: Harness.New is required")
	}
	t.Run("info", func(t *testing.T) { testInfo(t, h) })
	t.Run("lifecycle", func(t *testing.T) { testLifecycle(t, h) })
	t.Run("errors", func(t *testing.T) { testErrors(t, h) })
	t.Run("context", func(t *testing.T) { testContext(t, h) })
	t.Run("identity", func(t *testing.T) { testIdentity(t, h) })
	t.Run("capabilities", func(t *testing.T) { testCapabilities(t, h) })
	t.Run("snapshot", func(t *testing.T) { testSnapshot(t, h) })
	t.Run("partial", func(t *testing.T) { testPartial(t, h) })
	t.Run("concurrency", func(t *testing.T) { testConcurrency(t, h) })
	t.Run("sources", func(t *testing.T) { testSources(t, h) })
	t.Run("actions", func(t *testing.T) { testActions(t, h) })
	t.Run("fingerprints", func(t *testing.T) { testFingerprints(t, h) })
}

// ---- 1. info and registry ----

func major(v string) string { return strings.SplitN(strings.TrimPrefix(v, "v"), ".", 2)[0] }

func testInfo(t *testing.T, h Harness) {
	f := h.Factory
	info := f.Info
	if !routerkit.ValidName(info.Name) || info.Name == "auto" {
		t.Errorf("Info.Name %q: want [a-z0-9-]+", info.Name)
	}
	if info.Vendor == "" || info.Help == "" {
		t.Errorf("Info.Vendor / Help empty: %+v", info)
	}
	switch info.Stability {
	case routerkit.Experimental, routerkit.Beta, routerkit.Stable:
	default:
		t.Errorf("Info.Stability %q", info.Stability)
	}
	if major(info.Contract) != major(routerkit.ContractVersion) || info.Contract == "" {
		t.Errorf("Info.Contract %q, want major of %s", info.Contract, routerkit.ContractVersion)
	}
	if err := routerkit.CheckOptionSpecs(f.Options); err != nil {
		t.Errorf("Factory.Options: %v", err)
	}
	if f.New == nil {
		t.Fatal("Factory.New is nil")
	}
	if _, err := routerkit.NewRegistry(f); err != nil {
		t.Errorf("NewRegistry: %v", err)
	}
	if _, err := f.New(routerkit.Config{Host: "192.0.2.1", Options: map[string]string{"drivertest-unknown-option": "1"}}); err == nil {
		t.Error("New accepted an unknown option")
	}
	d := h.New(t)
	if got := d.Info(); !reflect.DeepEqual(got, info) {
		t.Errorf("Driver.Info = %+v, Factory.Info = %+v", got, info)
	}
}

// ---- 2. lifecycle ----

func testLifecycle(t *testing.T, h Harness) {
	ctx := ctxT(t)
	d := h.New(t)
	before, beforeErr := d.Capabilities(ctx)
	if err := d.Close(ctx); err != nil {
		t.Errorf("Close before Connect: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := d.Connect(ctx); err != nil {
			t.Fatalf("Connect #%d: %v", i, err)
		}
	}
	if _, err := d.Identity(ctx); err != nil {
		t.Errorf("Identity: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := d.Close(ctx); err != nil {
			t.Errorf("Close #%d: %v", i, err)
		}
	}
	noPanic(t, "Identity after Close", func() {
		if _, err := d.Identity(ctx); err == nil {
			t.Error("Identity after Close: no error")
		}
	})
	noPanic(t, "Poll after Close", func() {
		if _, err := d.Poll(ctx, routerkit.Poll(routerkit.AllParts...)); err == nil {
			t.Error("Poll after Close: no error")
		}
	})
	noPanic(t, "Get after Close", func() {
		if _, err := d.Get(ctx, "drivertest-any"); err == nil {
			t.Error("Get after Close: no error")
		}
	})
	noPanic(t, "Do after Close", func() {
		if _, err := d.Do(ctx, routerkit.Invocation{Name: "drivertest-any"}); err == nil {
			t.Error("Do after Close: no error")
		}
	})
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("Connect after Close: %v", err)
	}
	caps := capsOf(t, d)
	if parts := caps.Filter(routerkit.AllParts); len(parts) > 0 {
		if _, err := d.Poll(ctx, routerkit.Poll(parts...)); err != nil {
			t.Errorf("Poll after reconnect: %v", err)
		}
	}
	if beforeErr == nil && len(before.Parts) > 0 {
		for _, p := range caps.Filter(routerkit.AllParts) {
			if !before.Supports(p) {
				t.Errorf("part %s supported after Connect but not before: the static capabilities must be a superset", p)
			}
		}
	}
	_ = d.Close(ctx)
}

// ---- 3. errors ----

// capture is an slog handler recording every message and attribute.
type capture struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (c capture) Enabled(context.Context, slog.Level) bool { return true }
func (c capture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintln(c.buf, r.Message)
	r.Attrs(func(a slog.Attr) bool { fmt.Fprintln(c.buf, a.String()); return true })
	return nil
}
func (c capture) WithAttrs(as []slog.Attr) slog.Handler {
	c.mu.Lock()
	for _, a := range as {
		fmt.Fprintln(c.buf, a.String())
	}
	c.mu.Unlock()
	return c
}
func (c capture) WithGroup(string) slog.Handler { return c }

func testErrors(t *testing.T, h Harness) {
	if h.Unreachable == nil && h.BadAuth == nil {
		t.Skip("no Unreachable / BadAuth hook")
	}
	var mu sync.Mutex
	logs := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(capture{&mu, logs}))
	defer slog.SetDefault(prev)
	var seen []string
	note := func(err error) {
		if err == nil {
			return
		}
		seen = append(seen, err.Error())
		if b, jerr := json.Marshal(routerkit.NewPartError(err)); jerr == nil {
			seen = append(seen, string(b))
		}
	}
	ctx := ctxT(t)
	if h.Unreachable != nil {
		d := h.Unreachable(t)
		err := d.Connect(ctx)
		note(err)
		if !errors.Is(err, routerkit.ErrUnreachable) {
			t.Errorf("Connect to an unreachable device = %v, want ErrUnreachable", err)
		}
		_, err = d.Poll(ctx, routerkit.Poll(routerkit.AllParts...))
		note(err)
		if !errors.Is(err, routerkit.ErrUnreachable) {
			t.Errorf("Poll of an unreachable device = %v, want ErrUnreachable", err)
		}
		if routerkit.CodeOf(err) != routerkit.CodeUnreachable {
			t.Errorf("CodeOf(%v) = %s", err, routerkit.CodeOf(err))
		}
	}
	if h.BadAuth != nil {
		d := h.BadAuth(t, Sentinel)
		err := d.Connect(ctx)
		note(err)
		if !errors.Is(err, routerkit.ErrAuth) {
			t.Errorf("Connect with bad credentials = %v, want ErrAuth", err)
		}
		_, err = d.Poll(ctx, routerkit.Poll(routerkit.AllParts...))
		note(err)
		if err == nil {
			t.Error("Poll with bad credentials succeeded")
		}
		_, err = d.Identity(ctx)
		note(err)
		_ = d.Close(ctx)
	}
	mu.Lock()
	seen = append(seen, logs.String())
	mu.Unlock()
	for _, s := range seen {
		if strings.Contains(s, Sentinel) {
			t.Errorf("the password leaked: %q", s)
		}
	}
}

// ---- 4. context ----

func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		noPanic(t, what, fn)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Errorf("%s did not return within 2s of a cancelled context", what)
		<-done
	}
}

func testContext(t *testing.T, h Harness) {
	d := connected(t, h)
	caps := capsOf(t, d)
	parts := caps.Filter(routerkit.AllParts)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	within(t, "Poll", func() { _, _ = d.Poll(cancelled, routerkit.Poll(parts...)) })
	if len(caps.Sources) > 0 {
		within(t, "Get", func() { _, _ = d.Get(cancelled, caps.Sources[0].Name) })
	}
	if !h.Live {
		if inv, ok := safeInvocation(caps); ok {
			within(t, "Do", func() { _, _ = d.Do(cancelled, inv) })
		}
	}
	deadline, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	within(t, "Poll with a deadline", func() { _, _ = d.Poll(deadline, routerkit.Poll(parts...)) })
	within(t, "Connect", func() { _ = d.Connect(cancelled) })
}

// ---- 5. identity ----

func testIdentity(t *testing.T, h Harness) {
	d := connected(t, h)
	ctx := ctxT(t)
	id, err := d.Identity(ctx)
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.Vendor != d.Info().Vendor {
		t.Errorf("Identity.Vendor %q, Info().Vendor %q", id.Vendor, d.Info().Vendor)
	}
	if id.Model == "" {
		t.Error("Identity.Model is empty")
	}
	if id.MAC != "" && id.MAC != routerkit.NormalizeMAC(id.MAC) {
		t.Errorf("Identity.MAC %q is not normalized", id.MAC)
	}
	if v := d.Info().Variants; id.Variant != "" && len(v) > 0 && !contains(v, id.Variant) {
		t.Errorf("Identity.Variant %q is not one of Info().Variants %v", id.Variant, v)
	}
	again, err := d.Identity(ctx)
	if err != nil || again != id {
		t.Errorf("Identity is not stable: %+v then %+v (%v)", id, again, err)
	}
}

// ---- 6. capabilities and consistency ----

var partTypes = map[routerkit.Part]reflect.Type{
	routerkit.PartClients:  reflect.TypeOf(routerkit.Client{}),
	routerkit.PartWAN:      reflect.TypeOf(routerkit.WAN{}),
	routerkit.PartSystem:   reflect.TypeOf(routerkit.System{}),
	routerkit.PartTraffic:  reflect.TypeOf(routerkit.Interface{}),
	routerkit.PartNodes:    reflect.TypeOf(routerkit.Node{}),
	routerkit.PartFirmware: reflect.TypeOf(routerkit.Firmware{}),
}

func jsonName(f reflect.StructField) string {
	n := strings.Split(f.Tag.Get("json"), ",")[0]
	if n == "" {
		n = f.Name
	}
	return n
}

// fields returns the JSON field names of a part type; checkable only
// keeps those whose presence can be observed (not Extra, not plain bools).
func fields(typ reflect.Type, checkable bool) []string {
	var out []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		n := jsonName(f)
		if checkable && (n == "extra" || f.Type.Kind() == reflect.Bool) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// partValues returns the struct values of part p in s.
func partValues(s routerkit.Snapshot, p routerkit.Part) []reflect.Value {
	var v reflect.Value
	switch p {
	case routerkit.PartClients:
		v = reflect.ValueOf(s.Clients)
	case routerkit.PartWAN:
		v = reflect.ValueOf(s.WAN)
	case routerkit.PartSystem:
		v = reflect.ValueOf(s.System)
	case routerkit.PartTraffic:
		v = reflect.ValueOf(s.Traffic)
	case routerkit.PartNodes:
		v = reflect.ValueOf(s.Nodes)
	case routerkit.PartFirmware:
		v = reflect.ValueOf(s.Firmware)
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return []reflect.Value{v.Elem()}
	case reflect.Slice:
		out := make([]reflect.Value, v.Len())
		for i := range out {
			out[i] = v.Index(i)
		}
		return out
	}
	return nil
}

// present reports whether part p is non-nil in s.
func present(s routerkit.Snapshot, p routerkit.Part) bool {
	switch p {
	case routerkit.PartClients:
		return s.Clients != nil
	case routerkit.PartWAN:
		return s.WAN != nil
	case routerkit.PartSystem:
		return s.System != nil
	case routerkit.PartTraffic:
		return s.Traffic != nil
	case routerkit.PartNodes:
		return s.Nodes != nil
	case routerkit.PartFirmware:
		return s.Firmware != nil
	}
	return false
}

// setFields adds the JSON names of the fields of part p set in s.
func setFields(s routerkit.Snapshot, p routerkit.Part, into map[string]bool) {
	for _, v := range partValues(s, p) {
		for i := 0; i < v.NumField(); i++ {
			if !v.Field(i).IsZero() {
				into[jsonName(v.Type().Field(i))] = true
			}
		}
	}
}

func pollParts(t *testing.T, d routerkit.Driver, parts []routerkit.Part) routerkit.Snapshot {
	t.Helper()
	s, err := d.Poll(ctxT(t), routerkit.Poll(parts...))
	if err != nil {
		t.Fatalf("Poll %v: %v", parts, err)
	}
	return s
}

func testCapabilities(t *testing.T, h Harness) {
	d := connected(t, h)
	caps := capsOf(t, d)
	for p, ps := range caps.Parts {
		typ, known := partTypes[p]
		if !known {
			t.Errorf("Capabilities.Parts: unknown part %q", p)
			continue
		}
		if ps.Level > routerkit.Supported {
			t.Errorf("part %s: invalid level %d", p, ps.Level)
		}
		if (ps.Level == routerkit.Partial) != (len(ps.Missing) > 0) {
			t.Errorf("part %s: level %s with missing %v (partial iff fields are missing)", p, ps.Level, ps.Missing)
		}
		all := fields(typ, false)
		for _, m := range ps.Missing {
			if !contains(all, m) {
				t.Errorf("part %s: Missing lists %q, not a field of %s", p, m, typ.Name())
			}
		}
	}
	supported := caps.Filter(routerkit.AllParts)
	if len(caps.Parts) == 0 {
		t.Error("Capabilities declares no parts")
	}
	// Supported parts: present or failed; unsupported: absent, no error
	snaps := []routerkit.Snapshot{pollParts(t, d, routerkit.AllParts), pollParts(t, d, routerkit.AllParts)}
	for _, p := range routerkit.AllParts {
		for _, s := range snaps {
			_, failed := s.Errors[p]
			switch {
			case caps.Supports(p) && !present(s, p) && !failed:
				t.Errorf("part %s is supported but neither present nor in Errors", p)
			case !caps.Supports(p) && (present(s, p) || failed):
				t.Errorf("part %s is unsupported but present (%v) or failed (%v)", p, present(s, p), s.Errors[p])
			}
		}
	}
	// A part that was not requested is nil
	if len(supported) > 0 {
		one := supported[0]
		s := pollParts(t, d, []routerkit.Part{one})
		for _, p := range routerkit.AllParts {
			if p != one && present(s, p) {
				t.Errorf("Poll(%s) also returned %s", one, p)
			}
		}
	}
	for _, p := range h.Expect.NonEmpty {
		if len(partValues(snaps[0], p)) == 0 {
			t.Errorf("part %s is empty on this fixture", p)
		}
	}
	// Missing fields are never set; the others are set at least once
	for _, p := range supported {
		set := map[string]bool{}
		for _, s := range snaps {
			setFields(s, p, set)
		}
		if _, failed := snaps[0].Errors[p]; failed {
			continue
		}
		ps := caps.Parts[p]
		for _, f := range fields(partTypes[p], true) {
			missing := contains(ps.Missing, f)
			switch {
			case missing && set[f]:
				t.Errorf("%s.%s is listed Missing but set", p, f)
			case !missing && !set[f] && !contains(h.Expect.Unset, string(p)+"."+f) && len(partValues(snaps[0], p)) > 0:
				t.Errorf("%s.%s is never set on this fixture: list it in PartSupport.Missing or Expect.Unset", p, f)
			}
		}
	}
	// Listed actions and sources are unique
	checkUnique(t, "action", names(caps.Actions, func(a routerkit.ActionSpec) string { return a.Name }))
	checkUnique(t, "source", names(caps.Sources, func(s routerkit.SourceSpec) string { return s.Name }))
}

// ---- 7. snapshot invariants ----

var connections = []string{"", "wired", "2.4ghz", "5ghz", "6ghz", "wifi"}

func checkSnapshot(t *testing.T, s routerkit.Snapshot) {
	t.Helper()
	if s.Time.IsZero() {
		t.Error("Snapshot.Time is not set")
	}
	macs := map[string]bool{}
	for _, c := range s.Clients {
		if c.MAC == "" || c.MAC != routerkit.NormalizeMAC(c.MAC) {
			t.Errorf("client MAC %q is not normalized", c.MAC)
		}
		if macs[c.MAC] {
			t.Errorf("client MAC %s twice", c.MAC)
		}
		macs[c.MAC] = true
		if !contains(connections, c.Connection) {
			t.Errorf("client %s: connection %q outside the vocabulary", c.MAC, c.Connection)
		}
		if c.RSSI != nil && (*c.RSSI < -120 || *c.RSSI > 0) {
			t.Errorf("client %s: RSSI %d outside [-120, 0]", c.MAC, *c.RSSI)
		}
		nonNegative(t, "client "+c.MAC+" rx_rate", c.RxRate)
		nonNegative(t, "client "+c.MAC+" tx_rate", c.TxRate)
		if c.Node != "" && c.Node != routerkit.NormalizeMAC(c.Node) {
			t.Errorf("client %s: node %q is not a normalized MAC", c.MAC, c.Node)
		}
	}
	if w := s.WAN; w != nil {
		if w.PublicIP != "" && net.ParseIP(w.PublicIP) == nil {
			t.Errorf("WAN.PublicIP %q is not an IP", w.PublicIP)
		}
		for _, ip := range []string{w.IP, w.IPv6, w.Gateway} {
			if ip != "" && net.ParseIP(ip) == nil {
				t.Errorf("WAN address %q is not an IP", ip)
			}
		}
	}
	if sys := s.System; sys != nil {
		percent(t, "cpu_usage", sys.CPUUsage)
		percent(t, "ram_usage", sys.RAMUsage)
		for k, v := range sys.Temperatures {
			if v < -40 || v > 150 {
				t.Errorf("temperature %s = %v outside [-40, 150]", k, v)
			}
		}
		if sys.Uptime != nil && *sys.Uptime < 0 {
			t.Errorf("uptime %d < 0", *sys.Uptime)
		}
	}
	for _, it := range s.Traffic {
		if it.Name == "" {
			t.Error("interface without a name")
		}
		nonNegative(t, it.Name+" rx_rate", it.RxRate)
		nonNegative(t, it.Name+" tx_rate", it.TxRate)
	}
	nodes := map[string]bool{}
	for _, n := range s.Nodes {
		if n.MAC == "" || n.MAC != routerkit.NormalizeMAC(n.MAC) || nodes[n.MAC] {
			t.Errorf("node MAC %q: empty, not normalized or twice", n.MAC)
		}
		nodes[n.MAC] = true
	}
	for p, pe := range s.Errors {
		if pe.Code == "" || pe.Message == "" {
			t.Errorf("Errors[%s] = %+v: code and message required", p, pe)
		}
	}
	b1, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("snapshot is not JSON-able: %v", err)
	}
	var back routerkit.Snapshot
	if err := json.Unmarshal(b1, &back); err != nil {
		t.Fatalf("snapshot JSON does not decode: %v", err)
	}
	if b2, _ := json.Marshal(back); !bytes.Equal(b1, b2) {
		t.Errorf("snapshot JSON does not round-trip:\n%s\n%s", b1, b2)
	}
}

func nonNegative(t *testing.T, what string, v *float64) {
	t.Helper()
	if v != nil && *v < 0 {
		t.Errorf("%s = %v < 0", what, *v)
	}
}

func percent(t *testing.T, what string, v *float64) {
	t.Helper()
	if v != nil && (*v < 0 || *v > 100) {
		t.Errorf("%s = %v outside [0, 100]", what, *v)
	}
}

func testSnapshot(t *testing.T, h Harness) {
	d := connected(t, h)
	parts := capsOf(t, d).Filter(routerkit.AllParts)
	first := pollParts(t, d, parts)
	checkSnapshot(t, first)
	checkSnapshot(t, pollParts(t, d, parts))
	if h.Expect.Golden == "" || h.Live {
		return
	}
	// The golden snapshot is the first poll of a fresh driver
	fresh := connected(t, h)
	golden(t, h.Expect.Golden, pollParts(t, fresh, capsOf(t, fresh).Filter(routerkit.AllParts)))
}

// Normalize makes a snapshot comparable across runs: no poll time,
// "since" times replaced by the Unix epoch, lists sorted.
func Normalize(s routerkit.Snapshot) routerkit.Snapshot {
	s.Time = time.Time{}
	epoch := time.Unix(0, 0).UTC()
	s.Clients = append([]routerkit.Client(nil), s.Clients...)
	for i := range s.Clients {
		if s.Clients[i].Since != nil {
			s.Clients[i].Since = &epoch
		}
	}
	sort.SliceStable(s.Clients, func(i, j int) bool { return s.Clients[i].MAC < s.Clients[j].MAC })
	s.Traffic = append([]routerkit.Interface(nil), s.Traffic...)
	sort.SliceStable(s.Traffic, func(i, j int) bool { return s.Traffic[i].Name < s.Traffic[j].Name })
	s.Nodes = append([]routerkit.Node(nil), s.Nodes...)
	sort.SliceStable(s.Nodes, func(i, j int) bool { return s.Nodes[i].MAC < s.Nodes[j].MAC })
	return s
}

func golden(t *testing.T, path string, s routerkit.Snapshot) {
	t.Helper()
	got, err := json.MarshalIndent(Normalize(s), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden snapshot: %v (run with -drivertest.update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the snapshot differs from %s (run with -drivertest.update and review the diff):\n%s", path, firstDiff(want, got))
	}
}

func firstDiff(want, got []byte) string {
	wl, gl := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d:\n- %s\n+ %s", i+1, w, g)
		}
	}
	return ""
}

// ---- 8. partial failure ----

func testPartial(t *testing.T, h Harness) {
	if h.PartFailure == nil {
		t.Skip("no PartFailure hook")
	}
	ctx := ctxT(t)
	d, failing, healthy := h.PartFailure(t)
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close(context.Background())
	s, err := d.Poll(ctx, routerkit.Poll(failing, healthy))
	if err != nil {
		t.Fatalf("Poll with one failing part: %v", err)
	}
	if pe, ok := s.Errors[failing]; !ok || pe.Code == "" || pe.Message == "" {
		t.Errorf("Errors[%s] = %+v, want a code and a message", failing, s.Errors[failing])
	}
	if present(s, failing) {
		t.Errorf("failed part %s is also present", failing)
	}
	if !present(s, healthy) {
		t.Errorf("healthy part %s is missing", healthy)
	}
	if _, err := d.Poll(ctx, routerkit.Poll(failing)); err == nil {
		t.Errorf("Poll(%s) succeeded although every requested part failed", failing)
	}
}

// ---- 9. concurrency ----

func testConcurrency(t *testing.T, h Harness) {
	d := connected(t, h)
	caps := capsOf(t, d)
	parts := caps.Filter(routerkit.AllParts)
	ctx := ctxT(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("goroutine %d panicked: %v", i, r)
				}
			}()
			for round := 0; round < 5; round++ {
				if _, err := d.Poll(ctx, routerkit.Poll(parts...)); err != nil {
					t.Errorf("concurrent Poll: %v", err)
				}
				if len(caps.Sources) > 0 {
					if _, err := d.Get(ctx, caps.Sources[(i+round)%len(caps.Sources)].Name); err != nil && !errors.Is(err, routerkit.ErrUnsupported) {
						t.Errorf("concurrent Get: %v", err)
					}
				}
				if _, err := d.Identity(ctx); err != nil {
					t.Errorf("concurrent Identity: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}

// ---- 10. native sources ----

func testSources(t *testing.T, h Harness) {
	d := connected(t, h)
	ctx := ctxT(t)
	for _, s := range capsOf(t, d).Sources {
		v, err := d.Get(ctx, s.Name)
		if err != nil {
			if !errors.Is(err, routerkit.ErrUnsupported) {
				t.Errorf("Get(%s): %v (a listed source answers or is ErrUnsupported)", s.Name, err)
			}
			continue
		}
		if _, err := json.Marshal(v); err != nil {
			t.Errorf("Get(%s) is not JSON-able: %v", s.Name, err)
		}
	}
	_, err := d.Get(ctx, "drivertest-no-such-source")
	if !errors.Is(err, routerkit.ErrNotFound) && !errors.Is(err, routerkit.ErrUnsupported) {
		t.Errorf("Get of an unknown source = %v, want ErrNotFound or ErrUnsupported", err)
	}
}

// ---- 11. actions ----

// sampleArgs returns valid raw arguments for spec (required ones only).
func sampleArgs(spec routerkit.ActionSpec) map[string]string {
	raw := map[string]string{}
	for _, a := range spec.Args {
		if !a.Required {
			continue
		}
		switch a.Type {
		case routerkit.ArgBool:
			raw[a.Name] = "on"
		case routerkit.ArgEnum:
			if len(a.Choices) > 0 {
				raw[a.Name] = a.Choices[0]
			}
		case routerkit.ArgMAC:
			raw[a.Name] = "02:00:00:00:00:99"
		case routerkit.ArgIP:
			raw[a.Name] = "192.0.2.99"
		case routerkit.ArgInt:
			raw[a.Name] = "1"
		case routerkit.ArgDuration:
			raw[a.Name] = "1s"
		default:
			raw[a.Name] = "drivertest"
		}
	}
	return raw
}

// safeInvocation returns a bound Invocation of the first Safe action.
func safeInvocation(caps routerkit.Capabilities) (routerkit.Invocation, bool) {
	for _, a := range caps.Actions {
		if a.Danger != routerkit.Safe {
			continue
		}
		if _, inv, err := caps.Invoke(a.Name, sampleArgs(a)); err == nil {
			return inv, true
		}
	}
	return routerkit.Invocation{}, false
}

func testActions(t *testing.T, h Harness) {
	d := connected(t, h)
	caps := capsOf(t, d)
	ctx := ctxT(t)
	checkUnique(t, "action", names(caps.Actions, func(a routerkit.ActionSpec) string { return a.Name }))
	for _, a := range caps.Actions {
		if err := routerkit.CheckSpec(a); err != nil {
			t.Error(err)
		}
		if a.Name == routerkit.ActReboot && a.Danger != routerkit.Destructive {
			t.Errorf("reboot is %s, want destructive", a.Danger)
		}
		if _, err := a.Bind(sampleArgs(a)); err != nil {
			t.Errorf("action %s: sample arguments do not bind: %v", a.Name, err)
		}
	}
	if h.Live && !LiveWrite() {
		return // a real device: no action without ROUTERCTL_LIVE_WRITE=1
	}
	requests := func() int {
		if h.Requests == nil {
			return 0
		}
		return h.Requests()
	}
	before := requests()
	for _, a := range caps.Actions {
		bogus := sampleArgs(a)
		bogus["drivertest-bogus"] = "1"
		if _, err := d.Do(ctx, routerkit.Invocation{Name: a.Name, Args: routerkit.RawArgs(bogus)}); !errors.Is(err, routerkit.ErrInvalidArgs) {
			t.Errorf("Do(%s) with an unknown argument = %v, want ErrInvalidArgs", a.Name, err)
		}
		if len(sampleArgs(a)) > 0 {
			if _, err := d.Do(ctx, routerkit.Invocation{Name: a.Name}); !errors.Is(err, routerkit.ErrInvalidArgs) {
				t.Errorf("Do(%s) without its required arguments = %v, want ErrInvalidArgs", a.Name, err)
			}
		}
	}
	if _, err := d.Do(ctx, routerkit.Invocation{Name: "drivertest-no-such-action"}); !errors.Is(err, routerkit.ErrUnsupported) {
		t.Errorf("Do of an unknown action = %v, want ErrUnsupported", err)
	}
	if after := requests(); after != before {
		t.Errorf("invalid Do calls sent %d device request(s)", after-before)
	}
	if !h.Expect.RunSafeActions && !h.Live {
		return
	}
	for _, a := range caps.Actions {
		if a.Danger != routerkit.Safe {
			continue
		}
		_, inv, err := caps.Invoke(a.Name, sampleArgs(a))
		if err != nil {
			t.Errorf("Invoke(%s): %v", a.Name, err)
			continue
		}
		res, err := d.Do(ctx, inv)
		if err != nil || !res.Accepted {
			t.Errorf("safe action %s: %+v, %v", a.Name, res, err)
		}
		if _, err := json.Marshal(res); err != nil {
			t.Errorf("result of %s is not JSON-able: %v", a.Name, err)
		}
	}
}

// ---- 12. fingerprints ----

func testFingerprints(t *testing.T, h Harness) {
	f := h.Factory
	if len(f.Fingerprints) == 0 {
		if h.Homepages != nil {
			t.Error("Homepages given but the factory has no fingerprints")
		}
		t.Skip("no fingerprints")
	}
	for _, page := range []string{"", "<html><head><title>Router</title></head><body>login</body></html>"} {
		for _, m := range f.Matches(routerkit.HomeEvidence(page)) {
			if m.Score > routerkit.ScoreWeak {
				t.Errorf("a generic page %q matches with score %d", page, m.Score)
			}
		}
	}
	if h.Homepages == nil {
		t.Skip("no Homepages fixtures")
	}
	n := 0
	err := fs.WalkDir(h.Homepages, ".", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, err := fs.ReadFile(h.Homepages, path)
		if err != nil {
			return err
		}
		n++
		best := routerkit.Match{}
		for _, m := range f.Matches(routerkit.HomeEvidence(string(b))) {
			if m.Score > best.Score {
				best = m
			}
		}
		switch {
		case best.Score <= routerkit.ScoreWeak:
			t.Errorf("%s: not recognized (best score %d)", path, best.Score)
		case best.Score > routerkit.ScoreDefinitive:
			t.Errorf("%s: score %d above ScoreDefinitive", path, best.Score)
		case best.Driver != f.Info.Name:
			t.Errorf("%s: matched as %q", path, best.Driver)
		case h.Expect.Variant != "" && best.Variant != h.Expect.Variant:
			t.Errorf("%s: variant %q, want %q", path, best.Variant, h.Expect.Variant)
		case best.Reason == "":
			t.Errorf("%s: match without a reason", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("Homepages is empty")
	}
}

// ---- helpers ----

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func names[T any](list []T, name func(T) string) []string {
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = name(x)
	}
	return out
}

func checkUnique(t *testing.T, what string, list []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, n := range list {
		if seen[n] {
			t.Errorf("%s %q listed twice", what, n)
		}
		seen[n] = true
	}
}
