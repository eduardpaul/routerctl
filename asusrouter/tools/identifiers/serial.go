// Serial number tools (serial.py).

package identifiers

import (
	"encoding/hex"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Bytes of digest used to build a deterministic pseudo-serial
const serialMaskBytes = 4

// Serial mirrors `Serial`: a serial number representation.
type Serial struct {
	value string
}

// NewSerial mirrors `Serial(value)` (the value goes through `str()`).
func NewSerial(value any) Serial {
	return Serial{value: converters.PyStr(value)}
}

// SerialFromValue mirrors `Serial.from_value`.
func SerialFromValue(value any) Serial {
	switch v := value.(type) {
	case Serial:
		return v
	case *Serial:
		if v != nil {
			return *v
		}
	}
	return NewSerial(value)
}

// SerialFromValueSafe mirrors `Serial.from_value_safe`: nil for a missing or
// empty value.
func SerialFromValueSafe(value any) *Serial {
	if isNone(value) {
		return nil
	}
	v := SerialFromValue(value)
	if v.value == "" {
		return nil
	}
	return &v
}

// Value returns the raw serial value.
func (v Serial) Value() string { return v.value }

// RevealLevel returns REASONABLE.
func (v Serial) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelReasonable
}

// Maskable reports true.
func (v Serial) Maskable() bool { return true }

// Mask returns a deterministic pseudo-serial (a Serial).
func (v Serial) Mask() any {
	digest := security.HmacDigest([]byte(v.value), nil)
	return Serial{value: "sn-" + hex.EncodeToString(digest[:serialMaskBytes])}
}

// CanonicalValue returns the raw serial value.
func (v Serial) CanonicalValue() any { return v.value }

// String returns the raw serial value.
func (v Serial) String() string { return v.value }

// GoString returns the serial representation (Python repr).
func (v Serial) GoString() string { return "Serial(" + v.value + ")" }

// MarshalText returns the raw serial value.
func (v Serial) MarshalText() ([]byte, error) { return []byte(v.value), nil }

// LogValue renders the serial at the log security level.
func (v Serial) LogValue() slog.Value { return security.LogValue(v) }

// Equal mirrors `__eq__`: equal to a Serial or a string of the same value.
func (v Serial) Equal(other any) bool {
	switch o := other.(type) {
	case Serial:
		return v == o
	case *Serial:
		return o != nil && v == *o
	case string:
		return v.value == o
	}
	return false
}
