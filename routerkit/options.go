package routerkit

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// OptionSpec describes one brand-specific driver option (the `options:`
// map of a configured device).
type OptionSpec struct {
	Name string  `json:"name"`
	Help string  `json:"help"`
	Type ArgType `json:"type"`
	// Choices: the values of an ArgEnum option
	Choices []string `json:"choices,omitempty"`
	// Default applies when the option is absent
	Default string `json:"default,omitempty"`
}

func (o OptionSpec) arg() Arg {
	return Arg{Name: o.Name, Help: o.Help, Type: o.Type, Choices: o.Choices, Default: o.Default}
}

// BindOptions validates driver options against their specs and returns
// the typed values (defaults applied). Unknown options and values that
// do not parse are errors.
func BindOptions(specs []OptionSpec, opts map[string]string) (Args, error) {
	out := Args{vals: map[string]any{}, strs: map[string]string{}}
	known := make(map[string]bool, len(specs))
	var errs []error
	for _, o := range specs {
		known[o.Name] = true
		v, ok := opts[o.Name]
		if !ok || v == "" {
			if o.Default == "" {
				continue
			}
			v = o.Default
		}
		val, str, err := parseArg(o.arg(), v)
		if err != nil {
			errs = append(errs, fmt.Errorf("option %s", err))
			continue
		}
		out.vals[o.Name], out.strs[o.Name] = val, str
	}
	var unknown []string
	for k := range opts {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		names := make([]string, 0, len(specs))
		for _, o := range specs {
			names = append(names, o.Name)
		}
		accepted := "none"
		if len(names) > 0 {
			accepted = strings.Join(names, ", ")
		}
		errs = append(errs, fmt.Errorf("unknown option(s) %s (accepted: %s)", strings.Join(unknown, ", "), accepted))
	}
	if err := errors.Join(errs...); err != nil {
		return Args{}, err
	}
	return out, nil
}

// CheckOptionSpecs checks a factory's option specs: unique names and
// valid types (enum options with choices, defaults that parse).
func CheckOptionSpecs(specs []OptionSpec) error {
	var errs []error
	seen := map[string]bool{}
	for _, o := range specs {
		if o.Name == "" || seen[o.Name] {
			errs = append(errs, fmt.Errorf("option %q: empty or duplicate name", o.Name))
		}
		seen[o.Name] = true
		if !o.Type.Valid() {
			errs = append(errs, fmt.Errorf("option %s: invalid type %q", o.Name, o.Type))
		}
		if o.Type == ArgEnum && len(o.Choices) == 0 {
			errs = append(errs, fmt.Errorf("option %s: enum without choices", o.Name))
		}
		if o.Default != "" && o.Type.Valid() {
			if _, _, err := parseArg(o.arg(), o.Default); err != nil {
				errs = append(errs, fmt.Errorf("option %s: default: %w", o.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}
