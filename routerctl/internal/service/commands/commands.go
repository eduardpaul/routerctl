// Package commands is the one command path of routerctl: every transport
// (CLI `do`, MQTT command topics, the HTTP API, and so Node-RED) parses
// arguments with the same grammar, applies the same danger policy, runs
// the action through a Target and reports the same Result.
package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
	"github.com/eduardpaul/routerctl/routerkit"
)

// ---- policy ----

// Policy decides which actions a transport may run.
type Policy struct {
	// MaxDanger is the most disruptive level allowed
	MaxDanger routerkit.Danger
	// Allow: when non-empty, only these actions
	Allow []string
	// Deny: never these actions
	Deny []string
}

// DefaultPolicy allows everything but destructive actions (reboot,
// factory reset, firmware update).
var DefaultPolicy = Policy{MaxDanger: routerkit.Disruptive}

// ParseDanger reads "safe" | "disruptive" | "destructive" ("" is def).
func ParseDanger(s string, def routerkit.Danger) (routerkit.Danger, error) {
	if strings.TrimSpace(s) == "" {
		return def, nil
	}
	var d routerkit.Danger
	if err := d.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(s)))); err != nil {
		return def, fmt.Errorf("%q is not safe|disruptive|destructive", s)
	}
	return d, nil
}

// Restrict returns the policy that allows only what both allow.
func (p Policy) Restrict(q Policy) Policy {
	out := Policy{MaxDanger: min(p.MaxDanger, q.MaxDanger)}
	out.Deny = append(append([]string(nil), p.Deny...), q.Deny...)
	switch {
	case len(p.Allow) == 0:
		out.Allow = append([]string(nil), q.Allow...)
	case len(q.Allow) == 0:
		out.Allow = append([]string(nil), p.Allow...)
	default:
		for _, a := range p.Allow {
			if has(q.Allow, a) {
				out.Allow = append(out.Allow, a)
			}
		}
		if len(out.Allow) == 0 {
			// Disjoint allowlists allow nothing; keep the list non-empty
			out.Allow = []string{"\x00none"}
		}
	}
	return out
}

// ErrForbidden: the policy does not allow the action.
var ErrForbidden = errors.New("not allowed by the command policy")

