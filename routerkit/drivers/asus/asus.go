// Package asus implements the routerctl driver contract over the ASUS Go
// library (package asusrouter, the port of the Python library).
//
// Native data: every registered library source, by its Python class name
// (ARClientsSource, ARWanSource, ...), returned in canonical form
// (parity/canonical) so it is JSON-able. The normalized model is built
// from those canonical trees (package normalize). Factory returns the
// "asus" driver factory.
package asus

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	asusrouter "github.com/eduardpaul/routerctl/asusrouter"
	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
	_ "github.com/eduardpaul/routerctl/asusrouter/register" // registers every module
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/routerkit"
	"github.com/eduardpaul/routerctl/routerkit/drivers/asus/normalize"
)

// Source names used by the normalized model.
const (
	srcDevice      = "ARDeviceSource"
	srcClients     = "ARClientsSource"
	srcClientCount = "ARClientsCountSource"
	srcWAN         = "ARWanSource"
	srcSystem      = "ARSystemStatusSource"
	srcTemperature = "ARTemperatureSource"
	srcClock       = "ARClockSource"
	srcTraffic     = "ARTrafficSource"
	srcAiMesh      = "ARAiMeshSource"
	srcFirmware    = "ARFirmwareSource"
)

var partSources = map[routerkit.Part][]string{
	routerkit.PartClients:  {srcClients},
	routerkit.PartWAN:      {srcWAN},
	routerkit.PartSystem:   {srcSystem, srcTemperature, srcClock},
	routerkit.PartTraffic:  {srcTraffic},
	routerkit.PartNodes:    {srcAiMesh, srcClientCount},
	routerkit.PartFirmware: {srcFirmware},
}

// Factory returns the "asus" driver factory. Record and Replay work at
// the library's Transport seam, in the session format shared with the
// Python library (parity/py_harness.py).
func Factory() routerkit.Factory {
	return routerkit.Factory{
		Info:         Info,
		Options:      OptionSpecs,
		New:          func(cfg routerkit.Config) (routerkit.Driver, error) { return New(cfg) },
		Fingerprints: []routerkit.Fingerprint{{Match: Fingerprint}},
		Replay: func(path string, cfg routerkit.Config) (routerkit.Driver, error) {
			rt, err := asusrouter.NewReplayTransport(path)
			if err != nil {
				return nil, err
			}
			return NewWithOptions(cfg, Options{Transport: rt})
		},
		Record: func(cfg routerkit.Config) (routerkit.Driver, routerkit.Recorder, error) {
			d, err := NewWithOptions(cfg, Options{Record: true})
			if err != nil {
				return nil, nil, err
			}
			return d, recorder{d.rec}, nil
		},
	}
}

// Info is the static driver metadata.
var Info = routerkit.DriverInfo{
	Name:      "asus",
	Vendor:    "ASUS",
	Help:      "ASUSWRT routers (stock and Merlin), with AiMesh",
	Variants:  []string{"stock", "merlin"},
	Stability: routerkit.Stable,
	Contract:  routerkit.ContractVersion,
}

// OptionSpecs are the driver options.
var OptionSpecs = []routerkit.OptionSpec{
	{Name: "cache", Help: "how long the library reuses data (cache threshold)", Type: routerkit.ArgDuration, Default: "5s"},
}

// recorder adapts the library's recording transport to routerkit.Recorder.
type recorder struct{ *asusrouter.RecordingTransport }

func (r recorder) Len() int { return len(r.Records()) }

// modelRE finds an ASUS model name in a page title.
var modelRE = regexp.MustCompile(`\b((?:RT|GT|TUF|XT|ZenWiFi|BRT|DSL|RP|4G)-[A-Za-z0-9_]+(?: Pro)?)\b`)

// asusMarkers are the ASUSWRT web-UI markers and their scores.
var asusMarkers = []struct {
	marker string
	score  int
}{
	{"asuswrt", 70},
	{"asus_token", 70},
	{"asus wireless router", 70},
	{"/images/new_index/asus", 70},
	{"main_login.asp", 40},             // the ASUSWRT login page (also a redirect target)
	{"login.cgi", routerkit.ScoreWeak}, // a common login endpoint name
}

