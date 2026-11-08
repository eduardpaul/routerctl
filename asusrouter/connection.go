package asusrouter

// Mirrors asusrouter/connection.py.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/netkit/tlspin"
)

var tlsspin = tlspin.Apply

// loginCtxKey marks a context as belonging to the login request chain
// (Python: the current task is the connect task).
type loginCtxKey struct{}

func inLoginContext(ctx context.Context) bool { return ctx.Value(loginCtxKey{}) != nil }

// A credential change bounces. Wait and try to reconnect
// Fix attempts so we don't hit captcha or lockout on fail
const (
	credentialsReconnectInitialDelay = 5 * time.Second
	credentialsReconnectInterval     = 5 * time.Second
	credentialsReconnectMaxAttempts  = 3
)

// Upper bound of a response body. The Python library reads unbounded
// bodies; the Go port caps them (security review finding) - the largest
// real reply (a full syslog) is well under this.
const maxResponseBytes = 32 << 20

// ConnectionFallback mirrors `ConnectionFallback`.
type ConnectionFallback string

// Connection fallback strategies.
const (
	ConnectionFallbackHTTP  ConnectionFallback = "http"
	ConnectionFallbackHTTPS ConnectionFallback = "https"
)

// Response is one wire reply: what `async_query` returns.
type Response struct {
	Status  int
	Headers http.Header
	Content string
}

// Transport is the seam shared with the Python library (see PARITY.md
// "Seams"): `Connection.async_query` and the connection lifecycle. The
// HTTP Connection implements it; parity/replay sessions implement it too.
type Transport interface {
	Connect(ctx context.Context) (bool, error)
	Disconnect(ctx context.Context) (bool, error)
	Connected() bool
	ResetAuth()
	Query(ctx context.Context, ep endpoint.AREndpoint, payload *string, requestType consts.RequestType) (Response, error)
	Close() error
}

// ResponseCallback mirrors `response_callback`: called with every reply.
type ResponseCallback func(ep endpoint.AREndpoint, payload *string, resp Response)

// GenerateCredentials mirrors `generate_credentials`.
func GenerateCredentials(username, password string) (string, map[string]string, error) {
	auth := username + ":" + password
	for i := 0; i < len(auth); i++ {
		if auth[i] >= 0x80 {
			// Python raises UnicodeEncodeError (ascii codec); do not echo
			// the credentials in the error (security review finding)
			return "", nil, fmt.Errorf("credentials must be ASCII")
		}
	}
	token := base64.StdEncoding.EncodeToString([]byte(auth))
	return "login_authorization=" + token, map[string]string{"user-agent": consts.UserAgent}, nil
}

// ConnectionOptions are the keyword arguments of `Connection.__init__`.
type ConnectionOptions struct {
	Port             int
	UseSSL           bool
	HTTPClient       *http.Client
	Timeout          time.Duration
	ResponseCallback ResponseCallback
	Config           map[config.ARConnectionConfigKey]any
	// RootCAs / PinnedCertSHA256 are Go-only TLS options (not in Python):
	// verify the router certificate against a CA pool or a SHA-256
	// fingerprint pin instead of turning verification off.
	RootCAs          *x509.CertPool
	PinnedCertSHA256 []byte
}

// Connection mirrors `Connection`: the HTTP(S) link to the device.
type Connection struct {
	mu sync.Mutex

	config        *config.ARConnectionConfig
	usedFallbacks map[ConnectionFallback]bool
	// fallbackMu serializes fallback transitions; fallbackGen counts them so
	// a request that failed on a config another request already replaced
	// retries instead of advancing the state machine again
	fallbackMu  sync.Mutex
	fallbackGen int

	token     string
	header    map[string]string
	connected bool
	// Downgrade expected connection errors to debug
	suppressErrors bool
	timeout        time.Duration

	// Serializes login attempts (Python: single in-flight connect task)
	loginMu sync.Mutex
	// Set once the device refused the credentials, blocks new logins
	// until they are changed: every retry feeds the device lockout
	credentialsRefused bool

	hostname    string
	username    string
	password    string
	authPayload string
	authHeaders map[string]string

	requestSem chan struct{}

	responseCallback ResponseCallback

	client       *http.Client
	manageClient bool
	tlsOpts      ConnectionOptions
}

