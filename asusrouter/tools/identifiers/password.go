// Password tools (password.py).

package identifiers

import (
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Password mirrors `Password`: a password representation.
//
// A password is a secret: its raw value is exposed only at the unsafe
// security level, and Python's string form is always redacted (see
// PyStr; String returns the raw value).
type Password struct {
	value string
}

// NewPassword mirrors `Password(value)` (the value goes through `str()`).
func NewPassword(value any) Password {
	return Password{value: converters.PyStr(value)}
}

// PasswordFromValue mirrors `Password.from_value`.
func PasswordFromValue(value any) Password {
	switch v := value.(type) {
	case Password:
		return v
	case *Password:
		if v != nil {
			return *v
		}
	}
	return NewPassword(value)
}

// PasswordFromValueSafe mirrors `Password.from_value_safe`: nil for a missing or
// empty value.
func PasswordFromValueSafe(value any) *Password {
	if isNone(value) {
		return nil
	}
	v := PasswordFromValue(value)
	if v.value == "" {
		return nil
	}
	return &v
}

// Value returns the raw password value.
func (v Password) Value() string { return v.value }

// RevealLevel returns UNSAFE.
func (v Password) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelUnsafe
}

// Maskable reports false.
func (v Password) Maskable() bool { return false }

// Mask returns the redaction marker (password cannot be masked).
func (v Password) Mask() any { return security.Redacted }

// CanonicalValue returns the raw password value.
func (v Password) CanonicalValue() any { return v.value }

// String returns the raw password value (Python `str()` redacts: see PyStr).
func (v Password) String() string { return v.value }

// PyStr returns the redacted representation (Python `__str__`).
func (v Password) PyStr() string { return security.RedactedStr }

// GoString returns the password representation (Python repr).
func (v Password) GoString() string { return "Password(" + security.RedactedStr + ")" }

// MarshalText returns the raw password value.
func (v Password) MarshalText() ([]byte, error) { return []byte(v.value), nil }

// LogValue renders the password at the log security level.
func (v Password) LogValue() slog.Value { return security.LogValue(v) }

// Equal mirrors `__eq__`: equal to a Password or a string of the same value.
func (v Password) Equal(other any) bool {
	switch o := other.(type) {
	case Password:
		return v == o
	case *Password:
		return o != nil && v == *o
	case string:
		return v.value == o
	}
	return false
}
