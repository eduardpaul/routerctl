// Package ztedriver adapts the ZTE F86xx library (package zte) to the
// routerctl driver contract. Factory returns the "zte" driver factory.
//
// Options (config file; see OptionSpecs):
//
//	insecure      true accepts any https certificate (when no pin is set)
//	min_interval  minimum spacing between requests ("500ms"; default 0)
//	pon           false skips the PON optical read in System
package ztedriver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/brands/zte"
)

// Name is the registered driver name.
const Name = "zte"

// Info is the static driver metadata.
var Info = routerkit.DriverInfo{
	Name:      Name,
	Vendor:    "ZTE",
	Help:      "ZTE ZXHN F86xx fiber gateways (F8648P web UI), read-only",
	Variants:  []string{"lua"},
	Stability: routerkit.Beta,
	Contract:  routerkit.ContractVersion,
}

// OptionSpecs are the driver options.
var OptionSpecs = []routerkit.OptionSpec{
	{Name: "insecure", Help: "accept any https certificate (when no pin is set)", Type: routerkit.ArgBool, Default: "false"},
	{Name: "min_interval", Help: "minimum spacing between requests (e.g. 500ms)", Type: routerkit.ArgDuration},
	{Name: "pon", Help: "read the PON optics in System (false skips it)", Type: routerkit.ArgBool, Default: "true"},
}

// Factory returns the "zte" driver factory.
func Factory() routerkit.Factory {
	return routerkit.Factory{
		Info:         Info,
		Options:      OptionSpecs,
		New:          func(cfg routerkit.Config) (routerkit.Driver, error) { return New(cfg) },
		Fingerprints: []routerkit.Fingerprint{{Match: Fingerprint}},
	}
}

// ZTE home-gateway model names: F8648P, F6640, ZXHN H298A, ...
var modelRE = regexp.MustCompile(`^(?i)(ZXHN\s*)?[FH]\d{3,4}[A-Z]{0,3}$`)

// Fingerprint recognizes the ZTE F86xx login page: the entity-encoded
// model name in <title> together with the ZTE login form or markers
// (score 90, with the model), or the login form together with the ZTE
// login endpoints or logo (70). The loginData/menuView endpoints are
// the "lua" web-UI generation.
func Fingerprint(ev *routerkit.Evidence) (routerkit.Match, bool) {
	home := ev.Home()
	text := home.Text()
	has := func(s string) bool { return strings.Contains(text, s) }
	loginForm := has("Frm_Username") || has("_tag=login_entry") || has("_tag=login_token")
	zteMarker := has("Logo_ZTE") || has("_type=loginData") || has("_type=menuView")
	m := routerkit.Match{}
	if has("_type=loginData") || has("_type=menuView") || has("_tag=login_entry") {
		m.Variant = "lua"
	}
	title := home.Title
	if title == "" {
		title = routerkit.PageTitle(text)
	}
	switch {
	case modelRE.MatchString(title) && (loginForm || zteMarker):
		m.Score, m.Model, m.Reason = 90, title, "model "+title+" in the title with the ZTE login page"
	case loginForm && zteMarker:
		m.Score, m.Reason = 70, "ZTE login form and endpoints"
	default:
		return routerkit.Match{}, false
	}
	return m, true
}

// Driver is a ZTE device.
type Driver struct {
	routerkit.Base

	c           *zte.Client
	usePON      bool
	minInterval time.Duration

	mu        sync.Mutex
	identity  *routerkit.Identity
	closed    bool
	connected bool
	// ponKnown / noPON: whether the PON page is available (account,
	// model), learnt once after Connect
	ponKnown, noPON bool
}

var _ routerkit.Driver = (*Driver)(nil)

// Options for NewWithOptions beyond routerkit.Config.
type Options struct {
	// Transport replaces the HTTP transport (a netkit/httprec replay or
	// recorder)
	Transport http.RoundTripper
}

// New builds the driver (no request is sent).
func New(cfg routerkit.Config) (*Driver, error) { return NewWithOptions(cfg, Options{}) }

// NewWithOptions builds the driver, optionally over a custom transport.
func NewWithOptions(cfg routerkit.Config, x Options) (*Driver, error) {
	o, err := routerkit.BindOptions(OptionSpecs, cfg.Options)
	if err != nil {
		return nil, fmt.Errorf("zte: %w", err)
	}
	opts := &zte.Options{
		UseSSL:             cfg.UseSSL,
		Port:               cfg.Port,
		Timeout:            cfg.Timeout,
		PinSHA256:          cfg.PinSHA256,
		InsecureSkipVerify: o.Bool("insecure"),
		MinInterval:        o.Duration("min_interval"),
		Transport:          x.Transport,
	}
	c, err := zte.New(cfg.Host, cfg.Username, cfg.Password, opts)
	if err != nil {
		return nil, err
	}
	return &Driver{c: c, usePON: o.Bool("pon"), minInterval: opts.MinInterval}, nil
}

