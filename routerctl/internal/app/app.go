// Package app is the composition layer of routerctl: it maps the config
// file (package config) onto the drivers, the services and the sinks,
// which know nothing about the config format.
package app

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/config"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/commands"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/api"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/metrics"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/mqtt"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/notify"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/stdout"
	"github.com/eduardpaul/routerctl/routerctl/internal/sink/web"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/detect"
)

// Registry builds the driver registry of the program, detecting brands
// ("auto") from the device home page.
func Registry(fs ...routerkit.Factory) (*routerkit.Registry, error) {
	r, err := routerkit.NewRegistry(fs...)
	if err != nil {
		return nil, err
	}
	return r.WithProber(detect.Prober), nil
}

// OpenDevice returns the driver of a configured device. With driver
// "auto" (or none) the brand is detected on the first Connect.
func OpenDevice(reg *routerkit.Registry, d config.Device) (routerkit.Driver, error) {
	return reg.Open(d.Driver, d.DriverConfig())
}

// CheckDevice validates a configured device against the registry (what
// `routerctl config check` reports): a known driver, and options the
// driver accepts. With driver "auto" each option must be accepted by at
// least one driver (the detected driver checks them again on Connect).
func CheckDevice(reg *routerkit.Registry, d config.Device) error {
	name := d.Driver
	if name == "" || name == "auto" {
		var errs []error
		for _, k := range sortedKeys(d.Options) {
			var takers []string
			var last error
			for _, n := range reg.Names() {
				f, _ := reg.Lookup(n)
				if !hasOption(f.Options, k) {
					continue
				}
				if _, err := routerkit.BindOptions(f.Options, map[string]string{k: d.Options[k]}); err != nil {
					last = fmt.Errorf("%s: %w", n, err)
					continue
				}
				takers = append(takers, n)
			}
			switch {
			case len(takers) > 0:
			case last != nil:
				errs = append(errs, last)
			default:
				errs = append(errs, fmt.Errorf("unknown option %q (no driver accepts it)", k))
			}
		}
		if err := errors.Join(errs...); err != nil {
			return fmt.Errorf("device %s: %w", d.Name, err)
		}
		return nil
	}
	f, ok := reg.Lookup(name)
	if !ok {
		return fmt.Errorf("device %s: unknown driver %q (available: %v)", d.Name, name, reg.Names())
	}
	if _, err := routerkit.BindOptions(f.Options, d.Options); err != nil {
		return fmt.Errorf("device %s: driver %s: %w", d.Name, name, err)
	}
	return nil
}

