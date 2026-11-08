// Package routerkit is the vendor-neutral contract between the `routerctl`
// CLI and the per-brand libraries (ASUS: package asusrouter; ZTE: package
// zte). Everything the daemon does (inventory, presence, MQTT, alerts,
// metrics) works on the normalized model below, so it behaves the same on
// every brand. Brand-specific data stays reachable through Get/Do.
package routerkit

import (
	"context"
	"fmt"
	"time"
)

// Identity describes the device.
type Identity struct {
	Vendor   string `json:"vendor"`   // DriverInfo.Vendor: "ASUS", "ZTE"
	Model    string `json:"model"`    // "RT-AX88U_PRO", "F8648P"
	Firmware string `json:"firmware"` // as reported
	Serial   string `json:"serial,omitempty"`
	MAC      string `json:"mac,omitempty"` // lower-case, colon separated
	Hostname string `json:"hostname,omitempty"`
	// Variant is the sub-driver or generation ("stock", "merlin", "lua")
	Variant string `json:"variant,omitempty"`
}

// Client is a device seen by the router (LAN or WiFi).
type Client struct {
	MAC    string `json:"mac"` // key: lower-case, colon separated
	IP     string `json:"ip,omitempty"`
	Name   string `json:"name,omitempty"`
	Vendor string `json:"vendor,omitempty"`
	Online bool   `json:"online"`
	// Connection: "wired", "2.4ghz", "5ghz", "6ghz", "wifi" (band unknown)
	// or "" when unknown
	Connection string `json:"connection,omitempty"`
	// Node is the mesh node / access point the client is attached to
	Node string `json:"node,omitempty"`
	// RSSI in dBm, link rates in Mbit/s (nil when unknown)
	RSSI   *int     `json:"rssi,omitempty"`
	RxRate *float64 `json:"rx_rate,omitempty"`
	TxRate *float64 `json:"tx_rate,omitempty"`
	// Since: when the current connection started, if the device knows
	Since *time.Time `json:"since,omitempty"`
	// Extra: brand-specific fields, JSON-able
	Extra map[string]any `json:"extra,omitempty"`
}

