// Text security tools (text.py).

package security

import (
	"log/slog"
	"regexp"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// SensitiveText mirrors `SensitiveText`: a device string exposed only
// from its own level upwards.
//
// The zero Level is STRICT; use NewSensitiveText for the Python default
// (UNSAFE). Two texts are equal (==) when value and level match.
type SensitiveText struct {
	Value string
	Level ARSecurityLevel
}

// NewSensitiveText mirrors `SensitiveText(value)` with the default UNSAFE
// level.
func NewSensitiveText(value string) SensitiveText {
	return SensitiveText{Value: value, Level: ARSecurityLevelUnsafe}
}

// RevealLevel returns the level from which the text may be shown.
func (t SensitiveText) RevealLevel() ARSecurityLevel { return t.Level }

// Maskable reports false: free text can only be withheld.
func (t SensitiveText) Maskable() bool { return false }

// Mask returns the redaction marker.
func (t SensitiveText) Mask() any { return Redacted }

// CanonicalValue returns the text itself.
func (t SensitiveText) CanonicalValue() any { return t.Value }

// String returns the text itself (Python `str()` redacts: see PyStr).
func (t SensitiveText) String() string { return t.Value }

// PyStr returns the redacted representation (Python `__str__`).
func (t SensitiveText) PyStr() string { return RedactedStr }

// GoString returns the redacted representation (Python `__repr__`).
func (t SensitiveText) GoString() string { return "SensitiveText(" + RedactedStr + ")" }

// MarshalText returns the raw text.
func (t SensitiveText) MarshalText() ([]byte, error) { return []byte(t.Value), nil }

// LogValue renders the text at the current log security level.
func (t SensitiveText) LogValue() slog.Value { return LogValue(t) }

// Searchable mirrors `searchable`: return a secret's text as a message
// would spell it, if it has one.
func Searchable(secret ARSensitive) *string {
	var text string
	if value := secret.CanonicalValue(); value != nil {
		text = converters.PyStr(value)
	} else {
		text = PyStrOf(secret)
	}

	if text == "" || text == RedactedStr {
		return nil
	}
	return &text
}

// RemoveSecrets mirrors `remove_secrets`: take known secret values out of
// free text below their level (case-insensitive, literal replacement).
func RemoveSecrets(text string, secrets []ARSensitive, level ARSecurityLevel) string {
	for _, secret := range secrets {
		if level >= secret.RevealLevel() {
			continue
		}
		value := Searchable(secret)
		if value == nil {
			continue
		}

		replacement := RenderString(secret, level)
		pattern := regexp.MustCompile("(?i)" + regexp.QuoteMeta(*value))
		text = pattern.ReplaceAllLiteralString(text, replacement)
	}

	return text
}