// NewConnection mirrors `Connection.__init__`.
func NewConnection(hostname, username, password string, opts ConnectionOptions) (*Connection, error) {
	slog.Debug("Initializing a new connection", "host", identifiers.NewHostname(hostname))
	payload, headers, err := GenerateCredentials(username, password)
	if err != nil {
		return nil, err
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = consts.DefaultTimeout
	}
	c := &Connection{
		config:           config.NewARConnectionConfig(),
		usedFallbacks:    map[ConnectionFallback]bool{},
		timeout:          timeout,
		hostname:         hostname,
		username:         username,
		password:         password,
		authPayload:      payload,
		authHeaders:      headers,
		responseCallback: opts.ResponseCallback,
		tlsOpts:          opts,
	}
	port := opts.Port
	if port == 0 {
		if opts.UseSSL {
			port = consts.DefaultPortHTTPS
		} else {
			port = consts.DefaultPortHTTP
		}
	}
	c.config.Set(config.ARConnectionConfigKeyPort, port)
	c.config.Set(config.ARConnectionConfigKeyUseSsl, opts.UseSSL)
	for k, v := range opts.Config {
		c.config.Set(k, v)
	}

	// Bound concurrent HTTP requests; 1 keeps them fully serialized.
	// Read once here - changing the config later has no effect
	maxConcurrent := config.SafeIntConfig(c.config.Get(config.ARConnectionConfigKeyMaxConcurrentRequests))
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	c.requestSem = make(chan struct{}, maxConcurrent)

	if opts.HTTPClient != nil {
		c.client = opts.HTTPClient
	}
	return c, nil
}

// Config returns the connection config.
func (c *Connection) Config() *config.ARConnectionConfig { return c.config }

// Connected reports the connection status.
func (c *Connection) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// HTTP returns the scheme.
func (c *Connection) HTTP() string {
	if b, _ := c.config.Get(config.ARConnectionConfigKeyUseSsl).(bool); b {
		return "https"
	}
	return "http"
}

// Password returns the current password.
func (c *Connection) Password() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.password
}

// Username returns the current username.
func (c *Connection) Username() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

// Port returns the port number.
func (c *Connection) Port() int {
	return config.SafeIntConfig(c.config.Get(config.ARConnectionConfigKeyPort))
}

// Webpanel returns the web panel URL.
func (c *Connection) Webpanel() string {
	host := c.hostname
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]" // IPv6 literal
	}
	return fmt.Sprintf("%s://%s:%d", c.HTTP(), host, c.Port())
}

// SetErrorSuppression downgrades expected connection errors to debug.
func (c *Connection) SetErrorSuppression(suppress bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.suppressErrors = suppress
}

func (c *Connection) logError(msg string, args ...any) {
	c.mu.Lock()
	suppress := c.suppressErrors
	c.mu.Unlock()
	if suppress {
		slog.Debug(msg, args...)
	} else {
		slog.Error(msg, args...)
	}
}

// ---------------------------
// Session management -->
// ---------------------------

func (c *Connection) createClient() *http.Client {
	verify, _ := c.config.Get(config.ARConnectionConfigKeyVerifySsl).(bool)
	tlsConf := &tls.Config{}
	// VERIFY_SSL=False (the Python default) skips verification unless a
	// CA pool and/or a pin is configured; both together check both
	tlsspin(tlsConf, c.tlsOpts.RootCAs, c.tlsOpts.PinnedCertSHA256, !verify)
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: c.timeout}).DialContext,
		TLSClientConfig:     tlsConf,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     60 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   c.timeout,
		// Never follow redirects: a 307/308 would replay the login body
		// to another origin (security review finding)
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (c *Connection) ensureClient() *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		c.client = c.createClient()
		c.manageClient = true
	}
	return c.client
}

