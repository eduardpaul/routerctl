// Package httprec records HTTP exchanges into a JSON-lines session and
// replays them offline: an http.RoundTripper on each side, for brand
// libraries whose client takes a custom transport.
//
// A recorded session holds whatever the device answered (MACs, SSIDs,
// addresses): Redact removes credentials from the requests, cookies and
// chosen query parameters, but response bodies are stored as received and
// must be sanitized before a session is shared.
//
// Replay matches a request by method and URL (path and query, with
// redacted parameters masked and parameters sorted); request bodies are
// not compared, because they carry per-session tokens and password
// digests. Exchanges with the same key are served in recorded order and
// the last one repeats, so a session recorded once can be polled many
// times. An unknown request gets a 404.
package httprec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
)

// Masked replaces redacted values.
const Masked = "REDACTED"

// Exchange is one recorded request and its answer.
type Exchange struct {
	Method string `json:"method"`
	// URL is the path and query ("/?_type=menuData&_tag=x"), redacted
	URL string `json:"url"`
	// Body is the request body (form fields redacted)
	Body string `json:"body,omitempty"`
	// Status, Header (kept headers only) and Response are the answer
	Status   int                 `json:"status"`
	Header   map[string][]string `json:"header,omitempty"`
	Response string              `json:"response"`
}

// Redact says what never reaches a session file.
type Redact struct {
	// Query and Form parameters whose values are masked
	Query, Form []string
	// Headers are the response headers kept (canonical names); the values
	// of Set-Cookie are always masked
	Headers []string
}

// DefaultHeaders are the response headers kept by default.
var DefaultHeaders = []string{"Content-Type", "Location", "Set-Cookie"}

func (r Redact) headers() []string {
	if r.Headers == nil {
		return DefaultHeaders
	}
	return r.Headers
}

// key is the replay key of a request: method and the canonical,
// redacted URL.
func (r Redact) key(method string, u *url.URL) string {
	return method + " " + r.url(u)
}

func (r Redact) url(u *url.URL) string {
	q := u.Query()
	for _, k := range r.Query {
		if q.Has(k) {
			q.Set(k, Masked)
		}
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

func (r Redact) body(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(r.Form) == 0 {
		return string(b)
	}
	form, err := url.ParseQuery(string(b))
	if err != nil {
		return Masked
	}
	for _, k := range r.Form {
		if form.Has(k) {
			form.Set(k, Masked)
		}
	}
	return form.Encode()
}

func maskCookie(v string) string {
	name, rest, ok := strings.Cut(v, "=")
	if !ok {
		return Masked
	}
	if _, attrs, ok := strings.Cut(rest, ";"); ok {
		return name + "=" + Masked + ";" + attrs
	}
	return name + "=" + Masked
}

// ---- recording ----

// Recorder is an http.RoundTripper that records every exchange made
// through it (safe for concurrent use).
type Recorder struct {
	next   http.RoundTripper
	redact Redact
	// MaxBody caps the recorded size of one response (0: 4 MiB)
	MaxBody int64

	mu sync.Mutex
	ex []Exchange
}

// NewRecorder records the exchanges of next (http.DefaultTransport when
// nil).
func NewRecorder(next http.RoundTripper, r Redact) *Recorder {
	if next == nil {
		next = http.DefaultTransport
	}
	return &Recorder{next: next, redact: r}
}

// RoundTrip implements http.RoundTripper.
func (rec *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil && req.GetBody != nil {
		b, err := req.GetBody()
		if err == nil {
			reqBody, _ = io.ReadAll(b)
			b.Close()
		}
	}
	resp, err := rec.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	limit := rec.MaxBody
	if limit <= 0 {
		limit = 4 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if int64(len(body)) > limit {
		body = body[:limit]
	}
	ex := Exchange{
		Method:   req.Method,
		URL:      rec.redact.url(req.URL),
		Body:     rec.redact.body(reqBody),
		Status:   resp.StatusCode,
		Response: string(body),
	}
	for _, h := range rec.redact.headers() {
		vs := resp.Header.Values(h)
		if len(vs) == 0 {
			continue
		}
		if ex.Header == nil {
			ex.Header = map[string][]string{}
		}
		for _, v := range vs {
			if strings.EqualFold(h, "Set-Cookie") {
				v = maskCookie(v)
			}
			ex.Header[http.CanonicalHeaderKey(h)] = append(ex.Header[http.CanonicalHeaderKey(h)], v)
		}
	}
	rec.mu.Lock()
	rec.ex = append(rec.ex, ex)
	rec.mu.Unlock()
	return resp, nil
}

// Exchanges returns a copy of the recorded exchanges.
func (rec *Recorder) Exchanges() []Exchange {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]Exchange(nil), rec.ex...)
}

// Len returns the number of recorded exchanges.
func (rec *Recorder) Len() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.ex)
}

