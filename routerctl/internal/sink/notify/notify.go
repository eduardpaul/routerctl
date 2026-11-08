// Package notify sends events to ntfy, Gotify or a generic JSON webhook.
// Each target has an event filter and Go text/template title / message
// templates (fields: .Type .Time .Device .Data and .Summary).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
)

// Default templates.
const (
	DefaultTitle   = "routerctl: {{ .Title }}"
	DefaultMessage = "{{ .Summary }}"
)

// View is what templates see.
type View struct {
	events.Event
	// Summary is Event.Summary (Describe() when the event has none)
	Summary string
	// Title is a short title for the event type ("New device", ...)
	Title string
}

var titles = map[events.Type]string{
	events.DeviceNew:       "New device",
	events.DeviceOnline:    "Device online",
	events.DeviceOffline:   "Device offline",
	events.DeviceIPChanged: "Device IP changed",
	events.WANDown:         "Internet down",
	events.WANUp:           "Internet up",
	events.WANIPChanged:    "Public IP changed",
	events.FirmwareUpdate:  "Firmware update",
	events.NodeOffline:     "Mesh node offline",
	events.NodeOnline:      "Mesh node online",
	events.ActionDone:      "Action",
}

// NewView builds the template view of an event.
func NewView(e events.Event) View {
	t := titles[e.Type]
	if t == "" {
		t = string(e.Type)
	}
	sum := e.Summary
	if sum == "" {
		sum = e.Describe()
	}
	return View{Event: e, Summary: sum, Title: t}
}

// Options configure one notification target.
type Options struct {
	Type    string // ntfy | gotify | webhook
	URL     string
	Topic   string // ntfy
	Token   string
	Events  []string // empty = every event
	Devices []string // empty = every device
	// Title / Message are text/template sources ("" = the defaults)
	Title    string
	Message  string
	Priority int
	Tags     []string          // ntfy
	Headers  map[string]string // webhook
	// Timeout of one delivery (0 = 10s)
	Timeout time.Duration
}

// String masks the token.
func (o Options) String() string {
	tok := ""
	if o.Token != "" {
		tok = "********"
	}
	return fmt.Sprintf("{Type:%s URL:%s Topic:%s Token:%s Events:%v Devices:%v Priority:%d Timeout:%s}",
		o.Type, o.URL, o.Topic, tok, o.Events, o.Devices, o.Priority, o.Timeout)
}

// GoString masks the token.
func (o Options) GoString() string { return "notify.Options" + o.String() }

// Target is one notification target.
type Target struct {
	cfg     Options
	sel     events.Selector
	title   *template.Template
	message *template.Template
	client  *http.Client
}

// Sink fans events out to targets.
type Sink struct {
	targets []*Target
}

var funcs = template.FuncMap{
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	"default": func(def string, v any) string {
		if v == nil {
			return def
		}
		if s := fmt.Sprint(v); s != "" {
			return s
		}
		return def
	},
}

// NewTarget validates a target config (templates must parse, event
// names must exist).
func NewTarget(c Options, client *http.Client) (*Target, error) {
	for _, e := range c.Events {
		if !events.ValidType(e) {
			return nil, fmt.Errorf("notify %s: unknown event %q", c.Type, e)
		}
	}
	t := &Target{cfg: c, sel: events.NewSelector(c.Events, c.Devices, nil, false), client: client}
	title, msg := c.Title, c.Message
	if title == "" {
		title = DefaultTitle
	}
	if msg == "" {
		msg = DefaultMessage
	}
	var err error
	if t.title, err = template.New("title").Funcs(funcs).Option("missingkey=zero").Parse(title); err != nil {
		return nil, fmt.Errorf("notify %s: title template: %w", c.Type, err)
	}
	if t.message, err = template.New("message").Funcs(funcs).Option("missingkey=zero").Parse(msg); err != nil {
		return nil, fmt.Errorf("notify %s: message template: %w", c.Type, err)
	}
	if t.client == nil {
		timeout := c.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		t.client = &http.Client{Timeout: timeout}
	}
	return t, nil
}