// Client exposes the underlying library client.
func (d *Driver) Client() *zte.Client { return d.c }

// Info implements routerkit.Driver.
func (d *Driver) Info() routerkit.DriverInfo { return Info }

// mapErr adds the contract error kind to a library error.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, zte.ErrLocked):
		return routerkit.Classify(err, routerkit.ErrLockedOut)
	case errors.Is(err, zte.ErrAuth):
		return routerkit.Classify(err, routerkit.ErrAuth)
	case errors.Is(err, zte.ErrSession):
		// still on the login page after a login: another session holds
		// the device's single web session
		return routerkit.Classify(err, routerkit.ErrBusy)
	case errors.Is(err, zte.ErrNotAvailable):
		return routerkit.Classify(err, routerkit.ErrUnsupported)
	case errors.Is(err, zte.ErrUnknownSource):
		return routerkit.Classify(err, routerkit.ErrNotFound)
	case routerkit.IsNetwork(err):
		return routerkit.Classify(err, routerkit.ErrUnreachable)
	}
	return err
}

// Connect logs in (again after Close).
func (d *Driver) Connect(ctx context.Context) error {
	if err := d.c.Login(ctx); err != nil {
		return mapErr(err)
	}
	d.mu.Lock()
	d.closed, d.connected = false, true
	d.mu.Unlock()
	return nil
}

// Close logs out (frees the device's single web session).
func (d *Driver) Close(ctx context.Context) error {
	d.mu.Lock()
	d.closed, d.connected = true, false
	d.mu.Unlock()
	return mapErr(d.c.Logout(ctx))
}

func (d *Driver) checkOpen() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return routerkit.ErrClosed
	}
	return nil
}

// Capabilities implements routerkit.Driver. Nodes are unsupported, Firmware
// only reports the current version, and the PON temperature needs the
// PON page (admin accounts): after Connect the first call reads it once to
// tell.
func (d *Driver) Capabilities(ctx context.Context) (routerkit.Capabilities, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Capabilities{}, err
	}
	d.mu.Lock()
	probe := d.usePON && d.connected && !d.ponKnown
	d.mu.Unlock()
	if probe {
		d.learnPON(d.c.PON(ctx))
	}
	d.mu.Lock()
	pon := d.usePON && !d.noPON
	d.mu.Unlock()
	parts := routerkit.AllSupported()
	parts[routerkit.PartClients] = routerkit.PartialSupport("vendor", "node")
	parts[routerkit.PartWAN] = routerkit.PartialSupport("public_ip")
	parts[routerkit.PartTraffic] = routerkit.PartialSupport("rx_rate", "tx_rate")
	parts[routerkit.PartNodes] = routerkit.PartSupport{Level: routerkit.Unsupported, Note: "no mesh node list"}
	parts[routerkit.PartFirmware] = routerkit.PartialSupport("available", "update_available")
	var features []routerkit.Feature
	if pon {
		features = append(features, routerkit.FeaturePON)
	} else {
		parts[routerkit.PartSystem] = routerkit.PartialSupport("temperatures")
	}
	return routerkit.Capabilities{
		Parts:         parts,
		Features:      features,
		Sources:       d.sources(),
		MinInterval:   d.minInterval,
		SingleSession: true,
	}, nil
}

// learnPON records whether the PON page answered.
func (d *Driver) learnPON(_ zte.PON, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case err == nil:
		d.ponKnown, d.noPON = true, false
	case errors.Is(err, zte.ErrNotAvailable):
		d.ponKnown, d.noPON = true, true
	}
}

// Identity implements routerkit.Driver (cached after the first success).
func (d *Driver) Identity(ctx context.Context) (routerkit.Identity, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Identity{}, err
	}
	d.mu.Lock()
	if d.identity != nil {
		id := *d.identity
		d.mu.Unlock()
		return id, nil
	}
	d.mu.Unlock()
	info, err := d.c.DeviceInfo(ctx)
	if err != nil {
		return routerkit.Identity{}, mapErr(err)
	}
	id := MapIdentity(info, nil)
	if tp, err := d.c.Topology(ctx); err == nil {
		id = MapIdentity(info, &tp)
	} else if ctx.Err() != nil {
		return routerkit.Identity{}, ctx.Err()
	}
	d.mu.Lock()
	d.identity = &id
	d.mu.Unlock()
	return id, nil
}

// MapIdentity maps the library device info (and topology, for the MAC).
func MapIdentity(info zte.DeviceInfo, tp *zte.Topology) routerkit.Identity {
	id := routerkit.Identity{
		Vendor:   Info.Vendor,
		Model:    info.Model,
		Firmware: info.SoftwareVersion,
		Serial:   info.SerialNumber,
		Variant:  "lua", // the only web-UI generation implemented
	}
	if tp != nil {
		id.MAC = strings.ToLower(tp.MAC)
	}
	return id
}