func hasOption(specs []routerkit.OptionSpec, name string) bool {
	for _, o := range specs {
		if o.Name == name {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PollerDevice is the poller's view of a configured device.
func PollerDevice(d config.Device, drv routerkit.Driver) poller.Device {
	iv := map[routerkit.Part]time.Duration{}
	for _, p := range routerkit.AllParts {
		if v := d.Interval(p); v > 0 {
			iv[p] = v
		}
	}
	return poller.Device{Name: d.Name, Driver: drv, Intervals: iv, Timeout: d.Timeout.D()}
}

// PollerPresence maps the presence settings.
func PollerPresence(p config.Presence) poller.Presence {
	return poller.Presence{OfflineGrace: p.OfflineGrace.D(), OnlineDebounce: p.OnlineDebounce.D()}
}

// Policy is the command policy of a transport: the shared commands
// section, made stricter by the transport's own max_danger (a value
// config.Finish validated; "" keeps the shared level).
func Policy(c config.Commands, transportMax string) commands.Policy {
	base, _ := commands.ParseDanger(c.MaxDanger, commands.DefaultPolicy.MaxDanger)
	p := commands.Policy{MaxDanger: base, Allow: c.Allow, Deny: c.Deny}
	if transportMax != "" {
		d, _ := commands.ParseDanger(transportMax, base)
		p = p.Restrict(commands.Policy{MaxDanger: d})
	}
	return p
}

// MQTT maps the MQTT settings to the sink options and the broker
// connection (discovery: false clears the discovery prefix).
func MQTT(c config.MQTT, pol commands.Policy, version string) (mqtt.Options, mqtt.Broker) {
	o := mqtt.Options{
		Topics:          mqtt.Topics{Base: c.BaseTopic},
		DiscoveryPrefix: c.DiscoveryPrefix,
		Trackers:        c.Trackers,
		Commands:        c.CommandsEnabled(),
		Policy:          pol,
		BlockSwitches:   c.BlockSwitches,
		Version:         version,
		QoS:             byte(c.QoS),
	}
	if !c.DiscoveryEnabled() {
		o.DiscoveryPrefix = ""
	}
	b := mqtt.Broker{
		URL:       c.Broker,
		Username:  c.Username,
		Password:  c.Password.Value(),
		ClientID:  c.ClientID,
		WillTopic: c.BaseTopic + "/status",
		Insecure:  c.Insecure,
	}
	return o, b
}

// Notify maps one notification target.
func Notify(c config.Notify) notify.Options {
	return notify.Options{
		Type: c.Type, URL: c.URL, Topic: c.Topic, Token: c.Token.Value(),
		Events: c.Events, Devices: c.Devices, Title: c.Title, Message: c.Message,
		Priority: c.Priority, Tags: c.Tags, Headers: c.Headers, Timeout: c.Timeout.D(),
	}
}

// Sinks builds the enabled outputs. out receives the stdout sink. The
// HTTP server (when metrics or the API is on) comes last, so the
// handlers it serves are started first.
func Sinks(cfg *config.Config, version string, out io.Writer) ([]poller.Sink, error) {
	var sinks []poller.Sink
	if cfg.MQTT.Enabled {
		o, b := MQTT(cfg.MQTT, Policy(cfg.Commands, cfg.MQTT.MaxDanger), version)
		sinks = append(sinks, sink.Async(mqtt.New(o, mqtt.NewPaho(b)), 1024))
	}
	if len(cfg.Notify) > 0 {
		targets := make([]notify.Options, len(cfg.Notify))
		for i, n := range cfg.Notify {
			targets[i] = Notify(n)
		}
		n, err := notify.New(targets, nil)
		if err != nil {
			return nil, err
		}
		sinks = append(sinks, sink.Async(n, 256))
	}
	if cfg.Stdout.Enabled {
		sel := events.NewSelector(cfg.Stdout.Events, nil, nil, false)
		sinks = append(sinks, sink.Async(stdout.New(out, cfg.Stdout.Snapshots, sel), 1024))
	}
	if cfg.Metrics.Enabled || cfg.API.Enabled {
		srv := web.New(cfg.HTTP.Listen)
		token := cfg.HTTP.Token.Value()
		// The metrics sink also keeps the health state (/healthz)
		m := metrics.New(cfg.Metrics.PerClientEnabled())
		sinks = append(sinks, m)
		srv.Handle("/healthz", m.HealthHandler())
		if cfg.Metrics.Enabled {
			h := m.MetricsHandler()
			if cfg.HTTP.ProtectMetrics {
				h = web.RequireToken(token, h)
			}
			srv.Handle("/metrics", h)
		}
		if cfg.API.Enabled {
			a := api.New(api.Options{
				Token:   token,
				Policy:  Policy(cfg.Commands, cfg.API.MaxDanger),
				Buffer:  cfg.API.Buffer,
				Version: version,
			})
			// The API answers queries itself: not wrapped in Async, its
			// State/Event only queue frames without blocking
			sinks = append(sinks, a)
			srv.Handle(api.Prefix+"/", a.Handler())
			if token == "" {
				slog.Warn("api: no http.token; only local clients can reach it", "listen", cfg.HTTP.Listen)
			}
		}
		sinks = append(sinks, srv)
	}
	for _, s := range sinks {
		slog.Debug("output enabled", "sink", s.Name())
	}
	return sinks, nil
}