// New builds the sink from target options.
func New(cfgs []Options, client *http.Client) (*Sink, error) {
	s := &Sink{}
	var errs []error
	for _, c := range cfgs {
		t, err := NewTarget(c, client)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		s.targets = append(s.targets, t)
	}
	return s, errors.Join(errs...)
}

// Name implements poller.Sink.
func (s *Sink) Name() string { return "notify" }

// Start implements poller.Sink.
func (s *Sink) Start(context.Context, poller.Control) error { return nil }

// State implements poller.Sink (notifications only react to events).
func (s *Sink) State(poller.DeviceState) {}

// Close implements poller.Sink.
func (s *Sink) Close(context.Context) error { return nil }

// Event implements poller.Sink.
func (s *Sink) Event(e events.Event) {
	for _, t := range s.targets {
		if !t.Match(e) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), t.client.Timeout+time.Second)
		err := t.Send(ctx, e)
		cancel()
		if err != nil {
			slog.Warn("notification failed", "target", t.cfg.Type, "event", string(e.Type), "err", err)
		}
	}
}

// Match reports whether the target wants an event.
func (t *Target) Match(e events.Event) bool {
	return t.sel.Match(e)
}

// Render renders title and message for an event.
func (t *Target) Render(e events.Event) (title, message string, err error) {
	v := NewView(e)
	var b bytes.Buffer
	if err = t.title.Execute(&b, v); err != nil {
		return "", "", err
	}
	title = strings.TrimSpace(b.String())
	b.Reset()
	if err = t.message.Execute(&b, v); err != nil {
		return "", "", err
	}
	return title, strings.TrimSpace(b.String()), nil
}

// Send delivers one event (one retry on network errors / 5xx).
func (t *Target) Send(ctx context.Context, e events.Event) error {
	title, msg, err := t.Render(e)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := t.request(ctx, e, title, msg)
		if err != nil {
			return err
		}
		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = redact(err)
		} else {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			if resp.StatusCode < 500 {
				return lastErr
			}
		}
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(500 * time.Millisecond):
		}
	}
	return lastErr
}

// redact strips the URL (which may carry a token) from a client error.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

func (t *Target) request(ctx context.Context, e events.Event, title, msg string) (*http.Request, error) {
	c := t.cfg
	switch c.Type {
	case "ntfy":
		u := strings.TrimRight(c.URL, "/")
		if c.Topic != "" {
			u += "/" + url.PathEscape(c.Topic)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(msg))
		if err != nil {
			return nil, errors.New("ntfy: invalid url")
		}
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		req.Header.Set("Title", headerSafe(title))
		if c.Priority > 0 {
			req.Header.Set("Priority", strconv.Itoa(c.Priority))
		}
		tags := append([]string{}, c.Tags...)
		tags = append(tags, string(e.Type))
		req.Header.Set("Tags", strings.Join(tags, ","))
		if tok := c.Token; tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		return req, nil
	case "gotify":
		body := map[string]any{"title": title, "message": msg, "extras": map[string]any{"routerctl": map[string]any{"event": e}}}
		if c.Priority > 0 {
			body["priority"] = c.Priority
		}
		data, _ := json.Marshal(body)
		u := strings.TrimRight(c.URL, "/") + "/message"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
		if err != nil {
			return nil, errors.New("gotify: invalid url")
		}
		req.Header.Set("Content-Type", "application/json")
		// Header, not the ?token= query, so it never shows up in logs
		req.Header.Set("X-Gotify-Key", c.Token)
		return req, nil
	default: // webhook
		body := struct {
			events.Event
			Title   string `json:"title"`
			Message string `json:"message"`
		}{e, title, msg}
		data, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(data))
		if err != nil {
			return nil, errors.New("webhook: invalid url")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "routerctl")
		for k, v := range c.Headers {
			req.Header.Set(k, v)
		}
		if tok := c.Token; tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		return req, nil
	}
}

// headerSafe drops control characters and RFC 2047-encodes non-ASCII
// text (ntfy decodes it) so the value is a valid HTTP header.
func headerSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return mime.BEncoding.Encode("utf-8", s)
		}
	}
	return s
}
