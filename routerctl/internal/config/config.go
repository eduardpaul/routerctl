// Package config loads the routerctl configuration: a YAML file, then
// ROUTERCTL_* environment variables, then command-line flags (applied by
// the caller). Secrets can come from files (Docker secrets).
//
// Secrets are never printed: String / Redacted mask every password and
// token, and errors never include secret values.
package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/eduardpaul/routerctl/routerkit"
)

// Duration is a time.Duration that reads "30s", "5m" or a number of seconds.
type Duration time.Duration

// D returns the time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalYAML accepts "30s" style strings and plain seconds.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML writes the duration as a string.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// ParseDuration parses "30s" / "5m" / "1h30m" or a plain number of seconds.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(n * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// Secret is a string that never prints its value.
type Secret string

// String masks the secret.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return "********"
}

// GoString masks the secret (for %#v).
func (s Secret) GoString() string { return `config.Secret("********")` }

// MarshalYAML masks the secret in config dumps.
func (s Secret) MarshalYAML() (any, error) { return s.String(), nil }

// MarshalJSON masks the secret in JSON dumps.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(s.String())), nil }

// Value returns the real secret.
func (s Secret) Value() string { return string(s) }

// Device is one router / gateway to talk to.
type Device struct {
	Name         string            `yaml:"name"`
	Driver       string            `yaml:"driver"` // auto | asus | zte | fake
	Host         string            `yaml:"host"`
	Username     string            `yaml:"username"`
	Password     Secret            `yaml:"password"`
	PasswordFile string            `yaml:"password_file"`
	SSL          bool              `yaml:"ssl"`
	Port         int               `yaml:"port"`
	PinSHA256    string            `yaml:"pin_sha256"`
	Timeout      Duration          `yaml:"timeout"`
	Options      map[string]string `yaml:"options"`
	// Intervals per model part (clients, wan, system, traffic, nodes,
	// firmware); missing parts use Defaults; 0 or a negative value
	// disables a part
	Intervals map[string]Duration `yaml:"intervals"`
}

// DriverConfig converts to the driver factory config (resolving secrets).
func (d Device) DriverConfig() routerkit.Config {
	return routerkit.Config{
		Name:      d.Name,
		Host:      d.Host,
		Username:  d.Username,
		Password:  d.Password.Value(),
		UseSSL:    d.SSL,
		Port:      d.Port,
		PinSHA256: d.PinSHA256,
		Timeout:   d.Timeout.D(),
		Options:   d.Options,
	}
}

// Interval returns the poll interval of a part (0 = disabled).
func (d Device) Interval(p routerkit.Part) time.Duration {
	if v, ok := d.Intervals[string(p)]; ok {
		if v <= 0 {
			return 0
		}
		return v.D()
	}
	return DefaultIntervals[p]
}

// DefaultIntervals are the default poll intervals per part.
var DefaultIntervals = map[routerkit.Part]time.Duration{
	routerkit.PartClients:  30 * time.Second,
	routerkit.PartWAN:      30 * time.Second,
	routerkit.PartSystem:   60 * time.Second,
	routerkit.PartTraffic:  30 * time.Second,
	routerkit.PartNodes:    2 * time.Minute,
	routerkit.PartFirmware: 6 * time.Hour,
}

// Log configures logging.
type Log struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// Presence configures online/offline debouncing.
type Presence struct {
	// OfflineGrace: a client must stay offline this long before
	// device_offline fires (a sleeping phone drops WiFi briefly)
	OfflineGrace Duration `yaml:"offline_grace"`
	// OnlineDebounce: a client must stay online this long before
	// device_online fires
	OnlineDebounce Duration `yaml:"online_debounce"`
}

// Inventory configures the persistent device inventory.
type Inventory struct {
	// Baseline: on the first run (empty inventory) record every client
	// as known without firing device_new for each of them
	Baseline *bool `yaml:"baseline"`
}

// BaselineEnabled reports the effective baseline setting (default true).
func (i Inventory) BaselineEnabled() bool { return i.Baseline == nil || *i.Baseline }