// Fingerprint recognizes the ASUSWRT web UI (the login page or its
// redirect). An "ASUS" title with the login page scores 90 and names
// the model when the title does.
func Fingerprint(ev *routerkit.Evidence) (routerkit.Match, bool) {
	home := ev.Home()
	h := strings.ToLower(home.Text())
	m := routerkit.Match{}
	for _, mk := range asusMarkers {
		if strings.Contains(h, mk.marker) && mk.score > m.Score {
			m.Score, m.Reason = mk.score, "ASUSWRT marker "+mk.marker
		}
	}
	if m.Score == 0 {
		return routerkit.Match{}, false
	}
	title := home.Title
	if title == "" {
		title = routerkit.PageTitle(home.Text())
	}
	if strings.Contains(strings.ToUpper(title), "ASUS") && (strings.Contains(h, "login.cgi") || strings.Contains(h, "main_login.asp")) {
		m.Score, m.Reason = 90, "ASUS title with the ASUSWRT login page"
	}
	if mm := modelRE.FindStringSubmatch(title); mm != nil {
		m.Model = mm[1]
	}
	return m, true
}

// Driver is a connected ASUS router.
type Driver struct {
	routerkit.Base

	cfg   routerkit.Config
	r     *asusrouter.AsusRouter
	rec   *asusrouter.RecordingTransport
	cache time.Duration

	mu      sync.Mutex
	sources map[string]source.DataSource
	closed  bool
}

var _ routerkit.Driver = (*Driver)(nil)

// Options for New beyond routerkit.Config.
type Options struct {
	// Transport replaces the HTTP connection (replay)
	Transport asusrouter.Transport
	// Record wraps the HTTP connection with a recorder (capture)
	Record bool
}

// New builds a driver over HTTP(S).
func New(cfg routerkit.Config) (*Driver, error) { return NewWithOptions(cfg, Options{}) }

// NewWithOptions builds a driver, optionally over a custom transport or
// recording every query.
func NewWithOptions(cfg routerkit.Config, o Options) (*Driver, error) {
	opts, err := routerkit.BindOptions(OptionSpecs, cfg.Options)
	if err != nil {
		return nil, fmt.Errorf("asus: %w", err)
	}
	d := &Driver{cfg: cfg, sources: map[string]source.DataSource{}}
	transport := o.Transport
	if transport == nil {
		copts := asusrouter.ConnectionOptions{Port: cfg.Port, UseSSL: cfg.UseSSL, Timeout: cfg.Timeout}
		if cfg.PinSHA256 != "" {
			pin, err := asusrouter.ParseFingerprint(cfg.PinSHA256)
			if err != nil {
				return nil, fmt.Errorf("asus: pin_sha256: %w", err)
			}
			copts.PinnedCertSHA256 = pin
			copts.UseSSL = true
		}
		if o.Record {
			// Record at the Query seam: login happens beneath it, so the
			// credentials are never recorded
			conn, err := asusrouter.NewHTTPTransport(cfg.Host, cfg.Username, cfg.Password, copts)
			if err != nil {
				return nil, fmt.Errorf("asus: %w", err)
			}
			d.rec = asusrouter.NewRecordingTransport(conn)
			transport = d.rec
		} else {
			r, err := asusrouter.New(cfg.Host, cfg.Username, cfg.Password, asusrouter.Options{Port: cfg.Port, UseSSL: copts.UseSSL, Connection: copts})
			if err != nil {
				return nil, fmt.Errorf("asus: %w", err)
			}
			d.r = r
		}
	}
	if d.r == nil {
		r, err := asusrouter.New(cfg.Host, cfg.Username, "", asusrouter.Options{Transport: transport})
		if err != nil {
			return nil, fmt.Errorf("asus: %w", err)
		}
		d.r = r
	}
	cache := opts.Duration("cache")
	d.r.SetCacheThreshold(cache)
	d.cache = cache
	return d, nil
}

// Router exposes the library instance.
func (d *Driver) Router() *asusrouter.AsusRouter { return d.r }

// Recorder returns the capture recorder (nil unless Options.Record).
func (d *Driver) Recorder() *asusrouter.RecordingTransport { return d.rec }

// Info implements routerkit.Driver.
func (d *Driver) Info() routerkit.DriverInfo { return Info }

// mapErr adds the contract error kind to a library error.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if code, ok := arerror.AccessCode(err); ok {
		switch endpoint.ARAccessError(code) {
		case endpoint.ARAccessErrorCredentials, endpoint.ARAccessErrorAuthorization, endpoint.ARAccessErrorNoToken:
			return routerkit.Classify(err, routerkit.ErrAuth)
		case endpoint.ARAccessErrorTryAgain, endpoint.ARAccessErrorCaptcha:
			return routerkit.Classify(err, routerkit.ErrLockedOut)
		case endpoint.ARAccessErrorAnother:
			return routerkit.Classify(err, routerkit.ErrBusy)
		}
	}
	if errors.Is(err, arerror.ErrConnection) || errors.Is(err, arerror.ErrTimeout) || routerkit.IsNetwork(err) {
		return routerkit.Classify(err, routerkit.ErrUnreachable)
	}
	return err
}

