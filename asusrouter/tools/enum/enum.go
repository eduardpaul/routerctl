// Package enum ports `asusrouter/tools/enum`: the `FromIntMixin` and
// `FromStrMixin` resolution rules as generic helpers.
//
// An enum package calls them from its `<Enum>FromValue`:
//
//	func ARFooFromValue(value any) ARFoo {
//		return enum.FromStr(value, ARFooMembers, arFooByName, ARFooUnknown)
//	}
//
// `names` maps the Python member names (upper case, aliases included) to
// the members, standing in for `getattr(cls, name)`.
package enum

import (
	"fmt"
	"slices"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ErrNoUnknown mirrors the ValueError raised by `_unknown_or_raise`.
type ErrNoUnknown struct {
	Enum  string
	Value any
}

func (e *ErrNoUnknown) Error() string {
	return fmt.Sprintf(
		"%s.from_value: cannot resolve %s and no `UNKNOWN` member is defined",
		e.Enum, converters.PyRepr(e.Value),
	)
}

// FromIntOK mirrors `FromIntMixin.from_value` without the UNKNOWN
// fallback: ok is false when the value resolves to no member.
func FromIntOK[T ~int](value any, members []T, names map[string]T) (T, bool) {
	// If already an enum member, return it
	switch v := value.(type) {
	case T:
		if slices.Contains(members, v) {
			return v, true
		}
	case *T:
		if v != nil && slices.Contains(members, *v) {
			return *v, true
		}
	}

	// Try integer conversion first
	if vint := converters.RawToInt(value); vint != nil {
		if m := T(*vint); slices.Contains(members, m) {
			return m, true
		}
	}

	// Try key names
	if vstr := converters.RawToStr(value); vstr != nil {
		if m, ok := names[strings.ToUpper(*vstr)]; ok {
			return m, true
		}
	}

	var zero T
	return zero, false
}

// FromInt mirrors `FromIntMixin.from_value`: unresolved values give
// `unknown`.
func FromInt[T ~int](value any, members []T, names map[string]T, unknown T) T {
	if m, ok := FromIntOK(value, members, names); ok {
		return m
	}
	// Fallback to UNKNOWN member
	return unknown
}

// FromStrOK mirrors `FromStrMixin.from_value` without the UNKNOWN
// fallback: ok is false when the value resolves to no member.
func FromStrOK[T ~string](value any, members []T, names map[string]T) (T, bool) {
	// If already an enum member, return it
	switch v := value.(type) {
	case T:
		if slices.Contains(members, v) {
			return v, true
		}
	case *T:
		if v != nil && slices.Contains(members, *v) {
			return *v, true
		}
	}

	if vstr := converters.RawToStr(value); vstr != nil {
		// Check whether this is a value
		if m := T(*vstr); slices.Contains(members, m) {
			return m, true
		}
		// Check if a key exists
		if m, ok := names[strings.ToUpper(*vstr)]; ok {
			return m, true
		}
	}

	var zero T
	return zero, false
}

// FromStr mirrors `FromStrMixin.from_value`: unresolved values give
// `unknown`.
func FromStr[T ~string](value any, members []T, names map[string]T, unknown T) T {
	if m, ok := FromStrOK(value, members, names); ok {
		return m
	}
	// Fallback to UNKNOWN member
	return unknown
}