// MQTT configures the MQTT sink.
type MQTT struct {
	Enabled         bool   `yaml:"enabled"`
	Broker          string `yaml:"broker"` // tcp://host:1883, ssl://host:8883, ws://...
	Username        string `yaml:"username"`
	Password        Secret `yaml:"password"`
	PasswordFile    string `yaml:"password_file"`
	ClientID        string `yaml:"client_id"`
	BaseTopic       string `yaml:"base_topic"`
	Discovery       *bool  `yaml:"discovery"`
	DiscoveryPrefix string `yaml:"discovery_prefix"`
	// Trackers: which clients get a device_tracker entity:
	// known (approved or labelled; default) | all | none
	Trackers string `yaml:"trackers"`
	// Commands enables the command topics (driver actions)
	Commands *bool `yaml:"commands"`
	// MaxDanger: the most disruptive action command topics may run
	// (safe | disruptive | destructive); only stricter than commands
	MaxDanger string `yaml:"max_danger"`
	// BlockSwitches adds a "block internet" switch per tracked client
	BlockSwitches bool `yaml:"block_switches"`
	QoS           int  `yaml:"qos"`
	// TLS: skip broker certificate verification (self-signed brokers)
	Insecure bool `yaml:"insecure"`
}

// DiscoveryEnabled reports the effective discovery setting (default true).
func (m MQTT) DiscoveryEnabled() bool { return m.Discovery == nil || *m.Discovery }

// CommandsEnabled reports the effective commands setting (default true).
func (m MQTT) CommandsEnabled() bool { return m.Commands == nil || *m.Commands }

// Notify is one notification target.
type Notify struct {
	Type      string            `yaml:"type"` // ntfy | gotify | webhook
	URL       string            `yaml:"url"`
	Topic     string            `yaml:"topic"` // ntfy
	Token     Secret            `yaml:"token"`
	TokenFile string            `yaml:"token_file"`
	Events    []string          `yaml:"events"`  // empty = every event
	Devices   []string          `yaml:"devices"` // empty = every device
	Title     string            `yaml:"title"`   // text/template
	Message   string            `yaml:"message"` // text/template
	Priority  int               `yaml:"priority"`
	Tags      []string          `yaml:"tags"`    // ntfy
	Headers   map[string]string `yaml:"headers"` // webhook
	Timeout   Duration          `yaml:"timeout"`
}

// Metrics configures the Prometheus endpoint (/metrics on the HTTP
// server).
type Metrics struct {
	Enabled bool `yaml:"enabled"`
	// PerClient exports one series per client (online, rssi)
	PerClient *bool `yaml:"per_client"`
}

// PerClientEnabled reports the effective per_client setting (default true).
func (m Metrics) PerClientEnabled() bool { return m.PerClient == nil || *m.PerClient }

// Stdout configures the JSON-lines sink.
type Stdout struct {
	Enabled bool `yaml:"enabled"`
	// Snapshots also prints every poll result
	Snapshots bool `yaml:"snapshots"`
	// Events: only these event types (empty = every event)
	Events []string `yaml:"events"`
}

// HTTP configures the one HTTP server of the daemon: /healthz always,
// /metrics with metrics.enabled, /api/v1 with api.enabled.
type HTTP struct {
	// Listen: host:port (default 127.0.0.1:9100)
	Listen string `yaml:"listen"`
	// Token: the bearer token of /api (required unless Listen is a
	// loopback address)
	Token     Secret `yaml:"token"`
	TokenFile string `yaml:"token_file"`
	// ProtectMetrics also requires the token on /metrics
	ProtectMetrics bool `yaml:"protect_metrics"`
}

// Commands is the command policy shared by every transport (MQTT, API,
// CLI); a transport can only make it stricter.
type Commands struct {
	// MaxDanger: safe | disruptive (default) | destructive
	MaxDanger string `yaml:"max_danger"`
	// Allow: when set, only these actions
	Allow []string `yaml:"allow"`
	// Deny: never these actions
	Deny []string `yaml:"deny"`
}

// API configures the HTTP API (REST + Server-Sent Events), the transport
// of the Node-RED nodes.
type API struct {
	Enabled bool `yaml:"enabled"`
	// MaxDanger: stricter than commands.max_danger for the API only
	MaxDanger string `yaml:"max_danger"`
	// Buffer: events kept for stream resumption (default 500)
	Buffer int `yaml:"buffer"`
}

