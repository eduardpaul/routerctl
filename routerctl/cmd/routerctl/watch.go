package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/eduardpaul/routerctl/routerctl/internal/app"
	"github.com/eduardpaul/routerctl/routerctl/internal/config"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/inventory"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
)

func cmdWatch(ctx context.Context, args []string) error {
	fs := newFlagSet("watch", "[-c config] [-d device]")
	df := addDevFlags(fs, false)
	logFormat := fs.String("log-format", "", "text | json (default from config)")
	stateFile := fs.String("state", "", "state file (default from config)")
	stdoutOn := fs.Bool("stdout", false, "print events as JSON lines on stdout")
	httpListen := fs.String("http", "", "HTTP server address (default http.listen, 127.0.0.1:9100)")
	metricsOn := fs.Bool("metrics", false, "serve /metrics on the HTTP server")
	apiOn := fs.Bool("api", false, "serve the HTTP API (/api/v1, used by Node-RED and --via)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	cfg, err := df.loadConfig()
	if err != nil {
		return err
	}
	level := cfg.Log.Level
	if df.logLevel != "" {
		level = df.logLevel
	}
	format := cfg.Log.Format
	if *logFormat != "" {
		format = *logFormat
	}
	setupLogging(level, "info", format)
	if *stateFile != "" {
		cfg.StateFile = *stateFile
	}
	if *stdoutOn {
		cfg.Stdout.Enabled = true
	}
	if *httpListen != "" {
		cfg.HTTP.Listen = *httpListen
	}
	if *metricsOn {
		cfg.Metrics.Enabled = true
	}
	if *apiOn {
		cfg.API.Enabled = true
	}
	if *httpListen != "" || *apiOn {
		// Flags change what Finish validated (token on non-loopback)
		if err := cfg.Finish(); err != nil {
			return err
		}
	}

	devs := cfg.Devices
	if df.device != "" {
		d, err := cfg.Device(df.device)
		if err != nil {
			return err
		}
		devs = []config.Device{*d}
	}
	if len(devs) == 0 {
		return usagef("no device configured (config file, ROUTERCTL_HOST or --host)")
	}
	var wd []poller.Device
	for _, d := range devs {
		drv, err := app.OpenDevice(registry(), d)
		if err != nil {
			return err
		}
		wd = append(wd, app.PollerDevice(d, drv))
	}
	store, err := inventory.Open(cfg.StateFile)
	if err != nil {
		return err
	}
	sinks, err := app.Sinks(cfg, buildVersion(), os.Stdout)
	if err != nil {
		return err
	}
	if len(sinks) == 0 {
		slog.Warn("no output enabled (mqtt, notify, api, metrics, stdout); only the inventory is kept")
	}
	d, err := poller.New(wd, poller.Options{
		Presence: app.PollerPresence(cfg.Presence),
		Baseline: cfg.Inventory.BaselineEnabled(),
		Store:    store,
		Sinks:    sinks,
		Logger:   slog.Default(),
	})
	if err != nil {
		return err
	}
	names := make([]string, len(devs))
	for i, dv := range devs {
		names[i] = dv.Name + "=" + dv.Driver + "@" + dv.Host
	}
	slog.Info("routerctl watch starting", "version", buildVersion(), "devices", names, "state_file", cfg.StateFile)
	return d.Run(ctx)
}
