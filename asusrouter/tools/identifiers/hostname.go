// Hostname tools (hostname.py).

package identifiers

import (
	"encoding/hex"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Bytes of digest used to build a deterministic pseudo-hostname
const hostnameMaskBytes = 4

// Hostname mirrors `Hostname`: a hostname representation (IP address or
// DNS name).
//
// A hostname identifies the user's device, so it is exposed raw only at
// the reasonable level and can be replaced by a deterministic pseudo-host
// when sanitized.
type Hostname struct {
	value string
}

// NewHostname mirrors `Hostname(value)` (the value goes through `str()`).
func NewHostname(value any) Hostname {
	return Hostname{value: converters.PyStr(value)}
}

// HostnameFromValue mirrors `Hostname.from_value`.
func HostnameFromValue(value any) Hostname {
	switch v := value.(type) {
	case Hostname:
		return v
	case *Hostname:
		if v != nil {
			return *v
		}
	}
	return NewHostname(value)
}

// HostnameFromValueSafe mirrors `Hostname.from_value_safe`: nil for a
// missing or empty value.
func HostnameFromValueSafe(value any) *Hostname {
	if isNone(value) {
		return nil
	}
	h := HostnameFromValue(value)
	if h.value == "" {
		return nil
	}
	return &h
}

// Value returns the raw hostname value.
func (h Hostname) Value() string { return h.value }

// RevealLevel returns REASONABLE.
func (h Hostname) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelReasonable
}

// Maskable reports true.
func (h Hostname) Maskable() bool { return true }

// Mask returns a deterministic pseudo-hostname (a Hostname).
func (h Hostname) Mask() any {
	digest := security.HmacDigest([]byte(h.value), nil)
	return Hostname{value: "host-" + hex.EncodeToString(digest[:hostnameMaskBytes])}
}

// CanonicalValue returns the raw hostname value.
func (h Hostname) CanonicalValue() any { return h.value }

// String returns the raw hostname value.
func (h Hostname) String() string { return h.value }

// GoString returns the hostname representation (Python repr).
func (h Hostname) GoString() string { return "Hostname(" + converters.PyRepr(h.value) + ")" }

// MarshalText returns the raw hostname value.
func (h Hostname) MarshalText() ([]byte, error) { return []byte(h.value), nil }

// LogValue renders the hostname at the log security level.
func (h Hostname) LogValue() slog.Value { return security.LogValue(h) }

// Equal mirrors `__eq__`: equal to a Hostname or a string of the same
// value.
func (h Hostname) Equal(other any) bool {
	switch o := other.(type) {
	case Hostname:
		return h == o
	case *Hostname:
		return o != nil && h == *o
	case string:
		return h.value == o
	}
	return false
}
