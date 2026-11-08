// MAC address tools (mac.py).

package identifiers

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// MacCleanRe mirrors `MAC_CLEAN_RE`.
var MacCleanRe = regexp.MustCompile(`[^0-9a-fA-F]`)

const (
	MacLengthBytes = 6
	MacLengthStr   = 12
)

const (
	ErrorMacByte            = "MAC address must be exactly 6 bytes"
	ErrorMacInt             = "MAC address must be a 48-bit integer"
	ErrorMacStr             = "Invalid MAC address string"
	ErrorMacUnsupportedType = "Unsupported MAC address type"
)

// MacAddress mirrors `MacAddress`: a MAC address representation.
//
// It is comparable (usable as a map key); the zero value is
// 00:00:00:00:00:00.
type MacAddress struct {
	bytes [6]byte
}

// NewMacAddress mirrors `MacAddress(mac)`.
func NewMacAddress(mac any) (MacAddress, error) {
	b, err := macToBytes(mac)
	if err != nil {
		return MacAddress{}, err
	}
	return MacAddress{bytes: b}, nil
}

// macToBytes mirrors `_to_bytes`: convert supported values to the 6-byte
// canonical MAC representation.
//
// Supported inputs:
//   - MacAddress (or pointer) -> returns underlying bytes
//   - []byte / net.HardwareAddr (length 6), [6]byte
//   - int in [0, 2**48) (anything `RawToInt` reads, like Python)
//   - str with separators or plain hex
func macToBytes(value any) ([6]byte, error) {
	var out [6]byte
	if p, ok := value.(*string); ok && p != nil {
		value = *p
	}
	switch v := value.(type) {
	// Already a MacAddress instance
	case MacAddress:
		return v.bytes, nil
	case *MacAddress:
		if v != nil {
			return v.bytes, nil
		}
	// Bytes-like
	case [6]byte:
		return v, nil
	case net.HardwareAddr:
		return macFromSlice(v)
	case []byte:
		return macFromSlice(v)
	}

	// A separated MAC is never an integer, fast path
	if s, ok := value.(string); ok && strings.ContainsAny(s, ":-") {
		return macFromStr(s)
	}

	// Integer-compatible
	if vint := converters.RawToInt(value); vint != nil {
		if *vint < 0 || int64(*vint) >= 1<<48 {
			return out, errors.New(ErrorMacInt)
		}
		n := uint64(*vint)
		for i := 5; i >= 0; i-- {
			out[i] = byte(n)
			n >>= 8
		}
		return out, nil
	}

	// String-compatible
	if s, ok := value.(string); ok {
		return macFromStr(s)
	}

	return out, fmt.Errorf("%s: %s", ErrorMacUnsupportedType, pyType(value))
}

func macFromSlice(b []byte) ([6]byte, error) {
	var out [6]byte
	if len(b) != MacLengthBytes {
		return out, errors.New(ErrorMacByte)
	}
	copy(out[:], b)
	return out, nil
}

// macFromStr mirrors `_from_str`: convert a hex MAC string to its
// canonical bytes.
func macFromStr(value string) ([6]byte, error) {
	var out [6]byte
	cleaned := MacCleanRe.ReplaceAllString(value, "")
	if len(cleaned) != MacLengthStr {
		return out, errors.New(ErrorMacStr)
	}
	b, err := hex.DecodeString(cleaned)
	if err != nil {
		return out, errors.New(ErrorMacStr)
	}
	copy(out[:], b)
	return out, nil
}

// MacAddressFromValue mirrors `MacAddress.from_value`.
func MacAddressFromValue(value any) (MacAddress, error) {
	return NewMacAddress(value)
}

// MacAddressFromValueSafe mirrors `MacAddress.from_value_safe`: nil when
// the value cannot be parsed as a MAC address.
func MacAddressFromValueSafe(value any) *MacAddress {
	mac, err := MacAddressFromValue(value)
	if err != nil {
		return nil
	}
	return &mac
}

// AsAsus returns the MAC address in ASUS format (XX:XX:XX:XX:XX:XX).
func (m MacAddress) AsAsus() string {
	return strings.ToUpper(m.String())
}

// ToBytes returns the MAC address as bytes.
func (m MacAddress) ToBytes() []byte {
	return append([]byte{}, m.bytes[:]...)
}

// ToInt returns the MAC address as a 48-bit integer.
func (m MacAddress) ToInt() int64 {
	var n int64
	for _, b := range m.bytes {
		n = n<<8 | int64(b)
	}
	return n
}

// SetLocallyAdministered transforms the MAC address to a
// locally-administered address.
//
// For this, the locally-administered bit (bit 1) is set to 1 and the
// multicast bit (LSB) is cleared (set to 0).
func (m *MacAddress) SetLocallyAdministered() {
	m.bytes[0] = (m.bytes[0] &^ 0x01) | 0x02
}

// RevealLevel returns REASONABLE.
func (m MacAddress) RevealLevel() security.ARSecurityLevel {
	return security.ARSecurityLevelReasonable
}

// Maskable reports true.
func (m MacAddress) Maskable() bool { return true }

// Mask returns a deterministic locally-administered pseudo-MAC (a
// MacAddress).
func (m MacAddress) Mask() any {
	var masked MacAddress
	copy(masked.bytes[:], security.HmacDigest(m.bytes[:], nil)[:MacLengthBytes])
	masked.SetLocallyAdministered()
	return masked
}

// CanonicalValue returns the raw string form.
func (m MacAddress) CanonicalValue() any { return m.String() }

// String returns the string representation (aa:bb:cc:dd:ee:ff).
func (m MacAddress) String() string {
	parts := make([]string, len(m.bytes))
	for i, b := range m.bytes {
		parts[i] = hex.EncodeToString([]byte{b})
	}
	return strings.Join(parts, ":")
}

// GoString returns the Python repr (same as String).
func (m MacAddress) GoString() string { return m.String() }

// MarshalText returns the raw string form.
func (m MacAddress) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// LogValue renders the MAC at the log security level.
func (m MacAddress) LogValue() slog.Value { return security.LogValue(m) }

// Equal mirrors `__eq__`: other is parsed as a MAC address; values that
// cannot be parsed are not equal.
func (m MacAddress) Equal(other any) bool {
	o, err := MacAddressFromValue(other)
	return err == nil && o == m
}

// pyType renders a value's type like Python's `type(value)!r` for the
// common cases.
func pyType(value any) string {
	switch value.(type) {
	case nil:
		return "<class 'NoneType'>"
	case string:
		return "<class 'str'>"
	case bool:
		return "<class 'bool'>"
	case float32, float64:
		return "<class 'float'>"
	case []any:
		return "<class 'list'>"
	case map[string]any:
		return "<class 'dict'>"
	}
	return fmt.Sprintf("<class '%T'>", value)
}
