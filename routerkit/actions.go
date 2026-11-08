package routerkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ArgType is the type of an action argument (or of a driver option).
type ArgType string

// Argument types. Bind parses raw strings into these and canonicalizes
// them.
const (
	// ArgBool: on/off/true/false/1/0/yes/no/enable/disable (ParseOnOff);
	// canonical "on" / "off"
	ArgBool ArgType = "bool"
	// ArgEnum: one of Choices (case-insensitive); canonical: the choice
	ArgEnum ArgType = "enum"
	// ArgString: any text
	ArgString ArgType = "string"
	// ArgInt: a decimal integer
	ArgInt ArgType = "int"
	// ArgMAC: a MAC address, normalized with NormalizeMAC
	ArgMAC ArgType = "mac"
	// ArgIP: an IPv4 or IPv6 address
	ArgIP ArgType = "ip"
	// ArgDuration: a Go duration ("500ms", "5m")
	ArgDuration ArgType = "duration"
)

// Valid reports whether t is a known type.
func (t ArgType) Valid() bool {
	switch t {
	case ArgBool, ArgEnum, ArgString, ArgInt, ArgMAC, ArgIP, ArgDuration:
		return true
	}
	return false
}

// Arg describes one action argument.
type Arg struct {
	Name     string  `json:"name"`
	Help     string  `json:"help"`
	Type     ArgType `json:"type"`
	Required bool    `json:"required"`
	// Choices: the values of an ArgEnum (required for enums only)
	Choices []string `json:"choices,omitempty"`
	// Default is used when the argument is absent
	Default string `json:"default,omitempty"`
}

// Danger orders how disruptive an action is: the CLI asks for --yes at
// Destructive; automations (MQTT, the HTTP API) run actions up to the
// max_danger of their command policy (default Disruptive).
type Danger uint8

// Danger levels.
const (
	// Safe: LED, read-triggering actions (speedtest)
	Safe Danger = iota
	// Disruptive: drops some connectivity briefly (WiFi off, a service
	// restart, VPN, blocking a client)
	Disruptive
	// Destructive: reboot, factory reset, firmware update
	Destructive
)

var dangerNames = []string{"safe", "disruptive", "destructive"}

// String implements fmt.Stringer.
func (d Danger) String() string {
	if int(d) < len(dangerNames) {
		return dangerNames[d]
	}
	return fmt.Sprintf("danger(%d)", uint8(d))
}

// MarshalText implements encoding.TextMarshaler.
func (d Danger) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Danger) UnmarshalText(b []byte) error {
	for i, n := range dangerNames {
		if n == string(b) {
			*d = Danger(i)
			return nil
		}
	}
	return fmt.Errorf("unknown danger level %q", b)
}

// ActionSpec describes one action a device supports.
type ActionSpec struct {
	Name string `json:"name"` // "led", "reboot", "block-client", ...
	Help string `json:"help"`
	Args []Arg  `json:"args,omitempty"`
	// Standard: from the catalog (Std); the schema is fixed
	Standard bool   `json:"standard,omitempty"`
	Danger   Danger `json:"danger"`
	// Async: the outcome is read later (Result.Pending)
	Async bool `json:"async,omitempty"`
	// Toggle names the bool argument that makes the action a switch
	// (Home Assistant switch rather than button)
	Toggle string `json:"toggle,omitempty"`
}

// IsDangerous reports whether the action needs explicit confirmation
// (Danger >= Destructive).
func (s ActionSpec) IsDangerous() bool { return s.Danger >= Destructive }

// Arg returns the spec of a named argument.
func (s ActionSpec) Arg(name string) (Arg, bool) {
	for _, a := range s.Args {
		if a.Name == name {
			return a, true
		}
	}
	return Arg{}, false
}

// invalid returns an ErrInvalidArgs error.
func invalid(format string, a ...any) error {
	return Classify(fmt.Errorf(format, a...), ErrInvalidArgs)
}

// Bind validates raw (CLI / MQTT) arguments against the spec and returns
// typed, canonical values: required arguments present (or defaulted),
// types and choices respected, no unknown arguments. Errors are
// ErrInvalidArgs. raw is not modified.
func (s ActionSpec) Bind(raw map[string]string) (Args, error) {
	out := Args{vals: map[string]any{}, strs: map[string]string{}}
	for _, a := range s.Args {
		v, ok := raw[a.Name]
		if !ok || v == "" {
			if a.Default == "" {
				if a.Required {
					return Args{}, invalid("%s: missing argument %q", s.Name, a.Name)
				}
				continue
			}
			v = a.Default
		}
		val, str, err := parseArg(a, v)
		if err != nil {
			return Args{}, invalid("%s: %s", s.Name, err)
		}
		out.vals[a.Name], out.strs[a.Name] = val, str
	}
	var unknown []string
	for k := range raw {
		if _, ok := s.Arg(k); !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Args{}, invalid("%s: unknown argument(s) %s", s.Name, strings.Join(unknown, ", "))
	}
	return out, nil
}

