// Username tools (username.py).

package identifiers

import (
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Username mirrors `Username`: a login name representation (Python's string form is
// redacted, see PyStr; String returns the raw value).
type Username struct {
	value string
}

// NewUsername mirrors `Username(value)` (the value goes through `str()`).
func NewUsername(value any) Username {
	return Username{value: converters.PyStr(value)}
}

// UsernameFromValue mirrors `Username.from_value`.
func UsernameFromValue(value any) Username {
	switch v := value.(type) {
	case Username:
		return v
	case *Username:
		if v != nil {
			return *v
		}
	}
	return NewUsername(value)
}

// UsernameFromValueSafe mirrors `Username.from_value_safe`: nil for a missing or
// empty value.
func UsernameFromValueSafe(value any) *Username {
	if isNone(value) {
		return nil
	}
	v := UsernameFromValue(value)
	if v.value == "" {
		return nil
	}
	return &v
}

// Value returns the raw username value.
func (v Username) Value() string { return v.value }

// RevealLevel returns UNSAFE.
func (v Username) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelUnsafe
}

// Maskable reports false.
func (v Username) Maskable() bool { return false }

// Mask returns the redaction marker (username cannot be masked).
func (v Username) Mask() any { return security.Redacted }

// CanonicalValue returns the raw username value.
func (v Username) CanonicalValue() any { return v.value }

// String returns the raw username value (Python `str()` redacts: see PyStr).
func (v Username) String() string { return v.value }

// PyStr returns the redacted representation (Python `__str__`).
func (v Username) PyStr() string { return security.RedactedStr }

// GoString returns the username representation (Python repr).
func (v Username) GoString() string { return "Username(" + security.RedactedStr + ")" }

// MarshalText returns the raw username value.
func (v Username) MarshalText() ([]byte, error) { return []byte(v.value), nil }

// LogValue renders the username at the log security level.
func (v Username) LogValue() slog.Value { return security.LogValue(v) }

// Equal mirrors `__eq__`: equal to a Username or a string of the same value.
func (v Username) Equal(other any) bool {
	switch o := other.(type) {
	case Username:
		return v == o
	case *Username:
		return o != nil && v == *o
	case string:
		return v.value == o
	}
	return false
}
