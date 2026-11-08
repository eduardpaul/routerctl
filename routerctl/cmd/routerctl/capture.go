package main

// capture and replay, over the optional Factory.Record / Factory.Replay
// hooks (today only the ASUS driver has them: its sessions are the format
// shared with the Python library).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/eduardpaul/routerctl/routerctl/internal/output"
	"github.com/eduardpaul/routerctl/routerkit"
)

// withHook returns the factories that have a hook, by name.
func withHook(has func(routerkit.Factory) bool) []routerkit.Factory {
	var out []routerkit.Factory
	for _, n := range registry().Names() {
		if f, _ := registry().Lookup(n); has(f) {
			out = append(out, f)
		}
	}
	return out
}

// hookFactory picks the factory for a hook: the named driver, or with
// "auto" the only driver that has the hook (else the detected brand).
func hookFactory(ctx context.Context, what, name string, cfg routerkit.Config, has func(routerkit.Factory) bool) (routerkit.Factory, error) {
	if name == "" || name == "auto" {
		cands := withHook(has)
		switch {
		case len(cands) == 0:
			return routerkit.Factory{}, fmt.Errorf("%s is not supported by any driver in this build", what)
		case len(cands) == 1:
			return cands[0], nil
		}
		detected, err := registry().Detect(ctx, cfg)
		if err != nil {
			return routerkit.Factory{}, err
		}
		name = detected
	}
	f, ok := registry().Lookup(name)
	if !ok {
		return routerkit.Factory{}, fmt.Errorf("unknown driver %q (available: %v)", name, registry().Names())
	}
	if !has(f) {
		return routerkit.Factory{}, fmt.Errorf("%s is not supported by driver %s", what, name)
	}
	return f, nil
}

func canRecord(f routerkit.Factory) bool { return f.Record != nil }
func canReplay(f routerkit.Factory) bool { return f.Replay != nil }

func cmdCapture(ctx context.Context, args []string) error {
	fs := newFlagSet("capture", "<file> [source...]")
	df := addDevFlags(fs, false)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return usagef("want the output file")
	}
	setupLogging(df.logLevel, "warn", "text")
	cfg, err := df.loadConfig()
	if err != nil {
		return err
	}
	dev, err := df.selected(cfg)
	if err != nil {
		return err
	}
	f, err := hookFactory(ctx, "capture", dev.Driver, dev.DriverConfig(), canRecord)
	if err != nil {
		return fmt.Errorf("device %s: %w", dev.Name, err)
	}
	drv, rec, err := f.Record(dev.DriverConfig())
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 2*dev.Timeout.D())
	defer cancel()
	if err := drv.Connect(cctx); err != nil {
		return err
	}
	defer closeQuiet(drv)
	names := pos[1:]
	if len(names) == 0 {
		caps, err := drv.Capabilities(ctx)
		if err != nil {
			return err
		}
		for _, s := range caps.Sources {
			names = append(names, s.Name)
		}
	}
	var failed []string
	for _, n := range names {
		if _, err := drv.Get(ctx, n); err != nil {
			failed = append(failed, n)
		}
	}
	recs := rec.Len()
	// Create the file private before writing (it holds private data)
	pf, err := os.OpenFile(pos[0], os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	pf.Close()
	_ = os.Chmod(pos[0], 0o600)
	if err := rec.Save(pos[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "captured %d queries from %d sources to %s (%d failed)\n", recs, len(names), pos[0], len(failed))
	fmt.Fprintln(os.Stderr, "the capture holds private data (MACs, SSIDs, IPs): sanitize it before sharing")
	return nil
}

func cmdReplay(ctx context.Context, args []string) error {
	fs := newFlagSet("replay", "<file> [source...|status|clients]")
	out := fs.String("output", "json", "output: json | table | csv")
	fs.StringVar(out, "o", "json", "output (shorthand)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return usagef("want the session file")
	}
	f, err := output.ParseFormat(*out)
	if err != nil {
		return err
	}
	setupLogging("", "error", "text")
	cfg := routerkit.Config{Host: "replay"}
	fac, err := hookFactory(ctx, "replay", "", cfg, canReplay)
	if err != nil {
		return err
	}
	drv, err := fac.Replay(pos[0], cfg)
	if err != nil {
		return err
	}
	if err := drv.Connect(ctx); err != nil {
		return err
	}
	defer closeQuiet(drv)
	names := pos[1:]
	if len(names) == 1 && (names[0] == "status" || names[0] == "clients" || names[0] == "snapshot") {
		snap, err := drv.Poll(ctx, routerkit.Poll(routerkit.AllParts...))
		if err != nil {
			return err
		}
		id, _ := drv.Identity(ctx)
		var v any = map[string]any{"identity": id, "snapshot": snap}
		rows := output.Tabulate(map[string]any{})
		if names[0] == "clients" {
			v = snap.Clients
			rows = output.Rows{Header: []string{"mac", "name", "ip", "online", "connection", "node"}}
			for _, c := range snap.Clients {
				rows.Rows = append(rows.Rows, []string{c.MAC, c.Name, c.IP, fmt.Sprint(c.Online), c.Connection, c.Node})
			}
		}
		return output.Write(os.Stdout, f, v, rows)
	}
	if len(names) == 0 {
		caps, err := drv.Capabilities(ctx)
		if err != nil {
			return err
		}
		for _, s := range caps.Sources {
			names = append(names, s.Name)
		}
	}
	res := map[string]any{}
	var errs []error
	for _, n := range names {
		v, err := drv.Get(ctx, n)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n, err))
			continue
		}
		res[n] = v
	}
	sort.Strings(names)
	var v any = res
	if len(names) == 1 {
		v = res[names[0]]
	}
	if err := output.Write(os.Stdout, f, v, output.Tabulate(v)); err != nil {
		return err
	}
	return errors.Join(errs...)
}