// Close mirrors `async_close_session`.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.manageClient && c.client != nil {
		c.client.CloseIdleConnections()
		c.client = nil
	}
	return nil
}

// ---------------------------
// Connection management -->
// ---------------------------

// Connect mirrors `async_connect`: log in and get a new auth token.
func (c *Connection) Connect(ctx context.Context) (bool, error) {
	if c.Connected() {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.login(ctx)
}

func (c *Connection) login(ctx context.Context) (bool, error) {
	// Serialize logins: a concurrent caller waits and then sees the
	// connection established by the first one
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return true, nil
	}
	if c.credentialsRefused {
		c.mu.Unlock()
		return false, arerror.NewAccessError(
			"Credentials were refused before, change them to log in",
			int(endpoint.ARAccessErrorCredentials), map[string]any{})
	}
	payload, headers := c.authPayload, c.authHeaders
	c.mu.Unlock()

	slog.Debug("Requesting authorization")
	p := payload
	ctx = context.WithValue(ctx, loginCtxKey{}, true)
	resp, err := c.sendRequest(ctx, endpoint.AREndpointLogin, &p, headers, consts.RequestTypePost)
	if err != nil {
		if errors.Is(err, arerror.ErrSSLCertificate) {
			return false, arerror.NewAccessError(
				fmt.Sprintf("Cannot access %s due to the SSL certificate error", endpoint.AREndpointLogin),
				int(endpoint.ARAccessErrorUnknown), map[string]any{})
		}
		if code, ok := arerror.AccessCode(err); ok && code == int(endpoint.ARAccessErrorCredentials) {
			c.mu.Lock()
			c.credentialsRefused = true
			c.mu.Unlock()
			c.logError("Device refused the credentials. No new login is attempted until they are changed",
				"host", identifiers.NewHostname(c.hostname))
		}
		return false, err
	}

	var body map[string]any
	if json.Unmarshal([]byte(resp.Content), &body) != nil {
		c.logError("Invalid login response", "host", identifiers.NewHostname(c.hostname))
		return false, nil
	}
	token, _ := body["asus_token"].(string)
	if token == "" {
		c.logError("No token received")
		return false, nil
	}

	c.mu.Lock()
	if !c.connected {
		c.token = token
		c.header = map[string]string{
			"user-agent": consts.UserAgent,
			"cookie":     "asus_token=" + token,
		}
		c.connected = true
	}
	c.mu.Unlock()
	slog.Debug("Connected", "host", identifiers.NewHostname(c.hostname))
	return true, nil
}

// Disconnect mirrors `async_disconnect`.
func (c *Connection) Disconnect(ctx context.Context) (bool, error) {
	if !c.Connected() {
		return true, nil
	}
	_, err := c.sendRequest(ctx, endpoint.AREndpointLogout, nil, nil, consts.RequestTypePost)
	if err != nil && !errors.Is(err, arerror.ErrLogout) {
		slog.Debug("Error while disconnecting", "error", err)
		return false, nil
	}
	// Router signals successful logout with an error response
	c.ResetAuth()
	return true, nil
}

// ResetAuth mirrors `reset_auth`: clear auth state when the connection
// is no longer valid.
func (c *Connection) ResetAuth() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected {
		return
	}
	c.connected = false
	c.token = ""
	c.header = nil
}

// ClearCredentialsRefused re-allows logins after the device refused the
// credentials (Go only: Python offers this only through
// `async_set_credentials`). Call it for an explicit, user- or
// daemon-initiated reconnect; never in a tight loop (device lockout).
func (c *Connection) ClearCredentialsRefused() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.credentialsRefused = false
}