// Poll implements routerkit.Driver. Nodes are not supported (left nil);
// Firmware only reports the current version.
func (d *Driver) Poll(ctx context.Context, req routerkit.PollRequest) (routerkit.Snapshot, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Snapshot{}, err
	}
	snap := routerkit.Snapshot{Time: time.Now()}
	var firstErr error
	failed, tried := 0, 0
	fail := func(p routerkit.Part, err error) {
		err = mapErr(err)
		snap.SetError(p, err)
		failed++
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, p := range req.Parts {
		if err := ctx.Err(); err != nil {
			return snap, err
		}
		switch p {
		case routerkit.PartClients:
			tried++
			hs, err := d.c.Clients(ctx)
			if err != nil {
				fail(p, err)
				continue
			}
			snap.Clients = MapClients(hs)
		case routerkit.PartWAN:
			tried++
			w, err := d.c.WANStatus(ctx)
			if err != nil {
				fail(p, err)
				continue
			}
			snap.WAN = MapWAN(w)
		case routerkit.PartSystem:
			tried++
			s, err := d.c.System(ctx)
			if err != nil {
				fail(p, err)
				continue
			}
			var pon *zte.PON
			var ponErr error
			if d.usePON {
				pv, err := d.c.PON(ctx)
				d.learnPON(pv, err)
				if err == nil {
					pon = &pv
				} else if !errors.Is(err, zte.ErrNotAvailable) {
					ponErr = err
				}
			}
			snap.System = MapSystem(s, pon)
			if ponErr != nil {
				if snap.System.Extra == nil {
					snap.System.Extra = map[string]any{}
				}
				snap.System.Extra["pon_error"] = ponErr.Error()
			}
		case routerkit.PartTraffic:
			tried++
			t, err := d.c.Traffic(ctx)
			if err != nil {
				fail(p, err)
				continue
			}
			snap.Traffic = MapTraffic(t)
		case routerkit.PartFirmware:
			tried++
			info, err := d.c.DeviceInfo(ctx)
			if err != nil {
				fail(p, err)
				continue
			}
			snap.Firmware = &routerkit.Firmware{Current: info.SoftwareVersion}
		case routerkit.PartNodes:
			// unsupported: left nil, not an error
		}
	}
	if tried > 0 && failed == tried {
		return snap, firstErr
	}
	return snap, nil
}

// MapClients maps library hosts to normalized clients.
func MapClients(hs []zte.Host) []routerkit.Client {
	out := make([]routerkit.Client, 0, len(hs))
	for _, h := range hs {
		c := routerkit.Client{
			MAC:    strings.ToLower(h.MAC),
			IP:     h.IP,
			Name:   h.HostName,
			Online: true, // the device only lists connected clients
			RSSI:   h.RSSI,
			RxRate: h.RxRate,
			TxRate: h.TxRate,
			Extra:  map[string]any{},
		}
		switch h.Interface {
		case "lan":
			c.Connection = "wired"
		case "wlan":
			switch h.Band {
			case "2.4GHz":
				c.Connection = "2.4ghz"
			case "5GHz":
				c.Connection = "5ghz"
			case "6GHz":
				c.Connection = "6ghz"
			default:
				c.Connection = "wifi"
			}
		}
		if h.LinkTime != nil {
			since := time.Now().Add(-*h.LinkTime).Truncate(time.Second)
			c.Since = &since
		}
		put := func(k string, v any, ok bool) {
			if ok {
				c.Extra[k] = v
			}
		}
		put("port", h.Port, h.Port != "")
		put("ipv6", h.IPv6, h.IPv6 != "")
		put("ssid", h.SSID, h.SSID != "")
		put("snr", derefI(h.SNR), h.SNR != nil)
		put("noise", derefI(h.Noise), h.Noise != nil)
		put("mode", h.Mode, h.Mode != "")
		put("bandwidth", h.Bandwidth, h.Bandwidth != "")
		put("rx_bytes", derefU(h.RxBytes), h.RxBytes != nil)
		put("tx_bytes", derefU(h.TxBytes), h.TxBytes != nil)
		if len(c.Extra) == 0 {
			c.Extra = nil
		}
		out = append(out, c)
	}
	return out
}

