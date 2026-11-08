// SSID tools (ssid.py).

package identifiers

import (
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Ssid mirrors `Ssid`: a WiFi SSID representation.
//
// An SSID is broadcast unless the network is hidden, so it is shown by
// default and only hidden under a strict security level.
type Ssid struct {
	value string
}

// NewSsid mirrors `Ssid(value)` (the value goes through `str()`).
func NewSsid(value any) Ssid {
	return Ssid{value: converters.PyStr(value)}
}

// SsidFromValue mirrors `Ssid.from_value`.
func SsidFromValue(value any) Ssid {
	switch v := value.(type) {
	case Ssid:
		return v
	case *Ssid:
		if v != nil {
			return *v
		}
	}
	return NewSsid(value)
}

// SsidFromValueSafe mirrors `Ssid.from_value_safe`: nil for a missing or
// empty value.
func SsidFromValueSafe(value any) *Ssid {
	if isNone(value) {
		return nil
	}
	v := SsidFromValue(value)
	if v.value == "" {
		return nil
	}
	return &v
}

// Value returns the raw SSID value.
func (v Ssid) Value() string { return v.value }

// RevealLevel returns DEFAULT.
func (v Ssid) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelDefault
}

// Maskable reports false.
func (v Ssid) Maskable() bool { return false }

// Mask returns the redaction marker (SSID cannot be masked).
func (v Ssid) Mask() any { return security.Redacted }

// CanonicalValue returns the raw SSID value.
func (v Ssid) CanonicalValue() any { return v.value }

// String returns the raw SSID value.
func (v Ssid) String() string { return v.value }

// GoString returns the SSID representation (Python repr).
func (v Ssid) GoString() string { return "Ssid(" + converters.PyRepr(v.value) + ")" }

// MarshalText returns the raw SSID value.
func (v Ssid) MarshalText() ([]byte, error) { return []byte(v.value), nil }

// LogValue renders the SSID at the log security level.
func (v Ssid) LogValue() slog.Value { return security.LogValue(v) }

// Equal mirrors `__eq__`: equal to a Ssid or a string of the same value.
func (v Ssid) Equal(other any) bool {
	switch o := other.(type) {
	case Ssid:
		return v == o
	case *Ssid:
		return o != nil && v == *o
	case string:
		return v.value == o
	}
	return false
}