// SetCredentials mirrors `async_set_credentials`: swap the stored
// credentials and re-establish the session.
func (c *Connection) SetCredentials(ctx context.Context, username, password string) (bool, error) {
	payload, headers, err := GenerateCredentials(username, password)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.username, c.password = username, password
	c.authPayload, c.authHeaders = payload, headers
	prevSuppress := c.suppressErrors
	c.mu.Unlock()
	// Drop the stale auth
	c.ResetAuth()

	// httpd is restarting; expected login failures stay at debug
	c.SetErrorSuppression(true)
	defer c.SetErrorSuppression(prevSuppress)

	if err := sleepCtx(ctx, credentialsReconnectInitialDelay); err != nil {
		return false, err
	}
	for attempt := 0; attempt < credentialsReconnectMaxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, credentialsReconnectInterval); err != nil {
				return false, err
			}
		}
		// A refusal here is a reloading httpd, not a verdict on the
		// new credentials
		c.mu.Lock()
		c.credentialsRefused = false
		c.mu.Unlock()
		ok, err := c.Connect(ctx)
		if ok {
			return true, nil
		}
		if code, isAccess := arerror.AccessCode(err); isAccess && code != int(endpoint.ARAccessErrorCredentials) {
			slog.Error("Login was changed, but the device refused the new session. "+
				"The new credentials are stored; check the device for a captcha or a "+
				"temporary login lock before reconnecting", "error", endpoint.ARAccessError(code).Name())
			return false, nil
		}
	}
	// Keep a manual reconnect possible
	c.mu.Lock()
	c.credentialsRefused = false
	c.mu.Unlock()
	slog.Error("Login was changed, but the new session could not be established. "+
		"The new credentials are stored; reconnect manually if the device stays unreachable",
		"attempts", credentialsReconnectMaxAttempts)
	return false, nil
}

// ---------------------------
// Request handling -->
// ---------------------------

// Query mirrors `async_query`: send a request to the device, logging in
// first when needed.
func (c *Connection) Query(ctx context.Context, ep endpoint.AREndpoint, payload *string, requestType consts.RequestType) (Response, error) {
	if !c.Connected() {
		slog.Debug("Not connected. Connecting...")
		if _, err := c.Connect(ctx); err != nil {
			return Response{}, err
		}
	}
	if !c.Connected() {
		return Response{}, arerror.NewTimeoutError("Connection timed out — could not establish initial connection")
	}
	return c.sendRequest(ctx, ep, payload, nil, requestType)
}

func (c *Connection) sendRequest(ctx context.Context, ep endpoint.AREndpoint, payload *string, headers map[string]string, requestType consts.RequestType) (Response, error) {
	logRequest(ep, payload)

	c.fallbackMu.Lock()
	gen := c.fallbackGen
	c.fallbackMu.Unlock()

	resp, err := c.makeRequest(ctx, ep, payload, headers, requestType)
	if err != nil {
		return c.handleTransportError(ctx, err, gen, ep, payload, headers, requestType)
	}
	if err := checkResponse(ep, resp); err != nil {
		return Response{}, err
	}

	c.mu.Lock()
	used := len(c.usedFallbacks) > 0
	c.mu.Unlock()
	if used {
		if b, _ := c.config.Get(config.ARConnectionConfigKeyAllowMultipleFallbacks).(bool); !b {
			// Config discovery complete — lock connection parameters
			// so no further fallbacks alter the working config.
			c.config.Set(config.ARConnectionConfigKeyAllowFallback, false)
		}
		c.mu.Lock()
		c.usedFallbacks = map[ConnectionFallback]bool{}
		c.mu.Unlock()
	}
	return resp, nil
}

func logRequest(ep endpoint.AREndpoint, payload *string) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	if payload == nil {
		slog.Debug("Sending request", "endpoint", string(ep))
		return
	}
	// Payloads are logged only for non-sensitive endpoints (Go keeps
	// every write endpoint redacted: security review finding)
	if endpoint.GetEndpointPayloadSensitivity(ep) > endpoint.GetEndpointPayloadSensitivity(endpoint.AREndpointFetchData) ||
		ep == endpoint.AREndpointPushData {
		slog.Debug("Sending request", "endpoint", string(ep), "payload", "-=REDACTED=-")
		return
	}
	slog.Debug("Sending request", "endpoint", string(ep), "payload", *payload)
}

