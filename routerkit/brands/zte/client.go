package zte

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/netkit/tlspin"
)

// Defaults.
const (
	DefaultTimeout      = 15 * time.Second
	DefaultMaxBodyBytes = 4 << 20
	userAgent           = "zte-go/1"
)

// Errors.
var (
	// ErrAuth: the device rejected the credentials. Automatic re-login is
	// disabled after it (to avoid an account lockout) until Login is
	// called explicitly.
	ErrAuth = errors.New("zte: login rejected")
	// ErrLocked: the device locked the login after failed attempts.
	ErrLocked = errors.New("zte: login locked by the device")
	// ErrNotAvailable: the page/tag does not exist on this firmware or is
	// not visible to this account (e.g. PON optical info for "user").
	ErrNotAvailable = errors.New("zte: data not available for this device/account")
	// ErrSession: the session expired and could not be re-established.
	ErrSession = errors.New("zte: session expired")
	// ErrUnexpected: the device answered with something unexpected.
	ErrUnexpected = errors.New("zte: unexpected response")
	// ErrTooLarge: a response body exceeded Options.MaxBodyBytes.
	ErrTooLarge = errors.New("zte: response body too large")
	// ErrUnknownSource: Get was given a name that is neither a Sources
	// name, a known tag nor "view/tag".
	ErrUnknownSource = errors.New("zte: unknown source")
)

// LoginError carries the device's own message for a failed login.
type LoginError struct {
	Message string        // device message (never contains credentials)
	Locked  time.Duration // >0 when the device imposes a wait
	err     error         // ErrAuth or ErrLocked
}

func (e *LoginError) Error() string {
	msg := e.err.Error()
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if e.Locked > 0 {
		msg += fmt.Sprintf(" (retry in %s)", e.Locked)
	}
	return msg
}

func (e *LoginError) Unwrap() error { return e.err }

// Options tune a Client. The zero value is usable.
type Options struct {
	// UseSSL selects https (the device uses a self-signed certificate:
	// set PinSHA256 or InsecureSkipVerify)
	UseSSL bool
	// Port overrides the scheme default (80/443)
	Port int
	// Timeout per HTTP request (default 15s)
	Timeout time.Duration
	// PinSHA256 pins the server certificate (hex SHA-256, ':' optional)
	PinSHA256 string
	// InsecureSkipVerify accepts any certificate (https only)
	InsecureSkipVerify bool
	// MaxBodyBytes caps every response body (default 4 MiB)
	MaxBodyBytes int64
	// MinInterval spaces consecutive requests (0: no spacing)
	MinInterval time.Duration
	// KeepSecrets returns credential-like parameters (passwords, WiFi
	// keys, PPPoE user name) from Get, decrypted when the device marks
	// them as encoded. By default they are replaced with "".
	KeepSecrets bool
	// Transport overrides the HTTP transport (tests)
	Transport http.RoundTripper
	// Logger receives debug logs (never credentials); nil: discard
	Logger *slog.Logger
}

// Client is a ZTE F86xx web UI client. It is safe for concurrent use: the
// device keeps per-session "current page" state (a data tag can only be
// read after its page was loaded), so every read is serialized.
type Client struct {
	base    *url.URL
	user    string
	pass    string
	opts    Options
	httpc   *http.Client
	log     *slog.Logger
	maxBody int64

	mu          sync.Mutex
	loggedIn    bool
	token       string // latest _sessionTmpToken
	curView     string
	unavailable map[string]bool // views/tags known to be absent
	authErr     error           // sticky credential failure
	lockedUntil time.Time
	lastReq     time.Time
}

