package routerkit

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Factory builds the drivers of one brand family. Drivers export one
// (asus.Factory(), zte.Factory(), ...); the program that composes them
// passes the factories it wants to NewRegistry.
type Factory struct {
	// Info is the static metadata; Info.Name is the registry key
	Info DriverInfo
	// Options lists the brand-specific options (Config.Options); New
	// rejects unknown ones and values that do not parse (BindOptions)
	Options []OptionSpec
	New     func(cfg Config) (Driver, error)
	// Fingerprints recognize this brand's devices without logging in
	// (see routerkit/detect, which fetches and scores)
	Fingerprints []Fingerprint
	// Replay opens a recorded session (see Record) as an offline driver;
	// nil when the driver cannot replay. cfg carries no credentials.
	Replay func(path string, cfg Config) (Driver, error)
	// Record builds a driver that records every exchange with the device
	// into the returned Recorder; nil when the driver cannot record.
	Record func(cfg Config) (Driver, Recorder, error)
}

// Recorder holds the exchanges a recording driver made (see
// Factory.Record). Sessions hold private data (MACs, SSIDs, IPs).
type Recorder interface {
	// Len returns the number of recorded exchanges
	Len() int
	// Save writes the session to a file Factory.Replay can open
	Save(path string) error
}

// Registry is a set of driver factories. It is immutable after
// NewRegistry and safe for concurrent use.
type Registry struct {
	factories map[string]Factory
	names     []string
	probe     Prober
}

// NewRegistry builds a registry. Names must be non-empty and unique, and
// every factory needs New.
func NewRegistry(fs ...Factory) (*Registry, error) {
	r := &Registry{factories: make(map[string]Factory, len(fs))}
	var errs []error
	for _, f := range fs {
		name := f.Info.Name
		switch {
		case !ValidName(name) || name == "auto":
			errs = append(errs, fmt.Errorf("invalid driver name %q", name))
			continue
		case f.New == nil:
			errs = append(errs, fmt.Errorf("driver %s: no New", name))
			continue
		}
		if err := CheckOptionSpecs(f.Options); err != nil {
			errs = append(errs, fmt.Errorf("driver %s: %w", name, err))
			continue
		}
		if err := checkFingerprints(f.Fingerprints); err != nil {
			errs = append(errs, fmt.Errorf("driver %s: %w", name, err))
			continue
		}
		if _, dup := r.factories[name]; dup {
			errs = append(errs, fmt.Errorf("duplicate driver %q", name))
			continue
		}
		r.factories[name] = f
		r.names = append(r.names, name)
	}
	sort.Strings(r.names)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return r, nil
}

// ValidName reports whether s is a valid driver name: [a-z0-9-]+.
func ValidName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// WithProber returns a copy of the registry that detects brands ("auto")
// with p.
func (r *Registry) WithProber(p Prober) *Registry {
	c := *r
	c.probe = p
	return &c
}

// Names lists the drivers, sorted.
func (r *Registry) Names() []string { return append([]string(nil), r.names...) }

// Lookup returns a factory by name.
func (r *Registry) Lookup(name string) (Factory, bool) {
	f, ok := r.factories[name]
	return f, ok
}

// Factories returns the factories, sorted by name.
func (r *Registry) Factories() []Factory {
	out := make([]Factory, 0, len(r.names))
	for _, n := range r.names {
		out = append(out, r.factories[n])
	}
	return out
}

// Detect probes cfg's device (without logging in) and names its driver.
// Errors are prefixed with "<cfg.Name>: detect: ".
func (r *Registry) Detect(ctx context.Context, cfg Config) (string, error) {
	if r.probe == nil {
		return "", fmt.Errorf("%sdetect: no prober configured", label(cfg))
	}
	m, err := r.probe(ctx, r, cfg)
	if err != nil {
		return "", fmt.Errorf("%sdetect: %w", label(cfg), err)
	}
	if _, ok := r.factories[m.Driver]; !ok {
		return "", fmt.Errorf("%sdetect: the prober named an unknown driver %q", label(cfg), m.Driver)
	}
	return m.Driver, nil
}

// Open returns the driver `name` for cfg. With name "auto" (or "") it
// returns a Wrapper that detects the brand (see WithProber) on the first
// Connect and then delegates to that brand's driver.
func (r *Registry) Open(name string, cfg Config) (Driver, error) {
	if name == "" || name == "auto" {
		return &auto{reg: r, cfg: cfg}, nil
	}
	return r.build(name, cfg)
}

func (r *Registry) build(name string, cfg Config) (Driver, error) {
	f, ok := r.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("%sunknown driver %q (available: %v)", label(cfg), name, r.names)
	}
	if _, err := BindOptions(f.Options, cfg.Options); err != nil {
		return nil, fmt.Errorf("%s%s: %w", label(cfg), name, err)
	}
	drv, err := f.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s%w", label(cfg), err)
	}
	return drv, nil
}

// label prefixes messages with the device name, when there is one.
func label(cfg Config) string {
	if cfg.Name == "" {
		return ""
	}
	return cfg.Name + ": "
}
