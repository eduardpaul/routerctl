// Sensitive value classification and rendering (sensitive.py).

package security

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// RedactedStr is the general placeholder shown in place of any redacted
// value (`REDACTED_STR`).
const RedactedStr = "-=REDACTED=-"

// redactedMarker mirrors `_Redacted`: the marker for a fully redacted
// sensitive value. A single shared instance (`Redacted`) is used.
type redactedMarker struct{}

// String returns the redacted string representation.
func (redactedMarker) String() string { return RedactedStr }

// GoString returns the marker representation (Python repr).
func (redactedMarker) GoString() string { return "<redacted>" }

// CanonicalValue returns the redacted string.
func (redactedMarker) CanonicalValue() any { return RedactedStr }

// Redacted is the shared marker returned when a value is fully hidden
// (`REDACTED`).
var Redacted any = redactedMarker{}

// ARSensitive mirrors the `ARSensitive` mixin marking a value as
// sensitive.
//
// A type declares its reveal level (the minimum security level at which
// the raw value may be exposed) and whether it is maskable (can produce
// a deterministic sanitized representation via Mask). CanonicalValue
// returns the raw value (Python `value`, or `str(x)` when there is none).
type ARSensitive interface {
	RevealLevel() ARSecurityLevel
	Maskable() bool
	// Mask returns a deterministic sanitized representation; types that
	// cannot be masked return Redacted.
	Mask() any
	CanonicalValue() any
}

// PyStringer is implemented by sensitive types whose Python `__str__`
// differs from their Go String() (the redacting ones: Password,
// Username, SensitiveText). It is what a Python log line would show.
type PyStringer interface {
	PyStr() string
}

// Sensitive mirrors `Sensitive`: a generic sensitive value with a
// per-instance reveal level.
//
// Wraps an arbitrary value (e.g. a request payload) that cannot be
// deterministically sanitized: it is shown raw at or above its reveal
// level and redacted otherwise. The zero Level is STRICT; use
// NewSensitive for the Python default (UNSAFE).
type Sensitive struct {
	Value any
	Level ARSecurityLevel
}

// NewSensitive mirrors `Sensitive(value)` with the default UNSAFE level.
func NewSensitive(value any) Sensitive {
	return Sensitive{Value: value, Level: ARSecurityLevelUnsafe}
}

// RevealLevel returns the per-instance reveal level.
func (s Sensitive) RevealLevel() ARSecurityLevel { return s.Level }

// Maskable reports false: a generic value cannot be sanitized.
func (s Sensitive) Maskable() bool { return false }

// Mask returns the redaction marker.
func (s Sensitive) Mask() any { return Redacted }

// CanonicalValue returns the raw wrapped value.
func (s Sensitive) CanonicalValue() any { return s.Value }

// String returns the raw value as a string.
func (s Sensitive) String() string { return converters.PyStr(s.Value) }

// GoString returns the sensitive value representation (Python repr).
func (s Sensitive) GoString() string {
	return fmt.Sprintf(
		"Sensitive(%s, <ARSecurityLevel.%s: %d>)",
		converters.PyRepr(s.Value), s.Level.Name(), int(s.Level),
	)
}

// LogValue renders the value at the current log security level.
func (s Sensitive) LogValue() slog.Value { return LogValue(s) }

// Render mirrors `render`: render a value for the given security level.
//
// Non-sensitive values pass through unchanged. Sensitive values are
// exposed raw at or above their reveal level, sanitized in the masking
// band (maskable and at least SANITIZED), or redacted otherwise.
func Render(value any, level ARSecurityLevel) any {
	sensitive, ok := value.(ARSensitive)
	if !ok {
		return value
	}

	if level >= sensitive.RevealLevel() {
		return value
	}

	if sensitive.Maskable() && level >= ARSecurityLevelSanitized {
		return sensitive.Mask()
	}

	return Redacted
}

// RenderString renders a value at the level and returns the text Python
// would print for the result (`str(render(value, level))`).
func RenderString(value any, level ARSecurityLevel) string {
	return PyStrOf(Render(value, level))
}

// PyStrOf returns Python `str(value)` for a rendered value: the
// PyStringer form for redacting types, String() otherwise.
func PyStrOf(value any) string {
	switch v := value.(type) {
	case PyStringer:
		return v.PyStr()
	case fmt.Stringer:
		return v.String()
	}
	return converters.PyStr(value)
}

// logLevel is the level LogValue renders at. `security/log` points it at
// the effective log level (global and instance configs); until then the
// default log level (SANITIZED) applies.
var logLevel atomic.Pointer[func() ARSecurityLevel]

// SetLogLevelFunc sets the function giving the current log security
// level (used by `security/log`).
func SetLogLevelFunc(f func() ARSecurityLevel) {
	logLevel.Store(&f)
}

// LogLevel returns the current log security level.
func LogLevel() ARSecurityLevel {
	if f := logLevel.Load(); f != nil {
		return (*f)()
	}
	return ARSecurityLevelSanitized
}

// LogValue renders a sensitive value for slog at the current log level,
// as Python's `SensitiveFilter` would print it.
func LogValue(value ARSensitive) slog.Value {
	return slog.StringValue(RenderString(value, LogLevel()))
}