// New builds a client. host is "192.168.1.1", "192.168.1.1:8080" or a URL
// ("https://192.168.1.1"). No request is sent until the first call.
func New(host, user, pass string, opts *Options) (*Client, error) {
	var o Options
	if opts != nil {
		o = *opts
	}
	if host == "" {
		return nil, errors.New("zte: empty host")
	}
	scheme := "http"
	if o.UseSSL {
		scheme = "https"
	}
	raw := host
	if !strings.Contains(raw, "://") {
		raw = scheme + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("zte: invalid host %q", host)
	}
	if o.Port > 0 {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(o.Port))
	}
	u.Path, u.RawQuery, u.Fragment, u.User = "", "", "", nil
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxBodyBytes <= 0 {
		o.MaxBodyBytes = DefaultMaxBodyBytes
	}
	tr := o.Transport
	if tr == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		t.MaxIdleConnsPerHost = 1
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		var pin []byte
		if o.PinSHA256 != "" {
			p, err := tlspin.ParseFingerprint(o.PinSHA256)
			if err != nil {
				return nil, fmt.Errorf("zte: %w", err)
			}
			pin = p
		}
		tlspin.Apply(tlsCfg, nil, pin, o.InsecureSkipVerify)
		t.TLSClientConfig = tlsCfg
		tr = t
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	c := &Client{
		base:        u,
		user:        user,
		pass:        pass,
		opts:        o,
		log:         log,
		maxBody:     o.MaxBodyBytes,
		unavailable: map[string]bool{},
		httpc: &http.Client{
			Transport: tr,
			Timeout:   o.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	c.resetJar()
	return c, nil
}

// BaseURL is the device URL ("http://192.168.1.1").
func (c *Client) BaseURL() string { return c.base.String() }

func (c *Client) resetJar() {
	jar, _ := cookiejar.New(nil)
	c.httpc.Jar = jar
}

// LoggedIn reports whether the client believes it holds a session.
func (c *Client) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loggedIn
}

// Login opens a session (closing nothing: the device allows one web
// session, so a browser session of the same account may be replaced or
// may block this one). Calling Login clears a previous credential error.
func (c *Client) Login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authErr = nil
	return c.loginLocked(ctx)
}

// Logout closes the session. It is safe to call when not logged in.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loggedIn {
		return nil
	}
	form := url.Values{"IF_LogOff": {"1"}, "_sessionTOKEN": {c.token}}
	body, _, err := c.do(ctx, http.MethodPost, "/?_type=loginData&_tag=logout_entry", form)
	c.loggedIn, c.token, c.curView = false, "", ""
	c.resetJar()
	if err != nil {
		return fmt.Errorf("zte: logout: %w", err)
	}
	var res struct {
		NeedRefresh bool `json:"need_refresh"`
	}
	if json.Unmarshal(body, &res) != nil {
		return fmt.Errorf("%w: logout answer is not JSON", ErrUnexpected)
	}
	return nil
}

type loginEntry struct {
	SessToken   string `json:"sess_token"`
	LoginErrMsg string `json:"loginErrMsg"`
	PromptMsg   string `json:"promptMsg"`
	LockingTime any    `json:"lockingTime"`
	NeedRefresh bool   `json:"login_need_refresh"`
}

func (le *loginEntry) lock() time.Duration {
	switch v := le.LockingTime.(type) {
	case float64:
		return time.Duration(v) * time.Second
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return time.Duration(n) * time.Second
	}
	return 0
}

func (c *Client) loginLocked(ctx context.Context) error {
	if c.authErr != nil {
		return c.authErr
	}
	if d := time.Until(c.lockedUntil); d > 0 {
		return &LoginError{err: ErrLocked, Locked: d.Round(time.Second)}
	}
	c.loggedIn, c.token, c.curView = false, "", ""
	c.resetJar()
	c.log.Debug("zte: login", "host", c.base.Host)

	// 0. the login page sets the SID / _TESTCOOKIESUPPORT cookies
	if _, _, err := c.do(ctx, http.MethodGet, "/", nil); err != nil {
		return fmt.Errorf("zte: login: %w", err)
	}
	// 1. session token (and lock state) for the login form
	body, _, err := c.do(ctx, http.MethodGet, "/?_type=loginData&_tag=login_entry", nil)
	if err != nil {
		return fmt.Errorf("zte: login: %w", err)
	}
	var pre loginEntry
	if err := json.Unmarshal(body, &pre); err != nil {
		return fmt.Errorf("%w: login_entry is not JSON", ErrUnexpected)
	}
	if d := pre.lock(); d > 0 {
		c.lockedUntil = time.Now().Add(d)
		return &LoginError{err: ErrLocked, Message: firstNonEmpty(pre.PromptMsg, pre.LoginErrMsg), Locked: d}
	}
	// 2. random salt
	body, _, err = c.do(ctx, http.MethodGet, "/?_type=loginData&_tag=login_token", nil)
	if err != nil {
		return fmt.Errorf("zte: login: %w", err)
	}
	salt := xmlRootText(body)
	if salt == "" {
		return fmt.Errorf("%w: empty login token", ErrUnexpected)
	}
	// 3. credentials
	form := url.Values{
		"action":        {"login"},
		"Password":      {hashPassword(c.pass, salt)},
		"Username":      {c.user},
		"_sessionTOKEN": {pre.SessToken},
	}
	body, _, err = c.do(ctx, http.MethodPost, "/?_type=loginData&_tag=login_entry", form)
	if err != nil {
		return fmt.Errorf("zte: login: %w", err)
	}
	var res loginEntry
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("%w: login answer is not JSON", ErrUnexpected)
	}
	if !res.NeedRefresh {
		le := &LoginError{err: ErrAuth, Message: firstNonEmpty(res.LoginErrMsg, res.PromptMsg)}
		if d := res.lock(); d > 0 {
			// A temporary lock: block logins until it expires (lockedUntil),
			// then allow one new attempt - not sticky, or a daemon would stay
			// dead after a lock it has already waited out
			le.err, le.Locked = ErrLocked, d
			c.lockedUntil = time.Now().Add(d)
			return le
		}
		// Refused credentials are sticky: retrying them only feeds the
		// device lockout. Login() clears it explicitly
		c.authErr = le
		return le
	}
	c.token = res.SessToken
	// 4. the UI reloads "/" after login; do the same (it also confirms
	// the session and carries the page token)
	body, _, err = c.do(ctx, http.MethodGet, "/", nil)
	if err != nil {
		return fmt.Errorf("zte: login: %w", err)
	}
	if isLoginPage(body) {
		return fmt.Errorf("%w: still on the login page after login (another session active?)", ErrSession)
	}
	if t := extractToken(body); t != "" {
		c.token = t
	}
	c.loggedIn = true
	c.log.Debug("zte: logged in")
	return nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// xmlRootText returns the text of <ajax_response_xml_root>…</…>.