// Save writes the session (JSON lines, mode 0600: it holds private data).
func (rec *Recorder) Save(path string) error { return Save(path, rec.Exchanges()) }

// Save writes exchanges as a session file (mode 0600).
func Save(path string, ex []Exchange) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := Write(f, ex); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Write writes exchanges as JSON lines.
func Write(w io.Writer, ex []Exchange) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, e := range ex {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// Read reads a JSON-lines session.
func Read(r io.Reader) ([]Exchange, error) {
	var out []Exchange
	dec := json.NewDecoder(r)
	for {
		var e Exchange
		err := dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("httprec: exchange %d: %w", len(out)+1, err)
		}
		if e.Method == "" || e.URL == "" || e.Status == 0 {
			return nil, fmt.Errorf("httprec: exchange %d: method, url and status are required", len(out)+1)
		}
		out = append(out, e)
	}
}

// Load reads a session file.
func Load(path string) ([]Exchange, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// ---- replay ----

// Replayer is an http.RoundTripper that answers from recorded exchanges
// (safe for concurrent use).
type Replayer struct {
	redact Redact

	mu       sync.Mutex
	queues   map[string][]Exchange
	next     map[string]int
	requests int
	misses   []string
}

// NewReplayer serves ex. r must mask the same query parameters as the
// recording did.
func NewReplayer(ex []Exchange, r Redact) *Replayer {
	p := &Replayer{redact: r, queues: map[string][]Exchange{}, next: map[string]int{}}
	for _, e := range ex {
		u, err := url.Parse(e.URL)
		if err != nil {
			continue
		}
		k := r.key(e.Method, u)
		p.queues[k] = append(p.queues[k], e)
	}
	return p
}

// LoadReplayer reads a session file and serves it.
func LoadReplayer(path string, r Redact) (*Replayer, error) {
	ex, err := Load(path)
	if err != nil {
		return nil, err
	}
	return NewReplayer(ex, r), nil
}

// RoundTrip implements http.RoundTripper.
func (p *Replayer) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		req.Body.Close()
	}
	k := p.redact.key(req.Method, req.URL)
	p.mu.Lock()
	p.requests++
	q := p.queues[k]
	var e Exchange
	found := len(q) > 0
	if found {
		i := p.next[k]
		if i >= len(q) {
			i = len(q) - 1
		}
		e = q[i]
		p.next[k] = i + 1
	} else {
		p.misses = append(p.misses, k)
	}
	p.mu.Unlock()
	if !found {
		e = Exchange{Status: http.StatusNotFound, Response: "httprec: no recorded exchange for " + k}
	}
	h := http.Header{}
	for name, vs := range e.Header {
		for _, v := range vs {
			h.Add(name, v)
		}
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status)),
		StatusCode:    e.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(e.Response)),
		ContentLength: int64(len(e.Response)),
		Request:       req,
	}, nil
}

// Requests counts the requests served (answered or not).
func (p *Replayer) Requests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

// Misses lists the keys of requests that had no recorded exchange.
func (p *Replayer) Misses() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.misses...)
}

// Keys lists the recorded keys, sorted.
func (p *Replayer) Keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.queues))
	for k := range p.queues {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Rewind starts every queue from its first exchange again.
func (p *Replayer) Rewind() {
	p.mu.Lock()
	p.next = map[string]int{}
	p.mu.Unlock()
}
