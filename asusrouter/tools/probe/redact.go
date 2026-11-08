// Free-text scrubbing for AsusRouter probe reports (redact.py).

package probe

import (
	"regexp"
	"strings"
	"unicode"
)

// Placeholders left where an identifier was removed
const (
	RedactedMAC = "-=MAC=-"
	RedactedIP  = "-=IP=-"
)

var (
	macRe  = regexp.MustCompile(`\b[0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}\b`)
	ipv4Re = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	// Four groups minimum, so a `HH:MM:SS` timestamp is never taken for an
	// address; the compressed `::` form is only caught when it is this long
	ipv6Re = regexp.MustCompile(`\b[0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){3,}\b`)
)

// pySub mirrors Python `pattern.sub(repl, text)` for a pattern written
// with Python's Unicode `\b` and `\d`.
//
// RE2's `\b`/`\d` are ASCII-only, so the pattern runs on an ASCII shadow
// of the text with one byte per character: non-ASCII word characters
// become `_` (a word character no pattern matches), non-ASCII decimal
// digits become `0` when the pattern uses `\d`, anything else becomes
// `!`. Matches are then applied to the original characters.
func pySub(re *regexp.Regexp, text, repl string, unicodeDigits bool) string {
	runes := []rune(text)
	shadow := make([]byte, len(runes))
	ascii := true
	for i, r := range runes {
		switch {
		case r < 0x80:
			shadow[i] = byte(r)
			continue
		case unicodeDigits && unicode.Is(unicode.Nd, r):
			shadow[i] = '0'
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			shadow[i] = '_'
		default:
			shadow[i] = '!'
		}
		ascii = false
	}
	if ascii {
		return re.ReplaceAllLiteralString(text, repl)
	}

	var b strings.Builder
	last := 0
	for _, m := range re.FindAllIndex(shadow, -1) {
		b.WriteString(string(runes[last:m[0]]))
		b.WriteString(repl)
		last = m[1]
	}
	b.WriteString(string(runes[last:]))
	return b.String()
}

// ScrubText mirrors `scrub_text`: replace the identifiers a log message
// can carry in free text.
func ScrubText(text string) string {
	text = pySub(macRe, text, RedactedMAC, false)
	text = pySub(ipv6Re, text, RedactedIP, false)

	return pySub(ipv4Re, text, RedactedIP, true)
}
