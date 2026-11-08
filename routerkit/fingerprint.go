package routerkit

import (
	"crypto/x509"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Fingerprint scores. A match at or below ScoreWeak is a hint (a generic
// marker other brands share); a family driver scores below a brand
// driver built on it.
const (
	ScoreWeak       = 10
	ScoreStrong     = 50
	ScoreDefinitive = 100
)

// Page is one unauthenticated GET made while detecting a device.
type Page struct {
	Status int    `json:"status"`
	URL    string `json:"url"` // final URL, after same-host redirects
	// Header holds the response headers (lower-case names, first value)
	Header map[string]string `json:"header,omitempty"`
	Title  string            `json:"title,omitempty"`
	// Body is the response body, capped (64 KiB)
	Body string `json:"-"`
	// Redirects lists the redirect targets followed or refused
	Redirects []string `json:"redirects,omitempty"`
	// Err is the fetch error ("" when the page answered)
	Err string `json:"error,omitempty"`
}

// Text is the body followed by the redirect trail: what a fingerprint
// matches markers against (a login URL often only shows in a redirect).
func (p Page) Text() string {
	if len(p.Redirects) == 0 {
		return p.Body
	}
	return p.Body + "\n<!-- redirects: " + strings.Join(p.Redirects, " ") + " -->"
}

// Evidence is everything detection fetched from a device: the union of
// the probe paths of every fingerprint, each fetched once.
type Evidence struct {
	// Pages by probe path; "/" is always there
	Pages map[string]Page
	// TLSCert is the server certificate over https (nil otherwise)
	TLSCert *x509.Certificate
}

// Page returns the page fetched for path (zero when it was not fetched).
func (e *Evidence) Page(path string) Page {
	if e == nil {
		return Page{}
	}
	return e.Pages[path]
}

// Home returns the home page ("/").
func (e *Evidence) Home() Page { return e.Page("/") }

var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// PageTitle returns the text of the first <title> of an HTML page,
// entity-decoded and trimmed.
func PageTitle(body string) string {
	if m := titleRE.FindStringSubmatch(body); m != nil {
		return strings.TrimSpace(html.UnescapeString(m[1]))
	}
	return ""
}

// HomeEvidence builds the Evidence of a device whose home page is body
// (status 200): for fingerprint tests and offline detection.
func HomeEvidence(body string) *Evidence {
	return &Evidence{Pages: map[string]Page{"/": {Status: 200, URL: "/", Title: PageTitle(body), Body: body}}}
}

// Fingerprint recognizes a device of one driver from its Evidence. Match
// must not do I/O; detection fetches the Probes of every fingerprint
// once, unauthenticated (GET only), before scoring.
type Fingerprint struct {
	// Probes are extra paths to fetch beyond "/" ("/tr64desc.xml")
	Probes []string
	// Match scores the evidence; ok is false when nothing matched
	Match func(ev *Evidence) (m Match, ok bool)
}

// Match is one detection candidate.
type Match struct {
	// Driver is the registry name (set by detection)
	Driver  string `json:"driver"`
	Variant string `json:"variant,omitempty"`
	Model   string `json:"model,omitempty"`
	// Score: ScoreDefinitive for a model in a signed descriptor down to
	// ScoreWeak for a generic marker
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
}

// String implements fmt.Stringer ("zte score=90 variant=lua model=F8648P: reason").
func (m Match) String() string {
	s := fmt.Sprintf("%s score=%d", m.Driver, m.Score)
	if m.Variant != "" {
		s += " variant=" + m.Variant
	}
	if m.Model != "" {
		s += " model=" + m.Model
	}
	if m.Reason != "" {
		s += ": " + m.Reason
	}
	return s
}

// checkFingerprints validates a factory's fingerprints.
func checkFingerprints(fps []Fingerprint) error {
	for i, fp := range fps {
		if fp.Match == nil {
			return fmt.Errorf("fingerprint %d: no Match", i)
		}
		for _, p := range fp.Probes {
			if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, " #") {
				return fmt.Errorf("fingerprint %d: probe %q is not an absolute path", i, p)
			}
		}
	}
	return nil
}

// Matches scores ev against the fingerprints of f (the best match of
// each fingerprint, Driver set to f's name).
func (f Factory) Matches(ev *Evidence) []Match {
	var out []Match
	for _, fp := range f.Fingerprints {
		if m, ok := fp.Match(ev); ok {
			m.Driver = f.Info.Name
			out = append(out, m)
		}
	}
	return out
}