// parseArg parses one value: the typed value and its canonical string.
func parseArg(a Arg, v string) (any, string, error) {
	switch a.Type {
	case ArgBool:
		b, err := ParseOnOff(v)
		if err != nil {
			return nil, "", fmt.Errorf("%s=%q is not on/off", a.Name, v)
		}
		if b {
			return true, "on", nil
		}
		return false, "off", nil
	case ArgEnum:
		for _, c := range a.Choices {
			if strings.EqualFold(c, v) {
				return c, c, nil
			}
		}
		return nil, "", fmt.Errorf("%s=%q is not one of %s", a.Name, v, strings.Join(a.Choices, "|"))
	case ArgInt:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, "", fmt.Errorf("%s=%q is not a number", a.Name, v)
		}
		return n, strconv.Itoa(n), nil
	case ArgMAC:
		m := NormalizeMAC(v)
		if m == "" {
			return nil, "", fmt.Errorf("%s=%q is not a MAC address", a.Name, v)
		}
		return m, m, nil
	case ArgIP:
		ip := net.ParseIP(strings.TrimSpace(v))
		if ip == nil {
			return nil, "", fmt.Errorf("%s=%q is not an IP address", a.Name, v)
		}
		return ip.String(), ip.String(), nil
	case ArgDuration:
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return nil, "", fmt.Errorf("%s=%q is not a duration", a.Name, v)
		}
		return d, d.String(), nil
	}
	return v, v, nil // ArgString
}

// Args are bound action arguments (see ActionSpec.Bind). The zero value
// holds no argument.
type Args struct {
	vals map[string]any
	strs map[string]string
}

// RawArgs wraps unvalidated strings (e.g. decoded from JSON); drivers
// bind them again (BindInvocation) before use.
func RawArgs(raw map[string]string) Args {
	a := Args{strs: make(map[string]string, len(raw))}
	for k, v := range raw {
		a.strs[k] = v
	}
	return a
}

// Has reports whether an argument is set.
func (a Args) Has(name string) bool {
	_, ok := a.strs[name]
	return ok
}

// Len is the number of arguments set.
func (a Args) Len() int { return len(a.strs) }

// Bool returns an ArgBool argument (false when absent).
func (a Args) Bool(name string) bool {
	b, _ := a.vals[name].(bool)
	return b
}

// String returns the canonical text of any argument ("" when absent).
func (a Args) String(name string) string { return a.strs[name] }

// Int returns an ArgInt argument (0 when absent).
func (a Args) Int(name string) int {
	n, _ := a.vals[name].(int)
	return n
}

// MAC returns an ArgMAC argument, normalized ("" when absent).
func (a Args) MAC(name string) string {
	m, _ := a.vals[name].(string)
	return m
}

// Duration returns an ArgDuration argument (0 when absent).
func (a Args) Duration(name string) time.Duration {
	d, _ := a.vals[name].(time.Duration)
	return d
}

// Strings returns the canonical text of every argument (a copy).
func (a Args) Strings() map[string]string {
	out := make(map[string]string, len(a.strs))
	for k, v := range a.strs {
		out[k] = v
	}
	return out
}

// MarshalJSON implements json.Marshaler (the canonical strings).
func (a Args) MarshalJSON() ([]byte, error) { return json.Marshal(a.Strings()) }

// UnmarshalJSON implements json.Unmarshaler (raw, unvalidated strings).
func (a *Args) UnmarshalJSON(b []byte) error {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*a = RawArgs(m)
	return nil
}

// Invocation is one action call.
type Invocation struct {
	Name string `json:"name"`
	Args Args   `json:"args"`
}

// Result is the outcome of an action.
type Result struct {
	// Accepted: the device took the action (an ASUS {"success": false}
	// answer is not accepted)
	Accepted bool `json:"accepted"`
	// Data is the brand's answer, JSON-able
	Data any `json:"data,omitempty"`
	// Pending: an async action; read a source for the outcome later
	Pending bool `json:"pending,omitempty"`
	// ExpectDisconnect: the device goes away for a while (reboot)
	ExpectDisconnect bool `json:"expect_disconnect,omitempty"`
}

// ErrRejected: the device answered but did not accept the action.
var ErrRejected = errors.New("the device did not accept the action")

// Err returns nil for an accepted result and ErrRejected otherwise.
func (r Result) Err(action string) error {
	if r.Accepted {
		return nil
	}
	return fmt.Errorf("%s: %w", action, ErrRejected)
}

// BindInvocation finds the spec of inv in specs and binds inv.Args to it
// again (an Invocation may come from JSON or from another spec). Drivers
// call it first in Do: an unknown action is ErrUnsupported, bad
// arguments are ErrInvalidArgs, and neither touches the device.
func BindInvocation(specs []ActionSpec, inv Invocation) (ActionSpec, Args, error) {
	spec, ok := FindAction(specs, inv.Name)
	if !ok {
		return ActionSpec{}, Args{}, fmt.Errorf("unknown action %q: %w", inv.Name, ErrUnsupported)
	}
	args, err := spec.Bind(inv.Args.strs)
	if err != nil {
		return ActionSpec{}, Args{}, err
	}
	return spec, args, nil
}

// Invoke binds raw arguments to a listed action: the spec and the
// Invocation to pass to Driver.Do.
func (c Capabilities) Invoke(name string, raw map[string]string) (ActionSpec, Invocation, error) {
	spec, ok := c.Action(name)
	if !ok {
		return ActionSpec{}, Invocation{}, fmt.Errorf("unknown action %q: %w", name, ErrUnsupported)
	}
	args, err := spec.Bind(raw)
	if err != nil {
		return ActionSpec{}, Invocation{}, err
	}
	return spec, Invocation{Name: name, Args: args}, nil
}
