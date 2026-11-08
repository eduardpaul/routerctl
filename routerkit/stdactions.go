package routerkit

import (
	"errors"
	"fmt"
	"slices"
)

// Standard action names. A driver that offers one of these uses Std, so
// the name, the arguments and the danger are the same on every brand
// (CLI flags, MQTT topics and Home Assistant entities do not depend on
// the brand). Brand-only actions use any other name.
const (
	ActReboot       = "reboot"
	ActLED          = "led"
	ActWiFi         = "wifi"
	ActGuestWiFi    = "guest-wifi"
	ActBlockClient  = "block-client"
	ActBlockAll     = "block-all"
	ActKickClient   = "kick-client"
	ActReconnectWAN = "reconnect-wan"
	ActSpeedtest    = "speedtest"
	ActVPN          = "vpn"
)

func stateArg(help string) Arg {
	return Arg{Name: "state", Help: help, Type: ArgBool, Required: true}
}

func macArg(help string) Arg {
	return Arg{Name: "mac", Help: help, Type: ArgMAC, Required: true}
}

// catalog is the standard action catalog. Enum arguments have no choices
// here: the driver supplies them (WithChoices).
var catalog = []ActionSpec{
	{Name: ActBlockAll, Help: "block internet for every client (kill switch)", Args: []Arg{stateArg("on blocks, off unblocks")}, Danger: Disruptive, Toggle: "state"},
	{Name: ActBlockClient, Help: "block / unblock a client's internet access", Args: []Arg{macArg("client MAC address"), stateArg("on blocks, off unblocks")}, Danger: Disruptive, Toggle: "state"},
	{Name: ActGuestWiFi, Help: "turn a guest WiFi network on or off", Args: []Arg{{Name: "band", Help: "band", Type: ArgEnum, Required: true}, stateArg("on or off")}, Danger: Disruptive, Toggle: "state"},
	{Name: ActKickClient, Help: "disconnect a client", Args: []Arg{macArg("client MAC address")}, Danger: Disruptive},
	{Name: ActLED, Help: "turn the LEDs on or off", Args: []Arg{stateArg("on or off")}, Danger: Safe, Toggle: "state"},
	{Name: ActReboot, Help: "reboot the device", Danger: Destructive},
	{Name: ActReconnectWAN, Help: "reconnect the internet uplink", Danger: Disruptive},
	{Name: ActSpeedtest, Help: "run a speedtest", Danger: Safe, Async: true},
	{Name: ActVPN, Help: "turn a VPN client on or off", Args: []Arg{{Name: "protocol", Help: "VPN protocol", Type: ArgEnum, Required: true}, stateArg("on or off")}, Danger: Disruptive, Toggle: "state"},
	{Name: ActWiFi, Help: "turn a WiFi band on or off", Args: []Arg{{Name: "band", Help: "band", Type: ArgEnum, Required: true}, stateArg("on or off")}, Danger: Disruptive, Toggle: "state"},
}

// Catalog returns the standard actions (copies).
func Catalog() []ActionSpec {
	out := make([]ActionSpec, len(catalog))
	for i, s := range catalog {
		out[i] = cloneSpec(s)
	}
	return out
}

// StdSpec returns the catalog entry of a standard action.
func StdSpec(name string) (ActionSpec, bool) {
	s, ok := FindAction(catalog, name)
	if !ok {
		return ActionSpec{}, false
	}
	return cloneSpec(s), true
}

func cloneSpec(s ActionSpec) ActionSpec {
	s.Args = slices.Clone(s.Args)
	for i := range s.Args {
		s.Args[i].Choices = slices.Clone(s.Args[i].Choices)
	}
	return s
}

// StdOption adapts a standard action to a device (narrower choices, a
// brand help text, extra optional arguments).
type StdOption func(*ActionSpec)

// WithChoices sets the choices of an enum argument (a subset of what the
// action could take: the bands or protocols of this device).
func WithChoices(arg string, choices ...string) StdOption {
	return func(s *ActionSpec) {
		for i := range s.Args {
			if s.Args[i].Name == arg {
				s.Args[i].Choices = slices.Clone(choices)
			}
		}
	}
}

