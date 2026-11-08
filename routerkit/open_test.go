package routerkit

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubDriver is a minimal Driver for the auto wrapper tests.
type stubDriver struct {
	Base
	connected bool
}

func (s *stubDriver) Info() DriverInfo              { return DriverInfo{Name: "zz-open", Vendor: "ZZ"} }
func (s *stubDriver) Connect(context.Context) error { s.connected = true; return nil }
func (s *stubDriver) Close(context.Context) error   { return nil }
func (s *stubDriver) Identity(context.Context) (Identity, error) {
	return Identity{Vendor: "zz-open", Model: "M"}, nil
}
func (s *stubDriver) Poll(context.Context, PollRequest) (Snapshot, error) { return Snapshot{}, nil }

func newStub(Config) (Driver, error) { return &stubDriver{}, nil }

func stubRegistry(t *testing.T, probe Prober) *Registry {
	t.Helper()
	r, err := NewRegistry(Factory{
		Info:         DriverInfo{Name: "zz-open"},
		New:          newStub,
		Fingerprints: []Fingerprint{zzFingerprint("zz-open-home", ScoreStrong)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe != nil {
		r = r.WithProber(probe)
	}
	return r
}

func TestOpenAuto(t *testing.T) {
	var probed Config
	probe := func(_ context.Context, r *Registry, cfg Config) (Match, error) {
		probed = cfg
		for _, f := range r.Factories() {
			if ms := f.Matches(HomeEvidence("zz-open-home")); len(ms) > 0 {
				return ms[0], nil
			}
		}
		return Match{}, errors.New("no match")
	}
	cfg := Config{Name: "home", Host: "192.0.2.1", Port: 8443}
	drv, err := stubRegistry(t, probe).Open("auto", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if drv.Info().Name != "auto" {
		t.Errorf("Info before Connect = %+v", drv.Info())
	}
	if _, err := drv.Capabilities(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Capabilities before Connect: %v", err)
	}
	if _, ok := As[*stubDriver](drv); ok {
		t.Error("As found a driver before Connect")
	}
	w, ok := drv.(Wrapper)
	if !ok || w.Unwrap() != nil {
		t.Fatalf("auto must be a Wrapper with nothing inside before Connect")
	}
	if _, err := drv.Identity(context.Background()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Identity before Connect: %v", err)
	}
	if err := drv.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if probed.Host != cfg.Host || probed.Port != cfg.Port {
		t.Errorf("probe got %+v", probed)
	}
	inner, ok := w.Unwrap().(*stubDriver)
	if !ok || !inner.connected {
		t.Fatalf("Unwrap = %#v", w.Unwrap())
	}
	if drv.Info().Name != "zz-open" {
		t.Errorf("Info after Connect = %+v", drv.Info())
	}
	if got, ok := As[*stubDriver](drv); !ok || got != inner {
		t.Errorf("As through the wrapper = %v, %v", got, ok)
	}
	if _, ok := As[*auto](inner); ok {
		t.Error("As found a wrapper below the inner driver")
	}
}

func TestOpenErrors(t *testing.T) {
	r := stubRegistry(t, nil)
	if _, err := r.Open("zz-nope", Config{Name: "home"}); err == nil || !strings.HasPrefix(err.Error(), `home: unknown driver "zz-nope"`) {
		t.Errorf("unknown driver: %v", err)
	}
	if d, err := r.Open("zz-open", Config{}); err != nil || d.Info().Name != "zz-open" {
		t.Errorf("named driver: %v, %v", d, err)
	}
	drv, _ := r.Open("auto", Config{Name: "home"})
	if err := drv.Connect(context.Background()); err == nil || !strings.Contains(err.Error(), "no prober") {
		t.Errorf("no prober: %v", err)
	}
	r = stubRegistry(t, func(context.Context, *Registry, Config) (Match, error) { return Match{}, errors.New("refused") })
	drv, _ = r.Open("", Config{Name: "home"})
	if err := drv.Connect(context.Background()); err == nil || err.Error() != "home: detect: refused" {
		t.Errorf("probe failure: %v", err)
	}
	r = stubRegistry(t, func(context.Context, *Registry, Config) (Match, error) {
		return Match{}, errors.New("unrecognized device (known drivers: [zz-open])")
	})
	drv, _ = r.Open("auto", Config{Name: "home"})
	if err := drv.Connect(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "home: detect: unrecognized device") {
		t.Errorf("unrecognized: %v", err)
	}
	if _, err := r.Detect(context.Background(), Config{}); err == nil || !strings.HasPrefix(err.Error(), "detect: unrecognized") {
		t.Errorf("Detect without a name: %v", err)
	}
	r = stubRegistry(t, func(context.Context, *Registry, Config) (Match, error) { return Match{Driver: "zz-other"}, nil })
	if _, err := r.Detect(context.Background(), Config{}); err == nil || !strings.Contains(err.Error(), "unknown driver") {
		t.Errorf("a prober naming an unknown driver: %v", err)
	}
}