// Config is the whole file.
type Config struct {
	Log       Log       `yaml:"log"`
	StateFile string    `yaml:"state_file"`
	Devices   []Device  `yaml:"devices"`
	Presence  Presence  `yaml:"presence"`
	Inventory Inventory `yaml:"inventory"`
	MQTT      MQTT      `yaml:"mqtt"`
	Notify    []Notify  `yaml:"notify"`
	Metrics   Metrics   `yaml:"metrics"`
	Stdout    Stdout    `yaml:"stdout"`
	HTTP      HTTP      `yaml:"http"`
	Commands  Commands  `yaml:"commands"`
	API       API       `yaml:"api"`
}

// Defaults applied by Load.
const (
	DefaultStateFile       = "routerctl-state.json"
	DefaultBaseTopic       = "routerctl"
	DefaultDiscoveryPrefix = "homeassistant"
	DefaultHTTPListen      = "127.0.0.1:9100"
	DefaultOfflineGrace    = 3 * time.Minute
	DefaultTimeout         = 30 * time.Second
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// Parse reads a YAML document (no env, no defaults).
func Parse(data []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

// Load reads the file at path (optional when empty or missing and
// mustExist is false), applies ROUTERCTL_* env vars from getenv, resolves
// secret files, fills defaults and validates.
func Load(path string, mustExist bool, getenv func(string) string) (*Config, error) {
	c, err := LoadRaw(path, mustExist, getenv)
	if err != nil {
		return nil, err
	}
	if err := c.Finish(); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadRaw is Load without Finish (for callers that apply flags first).
func LoadRaw(path string, mustExist bool, getenv func(string) string) (*Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	c := &Config{}
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if c, err = Parse(data); err != nil {
				return nil, err
			}
		case os.IsNotExist(err) && !mustExist:
		default:
			return nil, fmt.Errorf("config: %w", err)
		}
	}
	if err := c.ApplyEnv(getenv); err != nil {
		return nil, err
	}
	return c, nil
}

// ApplyEnv applies ROUTERCTL_* variables. The single-device variables
// (ROUTERCTL_HOST, ...) create the device when the file has none, update
// it when the file has exactly one, and otherwise update (or add) the
// device named ROUTERCTL_NAME (default "router").
func (c *Config) ApplyEnv(getenv func(string) string) error {
	env := func(k string) (string, bool) {
		v := getenv("ROUTERCTL_" + k)
		return v, v != ""
	}
	var errs []error
	boolEnv := func(k string, dst *bool) {
		if v, ok := env(k); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("ROUTERCTL_%s: not a boolean", k))
				return
			}
			*dst = b
		}
	}
	strEnv := func(k string, dst *string) {
		if v, ok := env(k); ok {
			*dst = v
		}
	}

	// Single device
	devKeys := []string{"HOST", "USERNAME", "PASSWORD", "PASSWORD_FILE", "DRIVER", "SSL", "PORT", "PIN_SHA256", "TIMEOUT"}
	anySet := false
	for _, k := range devKeys {
		if _, ok := env(k); ok {
			anySet = true
		}
	}
	if anySet {
		name, _ := env("NAME")
		var d *Device
		switch {
		case len(c.Devices) == 0:
			c.Devices = append(c.Devices, Device{Name: name})
			d = &c.Devices[0]
		case len(c.Devices) == 1 && (name == "" || name == c.Devices[0].Name):
			d = &c.Devices[0]
		default:
			if name == "" {
				if _, hasHost := env("HOST"); !hasHost {
					return errors.New("several devices are configured: set ROUTERCTL_NAME to pick the one the ROUTERCTL_* device variables apply to")
				}
				name = "router"
			}
			for i := range c.Devices {
				if c.Devices[i].Name == name {
					d = &c.Devices[i]
				}
			}
			if d == nil {
				c.Devices = append(c.Devices, Device{Name: name})
				d = &c.Devices[len(c.Devices)-1]
			}
		}
		strEnv("HOST", &d.Host)
		strEnv("USERNAME", &d.Username)
		if v, ok := env("PASSWORD"); ok {
			d.Password = Secret(v)
		}
		strEnv("PASSWORD_FILE", &d.PasswordFile)
		strEnv("DRIVER", &d.Driver)
		boolEnv("SSL", &d.SSL)
		strEnv("PIN_SHA256", &d.PinSHA256)
		if v, ok := env("PORT"); ok {
			p, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, errors.New("ROUTERCTL_PORT: not a number"))
			}
			d.Port = p
		}
		if v, ok := env("TIMEOUT"); ok {
			t, err := ParseDuration(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("ROUTERCTL_TIMEOUT: %w", err))
			}
			d.Timeout = Duration(t)
		}
	}

	strEnv("STATE_FILE", &c.StateFile)
	strEnv("LOG_LEVEL", &c.Log.Level)
	strEnv("LOG_FORMAT", &c.Log.Format)
	if v, ok := env("OFFLINE_GRACE"); ok {
		t, err := ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ROUTERCTL_OFFLINE_GRACE: %w", err))
		}
		c.Presence.OfflineGrace = Duration(t)
	}

	// MQTT
	if v, ok := env("MQTT_BROKER"); ok {
		c.MQTT.Broker = v
		c.MQTT.Enabled = true
	}
	boolEnv("MQTT_ENABLED", &c.MQTT.Enabled)
	strEnv("MQTT_USERNAME", &c.MQTT.Username)
	if v, ok := env("MQTT_PASSWORD"); ok {
		c.MQTT.Password = Secret(v)
	}
	strEnv("MQTT_PASSWORD_FILE", &c.MQTT.PasswordFile)
	strEnv("MQTT_CLIENT_ID", &c.MQTT.ClientID)
	strEnv("MQTT_BASE_TOPIC", &c.MQTT.BaseTopic)
	strEnv("MQTT_DISCOVERY_PREFIX", &c.MQTT.DiscoveryPrefix)
	strEnv("MQTT_TRACKERS", &c.MQTT.Trackers)
	if v, ok := env("MQTT_DISCOVERY"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, errors.New("ROUTERCTL_MQTT_DISCOVERY: not a boolean"))
		}
		c.MQTT.Discovery = &b
	}
	strEnv("MQTT_MAX_DANGER", &c.MQTT.MaxDanger)

	// Metrics, HTTP server, API, command policy
	boolEnv("METRICS_ENABLED", &c.Metrics.Enabled)
	strEnv("HTTP_LISTEN", &c.HTTP.Listen)
	if v, ok := env("HTTP_TOKEN"); ok {
		c.HTTP.Token = Secret(v)
	}
	strEnv("HTTP_TOKEN_FILE", &c.HTTP.TokenFile)
	boolEnv("API_ENABLED", &c.API.Enabled)
	strEnv("API_MAX_DANGER", &c.API.MaxDanger)
	strEnv("COMMANDS_MAX_DANGER", &c.Commands.MaxDanger)

	// Stdout
	boolEnv("STDOUT", &c.Stdout.Enabled)

	// Notifications (one of each kind from env)
	split := func(v string) []string {
		var out []string
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if v, ok := env("NTFY_URL"); ok {
		n := Notify{Type: "ntfy", URL: v}
		strEnv("NTFY_TOPIC", &n.Topic)
		if t, ok := env("NTFY_TOKEN"); ok {
			n.Token = Secret(t)
		}
		strEnv("NTFY_TOKEN_FILE", &n.TokenFile)
		if e, ok := env("NTFY_EVENTS"); ok {
			n.Events = split(e)
		}
		c.Notify = append(c.Notify, n)
	}
	if v, ok := env("GOTIFY_URL"); ok {
		n := Notify{Type: "gotify", URL: v}
		if t, ok := env("GOTIFY_TOKEN"); ok {
			n.Token = Secret(t)
		}
		strEnv("GOTIFY_TOKEN_FILE", &n.TokenFile)
		if e, ok := env("GOTIFY_EVENTS"); ok {
			n.Events = split(e)
		}
		c.Notify = append(c.Notify, n)
	}
	if v, ok := env("WEBHOOK_URL"); ok {
		n := Notify{Type: "webhook", URL: v}
		if e, ok := env("WEBHOOK_EVENTS"); ok {
			n.Events = split(e)
		}
		c.Notify = append(c.Notify, n)
	}
	return errors.Join(errs...)
}