// Check returns nil when the policy allows the action, else an error
// wrapping ErrForbidden that says why.
func (p Policy) Check(spec routerkit.ActionSpec) error {
	switch {
	case has(p.Deny, spec.Name):
		return fmt.Errorf("%s is denied: %w", spec.Name, ErrForbidden)
	case len(p.Allow) > 0 && !has(p.Allow, spec.Name):
		return fmt.Errorf("%s is not in the allow list: %w", spec.Name, ErrForbidden)
	case spec.Danger > p.MaxDanger:
		return fmt.Errorf("%s is %s (allowed up to %s): %w", spec.Name, spec.Danger, p.MaxDanger, ErrForbidden)
	}
	return nil
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Action is an action spec with the policy verdict (what the HTTP API
// and the Node-RED editor list).
type Action struct {
	routerkit.ActionSpec
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Actions lists specs with the policy verdict.
func (p Policy) Actions(specs []routerkit.ActionSpec) []Action {
	out := make([]Action, 0, len(specs))
	for _, s := range specs {
		a := Action{ActionSpec: s, Allowed: true}
		if err := p.Check(s); err != nil {
			a.Allowed, a.Reason = false, strings.TrimSuffix(err.Error(), ": "+ErrForbidden.Error())
		}
		out = append(out, a)
	}
	return out
}

// ---- argument grammar ----

// IDKey is the reserved JSON payload key that carries a request id; it
// is removed before binding and echoed in the Result.
const IDKey = "_id"

// Parse reads a command payload: a JSON object, "k=v k2=v2" (also
// separated by & or newlines), a bare value (the toggle argument, else
// the only required one), or empty / "PRESS" (no arguments). It returns
// the raw arguments and the request id of a JSON payload.
func Parse(spec routerkit.ActionSpec, payload []byte) (map[string]string, string, error) {
	p := strings.TrimSpace(string(payload))
	switch {
	case p == "" || strings.EqualFold(p, "PRESS"):
		return map[string]string{}, "", nil
	case strings.HasPrefix(p, "{"):
		var raw map[string]any
		if err := json.Unmarshal([]byte(p), &raw); err != nil {
			return nil, "", invalid("invalid JSON payload: %v", err)
		}
		return FromJSON(raw)
	}
	args, err := ParseFields(spec, strings.FieldsFunc(p, func(r rune) bool { return r == ' ' || r == '&' || r == '\n' || r == '\t' }))
	return args, "", err
}

// FromJSON converts a decoded JSON object to raw arguments (strings,
// numbers and booleans; true/false become on/off) and its request id.
func FromJSON(raw map[string]any) (map[string]string, string, error) {
	args := map[string]string{}
	id := ""
	for k, v := range raw {
		if k == IDKey {
			id = fmt.Sprint(v)
			continue
		}
		switch t := v.(type) {
		case string:
			args[k] = t
		case bool:
			args[k] = map[bool]string{true: "on", false: "off"}[t]
		case float64:
			args[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case nil:
		default:
			return nil, "", invalid("argument %q must be a string, number or boolean", k)
		}
	}
	return args, id, nil
}

// ParseFields reads "k=v" fields; one bare field is the toggle argument,
// else the only required argument (the CLI `do` positional arguments).
func ParseFields(spec routerkit.ActionSpec, fields []string) (map[string]string, error) {
	args := map[string]string{}
	var bare []string
	for _, f := range fields {
		if f == "" {
			continue
		}
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			bare = append(bare, f)
			continue
		}
		if k == "" {
			return nil, invalid("invalid argument %q (want key=value)", f)
		}
		args[k] = v
	}
	switch len(bare) {
	case 0:
		return args, nil
	case 1:
		name := spec.Toggle
		if name == "" {
			var req []string
			for _, a := range spec.Args {
				if a.Required {
					req = append(req, a.Name)
				}
			}
			if len(req) == 1 {
				name = req[0]
			}
		}
		if name == "" {
			return nil, invalid("argument %q is not key=value", bare[0])
		}
		if _, dup := args[name]; dup {
			return nil, invalid("argument %s given twice", name)
		}
		args[name] = bare[0]
		return args, nil
	}
	return nil, invalid("arguments %s are not key=value", strings.Join(bare, " "))
}

func invalid(format string, a ...any) error {
	return routerkit.Classify(fmt.Errorf(format, a...), routerkit.ErrInvalidArgs)
}

// ---- running ----

// Target runs actions: the daemon (FromControl) or a driver the CLI
// opened itself (FromDriver).
type Target interface {
	// Actions returns the actions of a device (poller.ErrUnknownDevice,
	// poller.ErrNotReady)
	Actions(device string) ([]routerkit.ActionSpec, error)
	Do(ctx context.Context, device string, inv routerkit.Invocation) (routerkit.Result, error)
}

// FromControl adapts the daemon.
func FromControl(ctl poller.Control) Target { return controlTarget{ctl} }

type controlTarget struct{ ctl poller.Control }

func (c controlTarget) Actions(device string) ([]routerkit.ActionSpec, error) {
	st, ok := c.ctl.State(device)
	if !ok {
		return nil, fmt.Errorf("%q: %w", device, poller.ErrUnknownDevice)
	}
	if len(st.Capabilities.Actions) == 0 && len(st.Capabilities.Parts) == 0 {
		return nil, fmt.Errorf("%s: %w", device, poller.ErrNotReady)
	}
	return st.Capabilities.Actions, nil
}

func (c controlTarget) Do(ctx context.Context, device string, inv routerkit.Invocation) (routerkit.Result, error) {
	return c.ctl.Do(ctx, device, inv)
}

// FromDriver adapts one connected driver (the CLI without a daemon).
func FromDriver(name string, drv routerkit.Driver, caps routerkit.Capabilities) Target {
	return driverTarget{name, drv, caps}
}

type driverTarget struct {
	name string
	drv  routerkit.Driver
	caps routerkit.Capabilities
}

func (d driverTarget) Actions(device string) ([]routerkit.ActionSpec, error) {
	if device != d.name {
		return nil, fmt.Errorf("%q: %w", device, poller.ErrUnknownDevice)
	}
	return d.caps.Actions, nil
}

func (d driverTarget) Do(ctx context.Context, _ string, inv routerkit.Invocation) (routerkit.Result, error) {
	out, err := d.drv.Do(ctx, inv)
	if err == nil {
		err = out.Err(inv.Name)
	}
	return out, err
}

// Request is one command.
type Request struct {
	// ID is echoed in the result (a JSON payload's "_id" wins)
	ID     string
	Device string
	Action string
	// Payload is parsed with Parse unless Args is set
	Payload []byte
	Args    map[string]string
	// Source names the transport ("mqtt", "api", "cli")
	Source string
	// Timeout of the action (0 = 3 minutes)
	Timeout time.Duration
}

// Result is the outcome of a command, the same JSON on every transport.
type Result struct {
	ID     string            `json:"id,omitempty"`
	Device string            `json:"device"`
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
	OK     bool              `json:"ok"`
	// Pending: an async action (speedtest); read a source for the outcome
	Pending bool `json:"pending,omitempty"`
	// ExpectDisconnect: the device goes away for a while (reboot)
	ExpectDisconnect bool      `json:"expect_disconnect,omitempty"`
	Result           any       `json:"result,omitempty"`
	Error            *Error    `json:"error,omitempty"`
	Time             time.Time `json:"time"`
	// Danger of the action ("" when the action is unknown)
	Danger string `json:"danger,omitempty"`
}

// Error is a coded error.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Code classifies command (and HTTP API) errors.
type Code string

// Error codes.
const (
	CodeInvalidArgs   Code = "invalid_args"
	CodeForbidden     Code = "forbidden"
	CodeUnsupported   Code = "unsupported"
	CodeUnknownDevice Code = "unknown_device"
	CodeNotFound      Code = "not_found"
	CodeNotReady      Code = "not_ready"
	CodeRejected      Code = "rejected"
	CodeBusy          Code = "busy"
	CodeAuth          Code = "auth"
	CodeUnreachable   Code = "unreachable"
	CodeTimeout       Code = "timeout"
	CodeDevice        Code = "device"
)

// CodeOf classifies an error.
func CodeOf(err error) Code {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrForbidden):
		return CodeForbidden
	case errors.Is(err, routerkit.ErrInvalidArgs):
		return CodeInvalidArgs
	case errors.Is(err, poller.ErrUnknownDevice):
		return CodeUnknownDevice
	case errors.Is(err, poller.ErrNotReady):
		return CodeNotReady
	case errors.Is(err, routerkit.ErrRejected):
		return CodeRejected
	case errors.Is(err, routerkit.ErrNotFound):
		return CodeNotFound
	}
	switch routerkit.CodeOf(err) {
	case routerkit.CodeUnsupported:
		return CodeUnsupported
	case routerkit.CodeAuth:
		return CodeAuth
	case routerkit.CodeBusy:
		return CodeBusy
	case routerkit.CodeTimeout:
		return CodeTimeout
	case routerkit.CodeUnreachable:
		return CodeUnreachable
	}
	return CodeDevice
}

// NewError wraps err with its code (nil for nil).
func NewError(err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{Code: CodeOf(err), Message: err.Error()}
}

// Run parses, checks and runs one command. It never panics on bad input;
// every failure is in Result.Error.
func Run(ctx context.Context, t Target, pol Policy, req Request) Result {
	res := Result{ID: req.ID, Device: req.Device, Action: req.Action, Time: time.Now().UTC()}
	fail := func(err error) Result {
		res.Error = NewError(err)
		return res
	}
	specs, err := t.Actions(req.Device)
	if err != nil {
		return fail(err)
	}
	spec, ok := routerkit.FindAction(specs, req.Action)
	if !ok {
		return fail(fmt.Errorf("%s does not support action %q: %w", req.Device, req.Action, routerkit.ErrUnsupported))
	}
	res.Danger = spec.Danger.String()
	raw := req.Args
	var parseErr error
	if raw == nil {
		var id string
		raw, id, parseErr = Parse(spec, req.Payload)
		if id != "" {
			res.ID = id
		}
	}
	// The policy first: a refused action is refused whatever its arguments
	if err := pol.Check(spec); err != nil {
		return fail(err)
	}
	if parseErr != nil {
		return fail(parseErr)
	}
	bound, err := spec.Bind(raw)
	if err != nil {
		res.Args = raw
		return fail(err)
	}
	res.Args = bound.Strings()
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	ctx = poller.WithSource(ctx, req.Source)
	if res.ID != "" {
		ctx = poller.WithRequestID(ctx, res.ID)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := t.Do(ctx, req.Device, routerkit.Invocation{Name: req.Action, Args: bound})
	res.Time = time.Now().UTC()
	res.Result = out.Data
	if err != nil {
		return fail(err)
	}
	res.OK, res.Pending, res.ExpectDisconnect = true, out.Pending, out.ExpectDisconnect
	return res
}
