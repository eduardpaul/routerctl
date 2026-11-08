// Package detect identifies the brand of a device without logging in:
// it fetches the probe paths of every registered fingerprint once
// (unauthenticated GETs, bodies capped, same-host redirects only), scores
// every fingerprint against that evidence and picks the best match. A
// tie at the best score is an error (ErrAmbiguous), not a coin flip.
package detect

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerkit"
)

// MaxBody is how much of each page is read.
const MaxBody = 64 << 10

// MaxProbes caps the number of paths fetched (the home page included).
const MaxProbes = 8

// Errors of Pick and Detect.
var (
	ErrUnrecognized = errors.New("unrecognized device")
	ErrAmbiguous    = errors.New("ambiguous device")
)

// Target is the device to probe.
type Target struct {
	Host    string
	UseSSL  bool
	Port    int
	Timeout time.Duration
}

// TargetOf returns the target of a driver config.
func TargetOf(cfg routerkit.Config) Target {
	return Target{Host: cfg.Host, UseSSL: cfg.UseSSL, Port: cfg.Port, Timeout: cfg.Timeout}
}

// BaseURL builds the device URL.
func BaseURL(host string, ssl bool, port int) string {
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	h := host
	if port > 0 {
		h = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
	}
	return scheme + "://" + h + "/"
}

// Probes returns the paths to fetch for r: "/" and the union of the
// fingerprints' probes, in a stable order, at most MaxProbes.
func Probes(r *routerkit.Registry) []string {
	seen := map[string]bool{"/": true}
	out := []string{"/"}
	for _, f := range r.Factories() {
		for _, fp := range f.Fingerprints {
			for _, p := range fp.Probes {
				if !seen[p] && len(out) < MaxProbes {
					seen[p] = true
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func client(t Target, host string, trail *[]string) *http.Client {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // detection only, no credentials
			Proxy:               nil,
			TLSHandshakeTimeout: timeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 3 {
				return errors.New("too many redirects")
			}
			if req.URL.Hostname() != host {
				return http.ErrUseLastResponse
			}
			*trail = append(*trail, req.URL.String())
			return nil
		},
	}
}

// fetch GETs one path. Redirects are followed on the same host only (up
// to 3); their targets are kept in Page.Redirects.
func fetch(ctx context.Context, t Target, path string) (routerkit.Page, *tls.ConnectionState, error) {
	base := BaseURL(t.Host, t.UseSSL, t.Port)
	page := routerkit.Page{URL: base}
	u0, err := url.Parse(base)
	if err != nil || u0.Host == "" {
		return page, nil, fmt.Errorf("invalid host %q", t.Host)
	}
	ref, err := url.Parse(path)
	if err != nil {
		return page, nil, err
	}
	target := u0.ResolveReference(ref).String()
	page.URL = target
	var trail []string
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return page, nil, err
	}
	req.Header.Set("User-Agent", "routerctl-detect")
	resp, err := client(t, u0.Hostname(), &trail).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return page, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxBody))
	page.Status = resp.StatusCode
	page.URL = resp.Request.URL.String()
	page.Body = string(body)
	page.Header = map[string]string{}
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			page.Header[strings.ToLower(k)] = vs[0]
		}
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		trail = append(trail, loc)
	}
	page.Redirects = trail
	page.Title = routerkit.PageTitle(page.Body)
	return page, resp.TLS, nil
}

// Gather fetches the given paths once each. It fails only when the home
// page ("/", fetched first) cannot be read; other probe failures are
// recorded in Page.Err.
func Gather(ctx context.Context, t Target, paths []string) (*routerkit.Evidence, error) {
	ev := &routerkit.Evidence{Pages: map[string]routerkit.Page{}}
	if len(paths) == 0 || paths[0] != "/" {
		paths = append([]string{"/"}, paths...)
	}
	for i, p := range paths {
		if _, done := ev.Pages[p]; done {
			continue
		}
		page, state, err := fetch(ctx, t, p)
		if err != nil {
			if i == 0 {
				return ev, err
			}
			page.Err = err.Error()
		}
		if state != nil && len(state.PeerCertificates) > 0 && ev.TLSCert == nil {
			ev.TLSCert = state.PeerCertificates[0]
		}
		ev.Pages[p] = page
	}
	return ev, nil
}

// Score runs every fingerprint of r on ev: all candidates, best first
// (ties by driver name).
func Score(r *routerkit.Registry, ev *routerkit.Evidence) []routerkit.Match {
	var out []routerkit.Match
	for _, f := range r.Factories() {
		best, found := routerkit.Match{}, false
		for _, m := range f.Matches(ev) {
			if !found || m.Score > best.Score {
				best, found = m, true
			}
		}
		if found {
			out = append(out, best)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Driver < out[j].Driver
	})
	return out
}

// Pick returns the best candidate: ErrUnrecognized when there is none,
// ErrAmbiguous when two drivers share the best score.
func Pick(r *routerkit.Registry, cands []routerkit.Match) (routerkit.Match, error) {
	if len(cands) == 0 {
		return routerkit.Match{}, fmt.Errorf("%w (known drivers: %v)", ErrUnrecognized, r.Names())
	}
	if len(cands) > 1 && cands[1].Score == cands[0].Score {
		var tied []string
		for _, c := range cands {
			if c.Score == cands[0].Score {
				tied = append(tied, c.Driver)
			}
		}
		return routerkit.Match{}, fmt.Errorf("%w: %s match equally (score %d); set the driver explicitly", ErrAmbiguous, strings.Join(tied, ", "), cands[0].Score)
	}
	return cands[0], nil
}

// Detect gathers the evidence of t and picks its driver: the best match,
// every candidate (best first) and the error of Pick or Gather.
func Detect(ctx context.Context, r *routerkit.Registry, t Target) (routerkit.Match, []routerkit.Match, error) {
	ev, err := Gather(ctx, t, Probes(r))
	if err != nil {
		return routerkit.Match{}, nil, err
	}
	cands := Score(r, ev)
	best, err := Pick(r, cands)
	return best, cands, err
}

// Prober is the standard routerkit.Prober (Registry.WithProber).
func Prober(ctx context.Context, r *routerkit.Registry, cfg routerkit.Config) (routerkit.Match, error) {
	best, _, err := Detect(ctx, r, TargetOf(cfg))
	return best, err
}

var _ routerkit.Prober = Prober

// Result is what `routerctl detect` prints.
type Result struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Title  string `json:"title,omitempty"`
	Server string `json:"server,omitempty"`
	// Driver is the name of the detected driver
	Driver string `json:"driver,omitempty"`
	Error  string `json:"error,omitempty"`
	// Candidates are every matching driver, best first
	Candidates []routerkit.Match `json:"candidates,omitempty"`
}

// Probe detects the driver of a host for the CLI: the home page facts,
// the driver and every candidate.
func Probe(ctx context.Context, reg *routerkit.Registry, host string, ssl bool, port int, timeout time.Duration) (Result, error) {
	t := Target{Host: host, UseSSL: ssl, Port: port, Timeout: timeout}
	res := Result{URL: BaseURL(host, ssl, port)}
	ev, err := Gather(ctx, t, Probes(reg))
	home := ev.Home()
	if home.URL != "" {
		res.URL = home.URL
	}
	res.Status, res.Title, res.Server = home.Status, home.Title, home.Header["server"]
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	res.Candidates = Score(reg, ev)
	best, err := Pick(reg, res.Candidates)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	res.Driver = best.Driver
	return res, nil
}