// readSecretFile reads a secret file and trims the trailing newline. The
// error names the file, never the content.
func readSecretFile(path string) (Secret, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret file: %w", err)
	}
	return Secret(strings.TrimRight(string(data), "\r\n")), nil
}

// Finish resolves secret files, applies defaults and validates.
func (c *Config) Finish() error {
	var errs []error
	if c.StateFile == "" {
		c.StateFile = DefaultStateFile
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("log.level: %q is not debug|info|warn|error", c.Log.Level))
	}
	switch c.Log.Format {
	case "text", "json":
	default:
		errs = append(errs, fmt.Errorf("log.format: %q is not text|json", c.Log.Format))
	}
	if c.Presence.OfflineGrace == 0 {
		c.Presence.OfflineGrace = Duration(DefaultOfflineGrace)
	}

	seen := map[string]bool{}
	for i := range c.Devices {
		d := &c.Devices[i]
		if d.Name == "" {
			if len(c.Devices) == 1 {
				d.Name = "router"
			} else {
				d.Name = fmt.Sprintf("router%d", i+1)
			}
		}
		if !nameRe.MatchString(d.Name) {
			errs = append(errs, fmt.Errorf("devices[%d].name: %q must be letters, digits, '-' or '_'", i, d.Name))
		}
		if seen[d.Name] {
			errs = append(errs, fmt.Errorf("devices[%d].name: duplicate %q", i, d.Name))
		}
		seen[d.Name] = true
		if d.Driver == "" {
			d.Driver = "auto"
		}
		if d.Host == "" && d.Driver != "fake" {
			errs = append(errs, fmt.Errorf("devices[%d] (%s): host is required", i, d.Name))
		}
		if d.PasswordFile != "" && d.Password == "" {
			s, err := readSecretFile(d.PasswordFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("devices[%d] (%s): password_file: %w", i, d.Name, err))
			}
			d.Password = s
		}
		if d.Timeout == 0 {
			d.Timeout = Duration(DefaultTimeout)
		}
		if d.Port < 0 || d.Port > 65535 {
			errs = append(errs, fmt.Errorf("devices[%d] (%s): port %d out of range", i, d.Name, d.Port))
		}
		for k := range d.Intervals {
			if !validPart(k) {
				errs = append(errs, fmt.Errorf("devices[%d] (%s): intervals: unknown part %q", i, d.Name, k))
			}
		}
	}

	m := &c.MQTT
	if m.BaseTopic == "" {
		m.BaseTopic = DefaultBaseTopic
	}
	m.BaseTopic = strings.Trim(m.BaseTopic, "/")
	if m.DiscoveryPrefix == "" {
		m.DiscoveryPrefix = DefaultDiscoveryPrefix
	}
	if m.Trackers == "" {
		m.Trackers = "known"
	}
	switch m.Trackers {
	case "known", "all", "none":
	default:
		errs = append(errs, fmt.Errorf("mqtt.trackers: %q is not known|all|none", m.Trackers))
	}
	if m.PasswordFile != "" && m.Password == "" {
		s, err := readSecretFile(m.PasswordFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("mqtt.password_file: %w", err))
		}
		m.Password = s
	}
	if m.Enabled && m.Broker == "" {
		errs = append(errs, errors.New("mqtt.broker is required when mqtt is enabled"))
	}
	if m.QoS < 0 || m.QoS > 2 {
		errs = append(errs, fmt.Errorf("mqtt.qos: %d is not 0, 1 or 2", m.QoS))
	}
	if strings.ContainsAny(m.BaseTopic, "+#") || strings.ContainsAny(m.DiscoveryPrefix, "+#") {
		errs = append(errs, errors.New("mqtt: base_topic and discovery_prefix cannot contain + or #"))
	}

	for i := range c.Notify {
		n := &c.Notify[i]
		switch n.Type {
		case "ntfy":
			if n.Topic == "" && !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(n.URL, "https://"), "http://"), "/") {
				errs = append(errs, fmt.Errorf("notify[%d]: ntfy needs a topic (or a URL with the topic path)", i))
			}
		case "gotify", "webhook":
		default:
			errs = append(errs, fmt.Errorf("notify[%d].type: %q is not ntfy|gotify|webhook", i, n.Type))
		}
		if n.URL == "" {
			errs = append(errs, fmt.Errorf("notify[%d] (%s): url is required", i, n.Type))
		}
		if n.TokenFile != "" && n.Token == "" {
			s, err := readSecretFile(n.TokenFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("notify[%d].token_file: %w", i, err))
			}
			n.Token = s
		}
		if n.Type == "gotify" && n.Token == "" {
			errs = append(errs, fmt.Errorf("notify[%d]: gotify needs a token", i))
		}
		if n.Timeout == 0 {
			n.Timeout = Duration(10 * time.Second)
		}
	}

	h := &c.HTTP
	if h.Listen == "" {
		h.Listen = DefaultHTTPListen
	}
	if h.TokenFile != "" && h.Token == "" {
		s, err := readSecretFile(h.TokenFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("http.token_file: %w", err))
		}
		h.Token = s
	}
	if _, _, err := net.SplitHostPort(h.Listen); err != nil {
		errs = append(errs, fmt.Errorf("http.listen: %q is not host:port", h.Listen))
	} else if c.API.Enabled && h.Token == "" && !Loopback(h.Listen) {
		errs = append(errs, fmt.Errorf("api: http.token (or token_file / ROUTERCTL_HTTP_TOKEN) is required when http.listen (%s) is not a loopback address", h.Listen))
	}
	if h.ProtectMetrics && h.Token == "" {
		errs = append(errs, errors.New("http.protect_metrics needs http.token"))
	}
	if c.Commands.MaxDanger == "" {
		c.Commands.MaxDanger = "disruptive"
	}
	for name, v := range map[string]string{"commands.max_danger": c.Commands.MaxDanger, "mqtt.max_danger": m.MaxDanger, "api.max_danger": c.API.MaxDanger} {
		if v == "" {
			continue
		}
		var d routerkit.Danger
		if err := d.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not safe|disruptive|destructive", name, v))
		}
	}
	if c.API.Buffer < 0 {
		errs = append(errs, fmt.Errorf("api.buffer: %d is negative", c.API.Buffer))
	}
	for _, t := range c.Stdout.Events {
		if !validEvent(t) {
			errs = append(errs, fmt.Errorf("stdout.events: unknown event %q", t))
		}
	}
	return errors.Join(errs...)
}

