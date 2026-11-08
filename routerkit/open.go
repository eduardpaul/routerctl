package routerkit

import (
	"context"
	"fmt"
	"sync"
)

// Prober names the driver of cfg's device without logging in: it fetches
// the probes of r's fingerprints and returns the best match (or an error
// when nothing or more than one driver matches best). routerkit/detect.Prober
// is the standard implementation; it lives outside this package so the
// contract stays free of HTTP code.
type Prober func(ctx context.Context, r *Registry, cfg Config) (Match, error)

// auto detects the brand on Connect and then delegates.
type auto struct {
	Base
	reg   *Registry
	cfg   Config
	mu    sync.Mutex
	inner Driver
}

var _ Wrapper = (*auto)(nil)

func (a *auto) get() Driver {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.inner
}

// Unwrap implements Wrapper (nil before the first successful Connect).
func (a *auto) Unwrap() Driver { return a.get() }

var errNotConnected = fmt.Errorf("brand not detected yet (not connected): %w", ErrUnsupported)

// autoInfo is the Info of the auto driver before detection.
var autoInfo = DriverInfo{Name: "auto", Vendor: "auto", Help: "detects the brand on Connect", Stability: Stable, Contract: ContractVersion}

func (a *auto) Info() DriverInfo {
	if in := a.get(); in != nil {
		return in.Info()
	}
	return autoInfo
}

func (a *auto) Capabilities(ctx context.Context) (Capabilities, error) {
	if in := a.get(); in != nil {
		return in.Capabilities(ctx)
	}
	return Capabilities{}, errNotConnected
}

func (a *auto) Connect(ctx context.Context) error {
	in := a.get()
	if in == nil {
		vendor, err := a.reg.Detect(ctx, a.cfg)
		if err != nil {
			return err
		}
		in, err = a.reg.build(vendor, a.cfg)
		if err != nil {
			return err
		}
		a.mu.Lock()
		a.inner = in
		a.mu.Unlock()
	}
	return in.Connect(ctx)
}

func (a *auto) Close(ctx context.Context) error {
	if in := a.get(); in != nil {
		return in.Close(ctx)
	}
	return nil
}

func (a *auto) Identity(ctx context.Context) (Identity, error) {
	if in := a.get(); in != nil {
		return in.Identity(ctx)
	}
	return Identity{}, errNotConnected
}

func (a *auto) Poll(ctx context.Context, req PollRequest) (Snapshot, error) {
	if in := a.get(); in != nil {
		return in.Poll(ctx, req)
	}
	return Snapshot{}, errNotConnected
}

func (a *auto) Get(ctx context.Context, name string) (any, error) {
	if in := a.get(); in != nil {
		return in.Get(ctx, name)
	}
	return nil, errNotConnected
}

func (a *auto) Do(ctx context.Context, inv Invocation) (Result, error) {
	if in := a.get(); in != nil {
		return in.Do(ctx, inv)
	}
	return Result{}, errNotConnected
}