// Connect implements routerkit.Driver.
func (d *Driver) Connect(ctx context.Context) error {
	// An explicit (re)connect: clear a previous credentials refusal (a
	// firmware update or httpd reload can refuse one login transiently)
	ok, err := d.r.Reconnect(ctx)
	if err != nil {
		return mapErr(fmt.Errorf("asus: connect: %w", err))
	}
	if !ok {
		return routerkit.Classify(errors.New("asus: login failed"), routerkit.ErrAuth)
	}
	d.mu.Lock()
	d.closed = false
	d.mu.Unlock()
	return nil
}

// Close implements routerkit.Driver.
func (d *Driver) Close(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	return d.r.Close(ctx)
}

func (d *Driver) checkOpen() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return routerkit.ErrClosed
	}
	return nil
}

// Capabilities implements routerkit.Driver. The WAN part has no IPv6 address
// nor link uptime; the features come from the device's support flags
// (known after Connect).
func (d *Driver) Capabilities(context.Context) (routerkit.Capabilities, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Capabilities{}, err
	}
	parts := routerkit.AllSupported()
	parts[routerkit.PartWAN] = routerkit.PartialSupport("ipv6", "uptime")
	var features []routerkit.Feature
	if id := d.r.Description(); id != nil {
		sup := id.Support()
		if truthy(canonical.Of(sup[support.ARSupportTypeAimesh])) {
			features = append(features, routerkit.FeatureMesh)
		}
		if truthy(canonical.Of(sup[support.ARSupportTypeDsl])) {
			features = append(features, routerkit.FeatureDSL)
		}
	}
	return routerkit.Capabilities{
		Parts:    parts,
		Features: features,
		Actions:  d.actions(),
		Sources:  d.sourceSpecs(),
	}, nil
}

// instance returns the (cached) default instance of a source; sources
// can keep history (clients merge vanished entries), so one instance is
// reused for the driver's lifetime.
func (d *Driver) instance(name string) (source.DataSource, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if s, ok := d.sources[name]; ok {
		return s, nil
	}
	e := registry.SourceByName(name)
	if e == nil || e.Default == nil || (e.Fetch == nil && e.FetchMulti == nil) {
		return nil, fmt.Errorf("source %s: %w", name, routerkit.ErrUnsupported)
	}
	s := e.Default()
	d.sources[name] = s
	return s, nil
}

// fetchMany reads several sources (batched where the library can) and
// returns their canonical content by name; per-source errors are
// returned separately.
func (d *Driver) fetchMany(ctx context.Context, names []string, force bool) (map[string]any, map[string]error) {
	out := map[string]any{}
	errs := map[string]error{}
	var srcs []source.DataSource
	byID := map[source.ID]string{}
	for _, n := range names {
		s, err := d.instance(n)
		if err != nil {
			errs[n] = err
			continue
		}
		srcs = append(srcs, s)
		byID[source.IDOf(s)] = n
	}
	if len(srcs) == 0 {
		return out, errs
	}
	res, err := d.r.FetchDataMany(ctx, srcs, force, nil)
	err = mapErr(err)
	if err == nil {
		for id, v := range res {
			out[byID[id]] = canonical.Of(v)
		}
		return out, errs
	}
	if len(srcs) == 1 {
		errs[byID[source.IDOf(srcs[0])]] = err
		return out, errs
	}
	// Attribute the failure: fetch one by one
	for _, s := range srcs {
		n := byID[source.IDOf(s)]
		v, err := d.r.FetchData(ctx, s, force, nil)
		if err != nil {
			errs[n] = mapErr(err)
			continue
		}
		out[n] = canonical.Of(v)
	}
	return out, errs
}

// Identity implements routerkit.Driver.
func (d *Driver) Identity(ctx context.Context) (routerkit.Identity, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Identity{}, err
	}
	got, errs := d.fetchMany(ctx, []string{srcDevice}, false)
	if err := errs[srcDevice]; err != nil {
		return routerkit.Identity{}, err
	}
	id := normalize.Identity(got[srcDevice])
	id.Variant = d.variant()
	return id, nil
}

// variant is "merlin" for a Merlin-based firmware, else "stock" ("" while
// the firmware is unknown).
func (d *Driver) variant() string {
	desc := d.r.Description()
	if desc == nil {
		return ""
	}
	ft := desc.Firmware().FirmwareType()
	if _, merlin := firmware.ARFwMerlinLike[ft]; merlin {
		return "merlin"
	}
	if ft == "" || ft == firmware.ARFirmwareTypeUnknown {
		return ""
	}
	return "stock"
}