// Loopback reports whether a listen address only accepts local
// connections (127.0.0.0/8, ::1, localhost).
func Loopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// validEvent mirrors service/events (config only imports the core).
func validEvent(t string) bool {
	switch t {
	case "device_new", "device_online", "device_offline", "device_ip_changed", "wan_down", "wan_up",
		"wan_ip_changed", "firmware_update", "node_offline", "node_online", "action_done":
		return true
	}
	return false
}

func validPart(name string) bool {
	for _, p := range routerkit.AllParts {
		if string(p) == name {
			return true
		}
	}
	return false
}

// Device returns the device with the given name; with an empty name it
// returns the only device (an error when there are several).
func (c *Config) Device(name string) (*Device, error) {
	if name == "" {
		switch len(c.Devices) {
		case 0:
			return nil, errors.New("no device configured (use --host, ROUTERCTL_HOST or a config file)")
		case 1:
			return &c.Devices[0], nil
		default:
			names := make([]string, len(c.Devices))
			for i, d := range c.Devices {
				names[i] = d.Name
			}
			sort.Strings(names)
			return nil, fmt.Errorf("several devices configured, pick one with -d (%s)", strings.Join(names, ", "))
		}
	}
	for i := range c.Devices {
		if c.Devices[i].Name == name {
			return &c.Devices[i], nil
		}
	}
	return nil, fmt.Errorf("no device named %q in the config", name)
}

// Redacted renders the config as YAML with every secret masked.
func (c *Config) Redacted() string {
	out, err := yaml.Marshal(c)
	if err != nil {
		return "# " + err.Error()
	}
	return string(out)
}