func xmlRootText(b []byte) string {
	s := string(b)
	if i := strings.Index(s, "<ajax_response_xml_root>"); i >= 0 {
		s = s[i+len("<ajax_response_xml_root>"):]
		if j := strings.Index(s, "<"); j >= 0 {
			s = s[:j]
		}
		return strings.TrimSpace(s)
	}
	return ""
}

func isLoginPage(b []byte) bool {
	return bytes.Contains(b, []byte(`id="Frm_Username"`)) || bytes.Contains(b, []byte("_tag=login_token"))
}

func isNotFoundPage(status int, b []byte) bool {
	return status == http.StatusNotFound || bytes.Contains(b[:min(len(b), 512)], []byte("<title>404 Not Found</title>"))
}

// do sends one request. form != nil means POST form-urlencoded.
func (c *Client) do(ctx context.Context, method, pathQuery string, form url.Values) ([]byte, int, error) {
	if c.opts.MinInterval > 0 && !c.lastReq.IsZero() {
		if wait := c.opts.MinInterval - time.Since(c.lastReq); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, 0, ctx.Err()
			case <-t.C:
			}
		}
	}
	defer func() { c.lastReq = time.Now() }()
	var rd io.Reader
	if form != nil {
		rd = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base.String()+pathQuery, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "*/*")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	c.log.Debug("zte: request", "method", method, "path", redactPath(pathQuery))
	resp, err := c.httpc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = fmt.Errorf("%s %s: %w", method, redactPath(pathQuery), ue.Err)
		}
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(body)) > c.maxBody {
		return nil, resp.StatusCode, fmt.Errorf("%w (%s)", ErrTooLarge, redactPath(pathQuery))
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return body, resp.StatusCode, fmt.Errorf("%w: redirect %d to %q", ErrUnexpected, resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.StatusCode >= 500 {
		return body, resp.StatusCode, fmt.Errorf("%w: HTTP %d", ErrUnexpected, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}

// redactPath keeps only the routing parameters of a URL for logs/errors.
func redactPath(p string) string {
	i := strings.IndexByte(p, '?')
	if i < 0 {
		return p
	}
	q, err := url.ParseQuery(p[i+1:])
	if err != nil {
		return p[:i]
	}
	return p[:i] + "?_type=" + q.Get("_type") + "&_tag=" + q.Get("_tag")
}

// errStale signals "the session or page context is gone".
var errStale = errors.New("zte: stale page context")

// loadViewLocked loads a UI page (menuView) so its data tags can be read.
func (c *Client) loadViewLocked(ctx context.Context, view string) error {
	body, status, err := c.do(ctx, http.MethodGet, "/?_type=menuView&_tag="+url.QueryEscape(view)+"&Menu3Location=0", nil)
	if err != nil {
		return err
	}
	if isNotFoundPage(status, body) || isLoginPage(body) {
		c.curView = ""
		return errStale
	}
	if t := extractToken(body); t != "" {
		c.token = t
	}
	c.curView = view
	return nil
}

// readTagLocked loads (if needed) the view and reads one data tag.
func (c *Client) readTagLocked(ctx context.Context, view, tag string) (*Response, error) {
	fresh := false
	if c.curView != view {
		if err := c.loadViewLocked(ctx, view); err != nil {
			return nil, err
		}
		fresh = true
	}
	for {
		body, status, err := c.do(ctx, http.MethodGet, "/?_type=menuData&_tag="+tag, nil)
		if err != nil {
			return nil, err
		}
		if isNotFoundPage(status, body) {
			return nil, errStale
		}
		r, err := Parse(body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", redactPath("/?_tag="+tag), err)
		}
		if strings.EqualFold(r.ErrorStr(), "SessionTimeout") {
			if !fresh { // page context may have moved on: reload once
				if err := c.loadViewLocked(ctx, view); err != nil {
					return nil, err
				}
				fresh = true
				continue
			}
			return nil, errStale
		}
		r.Tag = tagName(tag)
		return r, nil
	}
}

// probeLocked reports whether the session is still alive by loading a
// page every account can see.
func (c *Client) probeLocked(ctx context.Context) (bool, error) {
	err := c.loadViewLocked(ctx, probeView)
	if errors.Is(err, errStale) {
		return false, nil
	}
	return err == nil, err
}

// fetch reads the given (view, tag) pairs in one page context, logging in
// or re-logging in as needed. A missing view/tag yields ErrNotAvailable.
func (c *Client) fetch(ctx context.Context, view string, tags ...string) ([]*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unavailable[view] {
		return nil, fmt.Errorf("%w (page %s)", ErrNotAvailable, view)
	}
	for _, t := range tags {
		if c.unavailable[view+"/"+t] {
			return nil, fmt.Errorf("%w (%s)", ErrNotAvailable, tagName(t))
		}
	}
	relogged := false
	// A tag is marked unavailable only after it failed twice with the
	// session confirmed alive (the first failure can be transient, e.g. a
	// browser session briefly taking the page over)
	retried := false
	for {
		if !c.loggedIn {
			if err := c.loginLocked(ctx); err != nil {
				return nil, err
			}
			relogged = true
		}
		out := make([]*Response, 0, len(tags))
		var err error
		failed := ""
		for _, t := range tags {
			var r *Response
			r, err = c.readTagLocked(ctx, view, t)
			if err != nil {
				failed = t
				break
			}
			out = append(out, r)
		}
		if err == nil {
			for _, r := range out {
				c.postProcessLocked(r)
			}
			return out, nil
		}
		if !errors.Is(err, errStale) {
			return nil, err
		}
		// Session gone, or this page/tag does not exist for us?
		alive, perr := c.probeLocked(ctx)
		if perr != nil {
			return nil, perr
		}
		if alive {
			if c.curView != view && view != probeView {
				// the probe page loaded but ours did not: page missing
				if c.loadViewLocked(ctx, view) != nil {
					c.unavailable[view] = true
					return nil, fmt.Errorf("%w (page %s)", ErrNotAvailable, view)
				}
			}
			if !retried {
				retried = true
				continue
			}
			c.unavailable[view+"/"+failed] = true
			return nil, fmt.Errorf("%w (%s)", ErrNotAvailable, tagName(failed))
		}
		if relogged {
			c.loggedIn = false
			return nil, ErrSession
		}
		c.log.Debug("zte: session expired, logging in again")
		c.loggedIn = false
	}
}

// DecodeValue decrypts a parameter the device marks as encoded (listed in
// Response.Encoded), using the current page token. It must be called
// right after the read that returned the value.
func (c *Client) DecodeValue(v string) (string, error) {
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	return decodeParaValue(tok, v)
}

// postProcessLocked applies the secret policy to a response (the page
// token used for decoding is the one of the page just loaded).
func (c *Client) postProcessLocked(r *Response) {
	if c.opts.KeepSecrets {
		if len(r.Encoded) == 0 {
			return
		}
		enc := map[string]bool{}
		for _, n := range r.Encoded {
			enc[n] = true
		}
		tok := c.token
		for _, insts := range r.Objects {
			for _, in := range insts {
				for k, v := range in {
					if enc[k] {
						if d, err := decodeParaValue(tok, v); err == nil {
							in[k] = d
						}
					}
				}
			}
		}
		return
	}
	for _, insts := range r.Objects {
		for _, in := range insts {
			for k := range in {
				if secretName(k) {
					in[k] = ""
				}
			}
		}
	}
}
