// Command routerctl is a multi-vendor home-router CLI and daemon: one-shot
// reads and actions, and a watch daemon that feeds MQTT (with Home
// Assistant discovery), notifications, Prometheus metrics and stdout.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/apiclient"
	"github.com/eduardpaul/routerctl/routerctl/internal/app"
	"github.com/eduardpaul/routerctl/routerctl/internal/config"
	"github.com/eduardpaul/routerctl/routerctl/internal/output"
	"github.com/eduardpaul/routerctl/routerkit"
)

// version is set with -ldflags "-X main.version=..."
var version = "dev"

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

var commands []command

func init() {
	commands = []command{
		{"detect", "name the brand of a device (no login)", cmdDetect},
		{"sources", "list the native data sources", cmdSources},
		{"get", "read native sources", cmdGet},
		{"status", "identity, WAN and system state", cmdStatus},
		{"clients", "the client table", cmdClients},
		{"nodes", "mesh nodes / access points", cmdNodes},
		{"do", "run an action (key=value args)", cmdDo},
		{"devices", "the persistent device inventory", cmdDevices},
		{"watch", "run the daemon (MQTT, notifications, metrics)", cmdWatch},
		{"capture", "record a session to a file (ASUS)", cmdCapture},
		{"replay", "read sources from a recorded session (ASUS)", cmdReplay},
		{"config", "config example | show | check", cmdConfig},
		{"version", "print the version", cmdVersion},
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `routerctl %s: multi-vendor home-router CLI and daemon

Usage: routerctl <command> [flags] [args]

Commands:
`, version)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-9s %s\n", c.name, c.summary)
	}
	fmt.Fprint(w, `
Device selection (most commands):
  -c, --config FILE     config file (or ROUTERCTL_CONFIG; default ./routerctl.yaml,
                        ~/.config/routerctl/config.yaml, /etc/routerctl/config.yaml)
  -d, --device NAME     device from the config file
  --host H --user U --driver auto|asus|zte|fake   ad-hoc device
  password: ROUTERCTL_PASSWORD, --password-file, or --password (visible in ps)
  -o, --output table|json|csv
  --via URL             do, clients, devices: go through a running "watch --api"
                        (its device session and policy; token: ROUTERCTL_HTTP_TOKEN)

Run "routerctl <command> -h" for the flags of a command.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	name := os.Args[1]
	switch name {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	case "-v", "--version":
		name = "version"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for _, c := range commands {
		if c.name == name {
			err := c.run(ctx, os.Args[2:])
			if err != nil {
				if errors.Is(err, flag.ErrHelp) {
					return
				}
				var ue usageError
				if errors.As(err, &ue) {
					fmt.Fprintf(os.Stderr, "routerctl %s: %v\n", name, err)
					os.Exit(2)
				}
				fmt.Fprintf(os.Stderr, "routerctl %s: %v\n", name, err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "routerctl: unknown command %q\n\n", name)
	usage(os.Stderr)
	os.Exit(2)
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

// parse parses flags interspersed with positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func newFlagSet(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: routerctl %s %s\n\nFlags:\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// ---- device selection ----

type devFlags struct {
	config, device               string
	host, user, password, pwFile string
	drv                          string
	ssl                          bool
	port                         int
	pin                          string
	timeout                      time.Duration
	output                       string
	logLevel                     string
	via, tokenFile               string
}

func addDevFlags(fs *flag.FlagSet, withOutput bool) *devFlags {
	f := &devFlags{}
	fs.StringVar(&f.config, "config", "", "config file")
	fs.StringVar(&f.config, "c", "", "config file (shorthand)")
	fs.StringVar(&f.device, "device", "", "device name from the config")
	fs.StringVar(&f.device, "d", "", "device name (shorthand)")
	fs.StringVar(&f.host, "host", "", "ad-hoc device host")
	fs.StringVar(&f.user, "user", "", "username (default admin)")
	fs.StringVar(&f.user, "username", "", "username")
	fs.StringVar(&f.password, "password", "", "password (prefer ROUTERCTL_PASSWORD or --password-file)")
	fs.StringVar(&f.pwFile, "password-file", "", "file holding the password")
	fs.StringVar(&f.drv, "driver", "", "driver: auto | "+strings.Join(registry().Names(), " | "))
	fs.BoolVar(&f.ssl, "ssl", false, "use HTTPS")
	fs.IntVar(&f.port, "port", 0, "port (0 = brand default)")
	fs.StringVar(&f.pin, "pin-sha256", "", "TLS certificate SHA-256 pin")
	fs.DurationVar(&f.timeout, "timeout", 0, "request timeout (default 30s)")
	fs.StringVar(&f.logLevel, "log-level", "", "debug | info | warn | error (default warn for one-shot commands)")
	fs.StringVar(&f.via, "via", os.Getenv("ROUTERCTL_VIA"), "run through a running daemon's API (http://host:9100; or ROUTERCTL_VIA)")
	fs.StringVar(&f.tokenFile, "token-file", "", "file holding the API token for --via (or ROUTERCTL_HTTP_TOKEN)")
	if withOutput {
		fs.StringVar(&f.output, "output", "table", "output: table | json | csv")
		fs.StringVar(&f.output, "o", "table", "output (shorthand)")
	}
	return f
}

func configPath(explicit string) (string, bool) {
	if explicit != "" {
		return explicit, true
	}
	if v := os.Getenv("ROUTERCTL_CONFIG"); v != "" {
		return v, true
	}
	cands := []string{"routerctl.yaml", "routerctl.yml"}
	if dir, err := os.UserConfigDir(); err == nil {
		cands = append(cands, filepath.Join(dir, "routerctl", "config.yaml"))
	}
	cands = append(cands, "/etc/routerctl/config.yaml")
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

// loadConfig loads the config and applies the device flags.
func (f *devFlags) loadConfig() (*config.Config, error) {
	path, must := configPath(f.config)
	cfg, err := config.LoadRaw(path, must, os.Getenv)
	if err != nil {
		return nil, err
	}
	var d *config.Device
	if f.host != "" {
		name := f.device
		for i := range cfg.Devices {
			if name != "" && cfg.Devices[i].Name == name {
				d = &cfg.Devices[i]
			}
		}
		// Env-only credentials (ROUTERCTL_PASSWORD, no host) go with --host
		for i := range cfg.Devices {
			if d == nil && name == "" && cfg.Devices[i].Host == "" {
				d = &cfg.Devices[i]
			}
		}
		if name == "" {
			name = "router"
		}
		if d != nil && d.Name == "" {
			d.Name = name
		}
		if d == nil {
			cfg.Devices = append(cfg.Devices, config.Device{Name: name, Driver: "auto"})
			d = &cfg.Devices[len(cfg.Devices)-1]
		}
		d.Host = f.host
		if f.device == "" {
			f.device = name
		}
	} else if f.device != "" || len(cfg.Devices) == 1 {
		if d, err = cfg.Device(f.device); err != nil {
			return nil, err
		}
	}
	if d != nil {
		if f.user != "" {
			d.Username = f.user
		}
		if f.password != "" {
			d.Password = config.Secret(f.password)
		}
		if f.pwFile != "" {
			d.PasswordFile, d.Password = f.pwFile, ""
		}
		if f.drv != "" {
			d.Driver = f.drv
		}
		if f.ssl {
			d.SSL = true
		}
		if f.port != 0 {
			d.Port = f.port
		}
		if f.pin != "" {
			d.PinSHA256 = f.pin
		}
		if f.timeout != 0 {
			d.Timeout = config.Duration(f.timeout)
		}
		if d.Username == "" {
			d.Username = "admin"
		}
	}
	if err := cfg.Finish(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// device returns the selected device config.
func (f *devFlags) selected(cfg *config.Config) (*config.Device, error) {
	return cfg.Device(f.device)
}

// connect opens and connects the selected device.
func (f *devFlags) connect(ctx context.Context) (routerkit.Driver, *config.Config, *config.Device, error) {
	setupLogging(f.logLevel, "warn", "text")
	cfg, err := f.loadConfig()
	if err != nil {
		return nil, nil, nil, err
	}
	d, err := f.selected(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	drv, err := app.OpenDevice(registry(), *d)
	if err != nil {
		return nil, nil, nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 2*d.Timeout.D())
	defer cancel()
	if err := drv.Connect(cctx); err != nil {
		return nil, nil, nil, err
	}
	return drv, cfg, d, nil
}

func (f *devFlags) format() (output.Format, error) { return output.ParseFormat(f.output) }

// viaClient returns the API client of --via (nil without --via).
func (f *devFlags) viaClient() (*apiclient.Client, error) {
	if f.via == "" {
		return nil, nil
	}
	setupLogging(f.logLevel, "warn", "text")
	token := os.Getenv("ROUTERCTL_HTTP_TOKEN")
	if f.tokenFile != "" {
		b, err := os.ReadFile(f.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("--token-file: %w", err)
		}
		token = strings.TrimRight(string(b), "\r\n")
	}
	return apiclient.New(f.via, token)
}

// viaDevice picks the daemon device: --device, else the only one.
func (f *devFlags) viaDevice(ctx context.Context, c *apiclient.Client) (string, error) {
	if f.device != "" {
		return f.device, nil
	}
	info, err := c.Info(ctx)
	if err != nil {
		return "", err
	}
	if len(info.Devices) != 1 {
		return "", usagef("the daemon has %d devices (%s): pick one with -d", len(info.Devices), strings.Join(info.Devices, ", "))
	}
	return info.Devices[0], nil
}

func closeQuiet(drv routerkit.Driver) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = drv.Close(ctx)
}

// ---- logging ----

func setupLogging(level, def, format string) {
	if level == "" {
		level = def
	}
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "info":
		lv = slog.LevelInfo
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelWarn
	}
	opts := &slog.HandlerOptions{Level: lv}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

// ---- version / config ----

func buildVersion() string {
	v := version
	if v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					v = "dev-" + s.Value[:7]
				}
			}
		}
	}
	return v
}

func cmdVersion(_ context.Context, args []string) error {
	fs := newFlagSet("version", "")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	fmt.Printf("routerctl %s (%s/%s, %s) drivers: %s\n", buildVersion(), runtime.GOOS, runtime.GOARCH, runtime.Version(), strings.Join(registry().Names(), ", "))
	return nil
}

func cmdConfig(_ context.Context, args []string) error {
	fs := newFlagSet("config", "example | show | check")
	path := fs.String("config", "", "config file")
	fs.StringVar(path, "c", "", "config file (shorthand)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("want one of: example, show, check")
	}
	switch pos[0] {
	case "example":
		fmt.Print(config.Example)
		return nil
	case "show", "check":
		p, must := configPath(*path)
		cfg, err := config.Load(p, must, os.Getenv)
		if err != nil {
			return err
		}
		if pos[0] == "check" {
			names := make([]string, 0, len(cfg.Devices))
			for _, d := range cfg.Devices {
				if err := app.CheckDevice(registry(), d); err != nil {
					return err
				}
				names = append(names, d.Name+" ("+d.Driver+")")
			}
			sort.Strings(names)
			src := p
			if src == "" {
				src = "environment only"
			}
			fmt.Printf("config OK (%s): %d device(s): %s\n", src, len(cfg.Devices), strings.Join(names, ", "))
			return nil
		}
		// Secrets are masked by config.Secret
		fmt.Print(cfg.Redacted())
		return nil
	}
	return usagef("unknown config subcommand %q", pos[0])
}