func derefI(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefU(p *uint64) uint64 {
	if p == nil {
		return 0
	}
	return *p
}

// MapWAN maps the internet connection (the default-route one).
func MapWAN(w zte.WANStatus) *routerkit.WAN {
	c, ok := w.Internet()
	if !ok {
		return &routerkit.WAN{}
	}
	out := &routerkit.WAN{
		Connected: routerkit.Ptr(c.Connected),
		IP:        c.IP,
		IPv6:      c.IPv6,
		Gateway:   c.Gateway,
		DNS:       c.DNS,
		Protocol:  c.Protocol,
		Extra: map[string]any{
			"name":   c.Name,
			"status": c.Status,
		},
	}
	if c.Connected || c.Uptime > 0 {
		out.Uptime = routerkit.Ptr(int64(c.Uptime / time.Second))
	}
	put := func(k string, v any, ok bool) {
		if ok {
			out.Extra[k] = v
		}
	}
	put("ipv6_prefix", c.IPv6Prefix, c.IPv6Prefix != "")
	put("gateway6", c.Gateway6, c.Gateway6 != "")
	put("dns6", c.DNS6, len(c.DNS6) > 0)
	put("vlan", c.VLAN, c.VLAN != 0)
	put("mtu", c.MTU, c.MTU != 0)
	put("mac", c.MAC, c.MAC != "")
	put("status6", c.Status6, c.Status6 != "")
	put("error", c.Error, c.Error != "")
	conns := make([]map[string]any, 0, len(w.Connections))
	for _, x := range w.Connections {
		conns = append(conns, map[string]any{
			"name": x.Name, "protocol": x.Protocol, "status": x.Status, "ip": x.IP,
			"services": x.Services, "vlan": x.VLAN, "default_gw": x.DefaultGW,
		})
	}
	out.Extra["connections"] = conns
	return out
}

// MapSystem maps CPU/RAM/uptime and, when available, the PON optics:
// Temperatures["pon"], Extra["pon_rx_power_dbm"], ["pon_tx_power_dbm"],
// ["pon_voltage_v"], ["pon_bias_current_ma"].
func MapSystem(s zte.System, pon *zte.PON) *routerkit.System {
	out := &routerkit.System{
		Uptime: routerkit.Ptr(int64(s.Uptime / time.Second)),
		Extra:  map[string]any{},
	}
	if len(s.CPUUsage) > 0 {
		out.CPUUsage = routerkit.Ptr(s.CPUAverage)
		out.Extra["cpu_cores"] = s.CPUUsage
	}
	if s.MemUsage != nil {
		out.RAMUsage = routerkit.Ptr(*s.MemUsage)
	}
	if pon != nil {
		if pon.Temperature != nil {
			out.Temperatures = map[string]float64{"pon": *pon.Temperature}
		}
		for k, v := range map[string]*float64{
			"pon_rx_power_dbm":    pon.RxPower,
			"pon_tx_power_dbm":    pon.TxPower,
			"pon_voltage_v":       pon.Voltage,
			"pon_bias_current_ma": pon.BiasCurrent,
		} {
			if v != nil {
				out.Extra[k] = *v
			}
		}
	}
	if len(out.Extra) == 0 {
		out.Extra = nil
	}
	return out
}

// MapTraffic maps counters: "wan" (the internet connection, which the
// library lists first), "wan:<name>" (other WAN connections with
// counters), "lan1".."lan5", "ssid1".. (enabled SSIDs).
func MapTraffic(t zte.Traffic) []routerkit.Interface {
	out := []routerkit.Interface{}
	for i, w := range t.WAN {
		name := "wan"
		if i > 0 {
			name = "wan:" + w.Name
		}
		out = append(out, routerkit.Interface{Name: name, RxBytes: w.RxBytes, TxBytes: w.TxBytes})
	}
	for _, p := range t.Ports {
		out = append(out, routerkit.Interface{Name: strings.ToLower(p.Name), RxBytes: routerkit.Ptr(p.RxBytes), TxBytes: routerkit.Ptr(p.TxBytes)})
	}
	for _, s := range t.SSIDs {
		out = append(out, routerkit.Interface{Name: strings.ToLower(s.Name), RxBytes: s.RxBytes, TxBytes: s.TxBytes})
	}
	return out
}

// sources lists the native data tags.
func (d *Driver) sources() []routerkit.SourceSpec {
	out := make([]routerkit.SourceSpec, 0, len(zte.Sources))
	for _, s := range zte.Sources {
		help := s.Help
		if !s.Verified {
			help += " [unverified]"
		}
		out = append(out, routerkit.SourceSpec{Name: s.Name, Help: help})
	}
	return out
}

// Get implements routerkit.Driver: a native tag as parsed instances. Names
// from Capabilities.Sources, raw tags and "view/tag" are accepted. No
// write action is implemented (Base.Do): the web UI signs every
// configuration POST and the forms could not be verified without
// changing the (ISP) gateway. See brands/zte/PROTOCOL.md.
func (d *Driver) Get(ctx context.Context, name string) (any, error) {
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	r, err := d.c.Get(ctx, name)
	if err != nil {
		return nil, mapErr(err)
	}
	return r, nil
}