func checkResponse(ep endpoint.AREndpoint, resp Response) error {
	if resp.Status == int(consts.HTTPStatusNotFound) {
		return arerror.New404Error(fmt.Sprintf("Endpoint %s not found", ep))
	}
	if resp.Status != int(consts.HTTPStatusOk) {
		return arerror.NewAccessError(fmt.Sprintf("Cannot access %s, status %d", ep, resp.Status),
			int(endpoint.ARAccessErrorUnknown), map[string]any{})
	}
	if strings.Contains(resp.Content, "error_status") {
		return endpoint.HandleAccessError(ep, resp.Status, resp.Content)
	}
	return nil
}

func (c *Connection) makeRequest(ctx context.Context, ep endpoint.AREndpoint, payload *string, headers map[string]string, requestType consts.RequestType) (Response, error) {
	// Bound the live request: hold a permit for the whole exchange,
	// including reading the body. Taken first, so the URL and headers
	// below reflect any fallback applied while this request waited
	select {
	case c.requestSem <- struct{}{}:
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
	defer func() { <-c.requestSem }()

	if headers == nil {
		c.mu.Lock()
		headers = c.header
		c.mu.Unlock()
	}
	u := c.Webpanel() + "/" + string(ep)
	var body io.Reader
	if requestType == consts.RequestTypeGet && payload != nil && *payload != "" {
		u += "?" + strings.ReplaceAll(*payload, ";", "&")
	}
	if requestType == consts.RequestTypePost && payload != nil && *payload != "" {
		// Some CGIs use a standard form parser and need the body verbatim;
		// re-quoting would corrupt the `=`/`&` separators
		if endpoint.GetEndpointRawPayload(ep) {
			body = strings.NewReader(*payload)
		} else {
			body = strings.NewReader(pyQuote(*payload))
		}
	}
	method := http.MethodPost
	if requestType == consts.RequestTypeGet {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return Response{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		// aiohttp sends str data as form-encoded
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.ensureClient().Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, err
	}
	if len(raw) > maxResponseBytes {
		return Response{}, arerror.NewConnectionError(fmt.Sprintf("Response from %s exceeds %d bytes", ep, maxResponseBytes))
	}
	content := string(raw)
	if !utf8.ValidString(content) {
		slog.Debug("Cannot decode response. Will ignore errors")
		content = strings.ToValidUTF8(content, "")
	}
	out := Response{Status: resp.StatusCode, Headers: resp.Header, Content: content}
	if c.responseCallback != nil {
		c.responseCallback(ep, payload, out)
	}
	return out, nil
}

// pyQuote mirrors Python `urllib.parse.quote(s)` (safe="/").
func pyQuote(s string) string {
	var b strings.Builder
	for _, ch := range []byte(s) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') ||
			ch == '_' || ch == '.' || ch == '-' || ch == '~' || ch == '/' {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

var _ = url.QueryEscape // keep net/url for callers building GET payloads

func (c *Connection) handleTransportError(ctx context.Context, err error, gen int, ep endpoint.AREndpoint, payload *string, headers map[string]string, requestType consts.RequestType) (Response, error) {
	if ctx.Err() != nil || isTimeout(err) {
		return Response{}, arerror.NewTimeoutError(fmt.Sprintf("Data cannot be retrieved. Connection failed: %v", err))
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var pinErr *tlspin.PinError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) || errors.As(err, &pinErr) {
		strict, _ := c.config.Get(config.ARConnectionConfigKeyStrictSsl).(bool)
		allow, _ := c.config.Get(config.ARConnectionConfigKeyAllowFallback).(bool)
		// Python behaviour (including for the login) unless the user
		// configured a CA pool or a pin: then never downgrade
		if !strict && allow && !c.pinned() {
			retry, ferr := c.transition(gen, func() (bool, int, error) {
				slog.Warn("Cannot verify SSL certificate. Since `STRICT_SSL` configuration is disabled, falling back to HTTP with a default port")
				c.mu.Lock()
				c.usedFallbacks[ConnectionFallbackHTTPS] = true
				c.mu.Unlock()
				return false, consts.DefaultPortHTTP, nil
			})
			if ferr != nil {
				return Response{}, ferr
			}
			if ferr := c.afterTransition(ctx, retry); ferr != nil {
				return Response{}, ferr
			}
			return c.sendRequest(ctx, ep, payload, headers, requestType)
		}
		return Response{}, arerror.NewSSLCertificateError(
			"SSL certificate verification failed. Your configuration requires a strict SSL certificate verification.")
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		if allow, _ := c.config.Get(config.ARConnectionConfigKeyAllowFallback).(bool); allow {
			retry, ferr := c.transition(gen, c.nextFallbackConfig)
			if ferr != nil {
				return Response{}, ferr
			}
			if ferr := c.afterTransition(ctx, retry); ferr != nil {
				return Response{}, ferr
			}
			return c.sendRequest(ctx, ep, payload, headers, requestType)
		}
		c.ResetAuth()
	}
	return Response{}, arerror.NewConnectionError(fmt.Sprintf(
		"Cannot connect to `%s` on port `%d`. Failed with error: `%v`",
		identifiers.NewHostname(c.hostname).LogValue().String(), c.Port(), redactHost(err, c.hostname)))
}

// afterTransition finishes our own fallback step, or - when another
// request made the step - waits for its re-login so the resend carries
// the new auth cookie (login-chain requests carry their own headers).
func (c *Connection) afterTransition(ctx context.Context, retry bool) error {
	if !retry {
		return c.finishFallback(ctx)
	}
	if inLoginContext(ctx) || c.Connected() {
		return nil
	}
	fctx, cancel := context.WithTimeout(ctx, consts.DefaultTimeoutFallback)
	defer cancel()
	if ok, err := c.login(fctx); !ok {
		if err != nil {
			return err
		}
		return arerror.NewConnectionError("Fallback reconnect failed")
	}
	return nil
}

// pinned reports whether the user configured a CA pool or a pin.
func (c *Connection) pinned() bool {
	return c.tlsOpts.RootCAs != nil || len(c.tlsOpts.PinnedCertSHA256) > 0
}

// transition applies one fallback step chosen by next, unless another
// request already changed the config since this request started (then
// it returns retry=true: just resend on the current config).
func (c *Connection) transition(gen int, next func() (bool, int, error)) (retry bool, err error) {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	if c.fallbackGen != gen {
		return true, nil
	}
	useSSL, port, err := next()
	if err != nil {
		return false, err
	}
	c.config.Set(config.ARConnectionConfigKeyUseSsl, useSSL)
	c.config.Set(config.ARConnectionConfigKeyPort, port)
	c.fallbackGen++
	return false, nil
}

func redactHost(err error, host string) string {
	return strings.ReplaceAll(err.Error(), host, identifiers.NewHostname(host).LogValue().String())
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// nextFallbackConfig mirrors `_next_fallback_config`.
//
// | Current             | Next            | Guard                   |
// | ------------------- | --------------- | ----------------------- |
// | HTTPS @ custom port | HTTPS @ default |                         |
// | HTTPS @ default     | HTTP @ default  | STRICT_SSL not set      |
// | HTTP @ custom port  | HTTP @ default  |                         |
// | HTTP @ default      | HTTPS @ default | ALLOW_UPGRADE_HTTP→HTTPS|
func (c *Connection) nextFallbackConfig() (bool, int, error) {
	useSSL, _ := c.config.Get(config.ARConnectionConfigKeyUseSsl).(bool)
	port := c.Port()
	c.mu.Lock()
	defer c.mu.Unlock()
	used := c.usedFallbacks

	if useSSL {
		if port != consts.DefaultPortHTTPS {
			if used[ConnectionFallbackHTTPS] {
				return false, 0, arerror.NewFallbackLoopError(fmt.Sprintf(
					"Fallback loop detected trying to heal HTTPS connection with set port `%d`", port))
			}
			slog.Warn("Cannot connect on the provided HTTPS port. Will fallback to the default port",
				"port", port, "default", consts.DefaultPortHTTPS)
			used[ConnectionFallbackHTTPS] = true
			return true, consts.DefaultPortHTTPS, nil
		}
		if strict, _ := c.config.Get(config.ARConnectionConfigKeyStrictSsl).(bool); strict {
			return false, 0, arerror.NewFallbackForbiddenError(
				"Fallback from HTTPS to HTTP connection is forbidden by the `STRICT_SSL` configuration option")
		}
		// Go only: a configured CA pool or pin means the user wants a
		// verified link; plaintext would send the credentials in clear
		if c.pinned() {
			return false, 0, arerror.NewFallbackForbiddenError(
				"Fallback from HTTPS to HTTP connection is forbidden when a CA pool or a certificate pin is configured")
		}
		if used[ConnectionFallbackHTTP] {
			return false, 0, arerror.NewFallbackLoopError("Fallback loop detected trying to heal HTTPS by switching to HTTP")
		}
		slog.Warn("Cannot connect on the default HTTPS port. Will fallback to the HTTP connection on default port",
			"https", consts.DefaultPortHTTPS, "http", consts.DefaultPortHTTP)
		used[ConnectionFallbackHTTP] = true
		return false, consts.DefaultPortHTTP, nil
	}

	if port != consts.DefaultPortHTTP {
		if used[ConnectionFallbackHTTP] {
			return false, 0, arerror.NewFallbackLoopError(fmt.Sprintf(
				"Fallback loop detected trying to heal HTTP connection with set port `%d`", port))
		}
		slog.Warn("Cannot connect on the custom HTTP port. Will try using the default HTTP port",
			"port", port, "default", consts.DefaultPortHTTP)
		used[ConnectionFallbackHTTP] = true
		return false, consts.DefaultPortHTTP, nil
	}

	if upgrade, _ := c.config.Get(config.ARConnectionConfigKeyAllowUpgradeHttpToHttps).(bool); !upgrade {
		return false, 0, arerror.NewFallbackError(
			"Cannot connect on HTTP — no further fallback options. Enable `ALLOW_UPGRADE_HTTP_TO_HTTPS` to allow HTTPS upgrade.")
	}
	if used[ConnectionFallbackHTTPS] {
		return false, 0, arerror.NewFallbackLoopError("Fallback loop detected trying to upgrade HTTP to HTTPS")
	}
	slog.Warn("Cannot connect on the default HTTP port. Will try upgrading to the default HTTPS port",
		"http", consts.DefaultPortHTTP, "https", consts.DefaultPortHTTPS)
	used[ConnectionFallbackHTTPS] = true
	return true, consts.DefaultPortHTTPS, nil
}

// finishFallback mirrors the tail of `_apply_fallback`: reset auth and,
// outside a login chain, reconnect with the short fallback timeout. Inside
// the login chain the caller simply resends the login on the new config.
func (c *Connection) finishFallback(ctx context.Context) error {
	c.ResetAuth()
	if inLoginContext(ctx) {
		// _login will retry _send_request on the new config — a separate
		// reconnect here would add a redundant login request.
		return nil
	}
	fctx, cancel := context.WithTimeout(ctx, consts.DefaultTimeoutFallback)
	defer cancel()
	if ok, _ := c.login(fctx); !ok {
		return arerror.NewConnectionError("Fallback reconnect failed")
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