// WAN is the internet uplink state.
type WAN struct {
	Connected *bool    `json:"connected"`
	IP        string   `json:"ip,omitempty"`
	IPv6      string   `json:"ipv6,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Protocol  string   `json:"protocol,omitempty"` // dhcp, pppoe, static, ...
	// PublicIP is the public address, when the driver knows it (it
	// differs from IP on a router behind another NAT)
	PublicIP string `json:"public_ip,omitempty"`
	// Uptime of the WAN link in seconds
	Uptime *int64         `json:"uptime,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// System is the device health.
type System struct {
	CPUUsage *float64 `json:"cpu_usage,omitempty"` // percent
	RAMUsage *float64 `json:"ram_usage,omitempty"` // percent
	// Temperatures in °C by sensor name ("cpu", "2.4ghz", ...)
	Temperatures map[string]float64 `json:"temperatures,omitempty"`
	// Uptime of the device in seconds
	Uptime *int64         `json:"uptime,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// Interface is the traffic of one interface (counters in bytes, rates
// in bytes per second; nil when unknown).
type Interface struct {
	Name    string   `json:"name"`
	RxBytes *uint64  `json:"rx_bytes,omitempty"`
	TxBytes *uint64  `json:"tx_bytes,omitempty"`
	RxRate  *float64 `json:"rx_rate,omitempty"`
	TxRate  *float64 `json:"tx_rate,omitempty"`
}

// Node is a mesh node / access point.
type Node struct {
	MAC    string `json:"mac"`
	Name   string `json:"name,omitempty"`
	Model  string `json:"model,omitempty"`
	IP     string `json:"ip,omitempty"`
	Online bool   `json:"online"`
	// Clients attached to this node
	Clients *int `json:"clients,omitempty"`
}

// Firmware is the firmware update state.
type Firmware struct {
	Current         string `json:"current"`
	Available       string `json:"available,omitempty"`
	UpdateAvailable *bool  `json:"update_available,omitempty"`
}

// Part selects what a Poll should read.
type Part string

// Parts of the normalized model.
const (
	PartClients  Part = "clients"
	PartWAN      Part = "wan"
	PartSystem   Part = "system"
	PartTraffic  Part = "traffic"
	PartNodes    Part = "nodes"
	PartFirmware Part = "firmware"
)

// AllParts lists every part.
var AllParts = []Part{PartClients, PartWAN, PartSystem, PartTraffic, PartNodes, PartFirmware}

// Snapshot is one poll result. A part is nil when it was not requested
// or is unsupported; Errors holds per-part failures (the poll itself only
// fails when the device cannot be reached at all, or when every requested
// part failed).
type Snapshot struct {
	Time     time.Time          `json:"time"`
	Clients  []Client           `json:"clients,omitempty"`
	WAN      *WAN               `json:"wan,omitempty"`
	System   *System            `json:"system,omitempty"`
	Traffic  []Interface        `json:"traffic,omitempty"`
	Nodes    []Node             `json:"nodes,omitempty"`
	Firmware *Firmware          `json:"firmware,omitempty"`
	Errors   map[Part]PartError `json:"errors,omitempty"`
}

// SetError records the failure of one part (see NewPartError).
func (s *Snapshot) SetError(p Part, err error) {
	if s.Errors == nil {
		s.Errors = map[Part]PartError{}
	}
	s.Errors[p] = NewPartError(err)
}

// PollRequest selects what a Poll reads.
type PollRequest struct {
	// Parts to read; parts the device does not support are left nil
	Parts []Part
	// MaxAge bounds how old cached data may be: 0 is the driver default
	// (e.g. the ASUS cache threshold), < 0 forces fresh reads
	MaxAge time.Duration
}

// Poll returns a PollRequest for parts (driver default freshness).
func Poll(parts ...Part) PollRequest { return PollRequest{Parts: parts} }

// SourceSpec describes one native (brand-specific) data source.
type SourceSpec struct {
	Name string `json:"name"` // "ARClientsSource", "wan_status", ...
	Help string `json:"help,omitempty"`
}

// ContractVersion is bumped on any change to the Go API or the JSON shape
// of this package (semver; additive changes are minor). Drivers report
// the version they were built against in DriverInfo.Contract.
const ContractVersion = "0.2.0"

// Driver is one device of some brand. Implementations embed Base (which
// supplies defaults for methods added later) and must be safe for
// concurrent use.
//
// Lifecycle: Connect is idempotent and logs in again after Close; Close
// is idempotent and frees single-session devices. After Close, every
// method but Info, Connect and Close returns an error (ErrClosed) until
// the next Connect.
type Driver interface {
	// Info is static metadata (the registry name, the display vendor)
	Info() DriverInfo
	Connect(ctx context.Context) error
	Close(ctx context.Context) error
	Identity(ctx context.Context) (Identity, error)
	// Capabilities is static (a superset) before Connect and exact after:
	// the parts, actions and native sources this device supports
	Capabilities(ctx context.Context) (Capabilities, error)
	// Poll reads the requested parts of the normalized model
	Poll(ctx context.Context, req PollRequest) (Snapshot, error)
	// Get reads a native source listed in Capabilities.Sources (JSON-able)
	Get(ctx context.Context, source string) (any, error)
	// Do runs an action listed in Capabilities.Actions. Callers bind the
	// arguments (Capabilities.Invoke, ActionSpec.Bind); drivers bind them
	// again (BindInvocation) before any request, so bad arguments are
	// ErrInvalidArgs and an unknown action ErrUnsupported
	Do(ctx context.Context, inv Invocation) (Result, error)

	base() // unexported: implementations embed Base
}

// Base supplies forward-compatible defaults: empty capabilities, and
// ErrUnsupported from Get and Do. A new contract method ships together
// with a default here, so existing drivers keep compiling.
type Base struct{}

// Capabilities implements Driver: nothing declared.
func (Base) Capabilities(context.Context) (Capabilities, error) { return Capabilities{}, nil }

// Get implements Driver: no native sources.
func (Base) Get(_ context.Context, source string) (any, error) {
	return nil, Classify(fmt.Errorf("unknown source %q", source), ErrNotFound)
}

// Do implements Driver: no actions.
func (Base) Do(_ context.Context, inv Invocation) (Result, error) {
	return Result{}, fmt.Errorf("unknown action %q: %w", inv.Name, ErrUnsupported)
}

func (Base) base() {}

// Stability says how far a driver can be trusted.
type Stability string

// Stability levels (shown by `routerctl version` and `detect`).
const (
	Experimental Stability = "experimental"
	Beta         Stability = "beta"
	Stable       Stability = "stable"
)

// DriverInfo is static driver metadata.
type DriverInfo struct {
	// Name is the registry key: lower case, [a-z0-9-]+ ("asus", "zte")
	Name string `json:"name"`
	// Vendor is the display name ("ASUS", "ZTE"), also Identity.Vendor
	Vendor string `json:"vendor"`
	Help   string `json:"help"`
	// Variants lists the sub-drivers or generations ("stock", "merlin")
	Variants  []string  `json:"variants,omitempty"`
	Stability Stability `json:"stability"`
	// Contract is the ContractVersion the driver was built against
	Contract string `json:"contract"`
}

// Wrapper is implemented by drivers that decorate another driver (the
// "auto" driver, later caching or metrics decorators).
type Wrapper interface {
	// Unwrap returns the wrapped driver, or nil when there is none yet
	Unwrap() Driver
}

// As finds the first driver of type T in the Unwrap chain of d: the
// typed escape hatch to a brand API through wrappers, e.g.
// As[*ztedriver.Driver](d).
func As[T any](d Driver) (T, bool) {
	for d != nil {
		if t, ok := d.(T); ok {
			return t, true
		}
		w, ok := d.(Wrapper)
		if !ok {
			break
		}
		d = w.Unwrap()
	}
	var zero T
	return zero, false
}

// Config is what a driver factory receives.
type Config struct {
	// Name labels the device in error messages (optional)
	Name     string
	Host     string
	Username string
	Password string
	// UseSSL / Port: 0 port means the brand default
	UseSSL bool
	Port   int
	// TLS: verify against a SHA-256 fingerprint pin when set
	PinSHA256 string
	Timeout   time.Duration
	// Options: brand-specific settings from the config file
	Options map[string]string
}

// Ptr is a small helper for optional fields.
func Ptr[T any](v T) *T { return &v }