// Poll implements routerkit.Driver. MaxAge < 0, or below the cache
// threshold, reads fresh data.
func (d *Driver) Poll(ctx context.Context, req routerkit.PollRequest) (routerkit.Snapshot, error) {
	if err := d.checkOpen(); err != nil {
		return routerkit.Snapshot{}, err
	}
	parts := req.Parts
	force := req.MaxAge < 0 || (req.MaxAge > 0 && req.MaxAge < d.cache)
	snap := routerkit.Snapshot{Time: time.Now()}
	var names []string
	seen := map[string]bool{}
	for _, p := range parts {
		for _, n := range partSources[p] {
			if !seen[n] {
				seen[n] = true
				names = append(names, n)
			}
		}
	}
	needMAC := seen[srcSystem]
	if needMAC {
		names = append(names, srcDevice)
	}
	got, errs := d.fetchMany(ctx, names, force)

	var firstErr error
	partErr := func(p routerkit.Part, n string) bool {
		if err := errs[n]; err != nil {
			snap.SetError(p, fmt.Errorf("%s: %w", n, err))
			if firstErr == nil {
				firstErr = err
			}
			return true
		}
		return false
	}
	failed := 0
	for _, p := range parts {
		bad := false
		for _, n := range partSources[p] {
			// Optional companions do not fail the part
			if n == srcClientCount || n == srcTemperature || n == srcClock {
				continue
			}
			bad = bad || partErr(p, n)
		}
		if bad {
			failed++
			continue
		}
		switch p {
		case routerkit.PartClients:
			snap.Clients = normalize.Clients(got[srcClients])
		case routerkit.PartWAN:
			snap.WAN = normalize.WAN(got[srcWAN])
		case routerkit.PartSystem:
			mac := normalize.Identity(got[srcDevice]).MAC
			snap.System = normalize.System(got[srcSystem], got[srcTemperature], got[srcClock], mac)
		case routerkit.PartTraffic:
			snap.Traffic = normalize.Traffic(got[srcTraffic])
		case routerkit.PartNodes:
			counts := got[srcClientCount]
			if counts == nil {
				// No count source on this port/firmware: count online
				// clients per node from the client table
				cl, _ := d.fetchMany(ctx, []string{srcClients}, force)
				if c := cl[srcClients]; c != nil {
					counts = normalize.CountByNode(normalize.Clients(c))
				}
			}
			snap.Nodes = normalize.Nodes(got[srcAiMesh], counts)
		case routerkit.PartFirmware:
			snap.Firmware = normalize.Firmware(got[srcFirmware])
		}
	}
	if failed == len(parts) && len(parts) > 0 {
		var msgs []string
		for _, p := range parts {
			msgs = append(msgs, snap.Errors[p].Message)
		}
		return routerkit.Snapshot{}, routerkit.Classify(fmt.Errorf("asus: %s", strings.Join(msgs, "; ")), routerkit.KindOf(firstErr))
	}
	return snap, nil
}

// sourceSpecs lists every fetchable registered source.
func (d *Driver) sourceSpecs() []routerkit.SourceSpec {
	var out []routerkit.SourceSpec
	for _, n := range registry.SourceNames() {
		e := registry.SourceByName(n)
		if e == nil || e.Default == nil || (e.Fetch == nil && e.FetchMulti == nil) {
			continue
		}
		out = append(out, routerkit.SourceSpec{Name: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get implements routerkit.Driver: the canonical content of a source.
// Names are the Python class names; the "AR" prefix and "Source" suffix
// may be omitted, case-insensitively ("clients", "wan", ...).
func (d *Driver) Get(ctx context.Context, name string) (any, error) {
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	n := resolveSource(name)
	if n == "" {
		return nil, routerkit.Classify(fmt.Errorf("unknown source %q", name), routerkit.ErrNotFound)
	}
	got, errs := d.fetchMany(ctx, []string{n}, true)
	if err := errs[n]; err != nil {
		return nil, err
	}
	return got[n], nil
}

func resolveSource(name string) string {
	want := strings.ToLower(name)
	for _, n := range registry.SourceNames() {
		l := strings.ToLower(n)
		short := strings.TrimSuffix(strings.TrimPrefix(l, "ar"), "source")
		if l == want || short == want || short == strings.ReplaceAll(want, "_", "") {
			return n
		}
	}
	return ""
}