// WithHelp replaces the action help text.
func WithHelp(help string) StdOption { return func(s *ActionSpec) { s.Help = help } }

// WithArg appends an optional brand-specific argument (it is made
// optional: a standard action never requires more than the catalog).
func WithArg(a Arg) StdOption {
	return func(s *ActionSpec) {
		a.Required = false
		s.Args = append(s.Args, a)
	}
}

// Std returns the standard action name adapted with opts. It panics on a
// name outside the catalog (a programming error).
func Std(name string, opts ...StdOption) ActionSpec {
	s, ok := StdSpec(name)
	if !ok {
		panic(fmt.Sprintf("routerkit.Std: %q is not a standard action", name))
	}
	for _, o := range opts {
		o(&s)
	}
	s.Standard = true
	return s
}

// CheckSpec checks one action spec: a valid name, valid and unique
// arguments (choices on enums, and only there), a Toggle naming a bool
// argument;
// a standard action matches the catalog (only narrower choices, the help
// texts and extra optional arguments may differ) and a non-standard one
// does not reuse a catalog name.
func CheckSpec(s ActionSpec) error {
	var errs []error
	if !ValidName(s.Name) {
		errs = append(errs, fmt.Errorf("invalid action name %q", s.Name))
	}
	if s.Danger > Destructive {
		errs = append(errs, fmt.Errorf("unknown danger %d", s.Danger))
	}
	seen := map[string]bool{}
	for _, a := range s.Args {
		if a.Name == "" || seen[a.Name] {
			errs = append(errs, fmt.Errorf("argument %q: empty or duplicate name", a.Name))
		}
		seen[a.Name] = true
		switch {
		case !a.Type.Valid():
			errs = append(errs, fmt.Errorf("argument %s: invalid type %q", a.Name, a.Type))
		case a.Type == ArgEnum && len(a.Choices) == 0:
			errs = append(errs, fmt.Errorf("argument %s: enum without choices", a.Name))
		case a.Type != ArgEnum && len(a.Choices) > 0:
			errs = append(errs, fmt.Errorf("argument %s: choices on a %s argument", a.Name, a.Type))
		}
		if a.Default != "" {
			if _, _, err := parseArg(a, a.Default); err != nil {
				errs = append(errs, fmt.Errorf("argument %s: default: %w", a.Name, err))
			}
		}
	}
	if s.Toggle != "" {
		if a, ok := s.Arg(s.Toggle); !ok || a.Type != ArgBool {
			errs = append(errs, fmt.Errorf("toggle %q is not a bool argument", s.Toggle))
		}
	}
	std, isStd := FindAction(catalog, s.Name)
	switch {
	case s.Standard && !isStd:
		errs = append(errs, fmt.Errorf("marked standard but not in the catalog"))
	case !s.Standard && isStd:
		errs = append(errs, fmt.Errorf("reuses the standard name %q: build it with Std", s.Name))
	case isStd:
		errs = append(errs, checkStd(s, std)...)
	}
	if len(errs) > 0 {
		return fmt.Errorf("action %s: %w", s.Name, errors.Join(errs...))
	}
	return nil
}

func checkStd(s, std ActionSpec) []error {
	var errs []error
	if s.Danger != std.Danger || s.Toggle != std.Toggle || s.Async != std.Async {
		errs = append(errs, fmt.Errorf("danger/toggle/async differ from the catalog"))
	}
	if len(s.Args) < len(std.Args) {
		return append(errs, fmt.Errorf("arguments differ from the catalog"))
	}
	for i, want := range std.Args {
		got := s.Args[i]
		if got.Name != want.Name || got.Type != want.Type || got.Required != want.Required || got.Default != want.Default {
			errs = append(errs, fmt.Errorf("argument %d (%s) differs from the catalog", i, got.Name))
			continue
		}
	}
	for _, extra := range s.Args[len(std.Args):] {
		if extra.Required {
			errs = append(errs, fmt.Errorf("extra argument %s must be optional", extra.Name))
		}
	}
	return errs
}
