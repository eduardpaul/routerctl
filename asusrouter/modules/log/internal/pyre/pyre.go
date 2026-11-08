// Package pyre is a Go-only helper of the log port (no Python
// counterpart): it lets the log patterns keep their Python `re` text
// byte for byte while matching exactly what Python matches.
//
// Python `re` on a str pattern is Unicode-aware, Go's RE2 is not:
//
//   - `\d` is any Unicode decimal digit (Go: ASCII only)
//   - `\w` is any Unicode letter, digit or `_` (Go: ASCII only)
//   - `\s` is any `str.isspace()` character (Go: ASCII, without `\v`)
//   - `$` also matches right before a final newline (Go: end only)
//
// Compile rewrites those tokens so RE2 behaves like Python. The Python
// patterns of the log package use no lookaround and no backreferences,
// so nothing else needs rewriting; Compile panics on the few tokens it
// cannot express (a negated shorthand inside a class, `\b`, `\B`).
//
// Int mirrors Python `int(text, base)`, including the Unicode digits a
// rewritten `\d` lets through.
package pyre

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	// Python `\d`
	digit = `\p{Nd}`
	// Python `\w`
	word = `\p{L}\p{N}_`
	// Python `\s`: the characters `str.isspace()` accepts
	space = `\t-\r\x1c-\x1f\x85\p{Z}`
)

// Python `$` without MULTILINE: the end, or right before a final newline
const end = `(?:\n?\z)`

var errUnsupported = errors.New("unsupported Python regex token")

// Translate rewrites a Python `re` pattern into the RE2 equivalent.
func Translate(pattern string) (string, error) {
	var b strings.Builder
	rs := []rune(pattern)
	inClass := false
	// Index where the current class content starts (after `[` / `[^`)
	classStart := 0
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\':
			if i+1 >= len(rs) {
				return "", fmt.Errorf("%w: trailing backslash", errUnsupported)
			}
			i++
			n := rs[i]
			repl, err := shorthand(n, inClass)
			if err != nil {
				return "", err
			}
			if repl != "" {
				b.WriteString(repl)
				continue
			}
			b.WriteRune('\\')
			b.WriteRune(n)
		case inClass:
			// A `]` first in the class is a literal, like in Python
			if c == ']' && i != classStart {
				inClass = false
			}
			b.WriteRune(c)
		case c == '[':
			inClass = true
			b.WriteRune(c)
			classStart = i + 1
			if classStart < len(rs) && rs[classStart] == '^' {
				b.WriteRune('^')
				i++
				classStart++
			}
		case c == '$':
			b.WriteString(end)
		default:
			b.WriteRune(c)
		}
	}
	return b.String(), nil
}

// shorthand returns the rewrite of `\<n>` ("" when it stays as written).
func shorthand(n rune, inClass bool) (string, error) {
	switch n {
	case 'd':
		return digit, nil
	case 'D':
		return `\P{Nd}`, nil
	case 'w':
		if inClass {
			return word, nil
		}
		return "[" + word + "]", nil
	case 's':
		if inClass {
			return space, nil
		}
		return "[" + space + "]", nil
	case 'W', 'S':
		if inClass {
			return "", fmt.Errorf("%w: \\%c inside a class", errUnsupported, n)
		}
		if n == 'W' {
			return "[^" + word + "]", nil
		}
		return "[^" + space + "]", nil
	case 'b', 'B':
		return "", fmt.Errorf("%w: \\%c", errUnsupported, n)
	case 'Z':
		return `\z`, nil
	}
	return "", nil
}

// Compile translates and compiles a Python pattern, panicking on error
// (like `regexp.MustCompile`; the patterns are package constants).
func Compile(pattern string) *regexp.Regexp {
	translated, err := Translate(pattern)
	if err != nil {
		panic(fmt.Sprintf("pyre: %q: %v", pattern, err))
	}
	return regexp.MustCompile(translated)
}

// Split mirrors Python `re.split` for a pattern with capturing groups:
// the text between matches interleaved with every group of each match
// (a group that did not take part gives "", Python gives None).
func Split(re *regexp.Regexp, text string) []string {
	matches := re.FindAllStringSubmatchIndex(text, -1)
	out := make([]string, 0, len(matches)*(re.NumSubexp()+1)+1)
	last := 0
	for _, m := range matches {
		out = append(out, text[last:m[0]])
		for g := 1; g <= re.NumSubexp(); g++ {
			if m[2*g] < 0 {
				out = append(out, "")
				continue
			}
			out = append(out, text[m[2*g]:m[2*g+1]])
		}
		last = m[1]
	}
	return append(out, text[last:])
}

// Group returns a named group of a match and whether it took part
// (Python `match.group(name)` being not None).
func Group(re *regexp.Regexp, text string, match []int, name string) (string, bool) {
	index := re.SubexpIndex(name)
	if index < 0 || match[2*index] < 0 {
		return "", false
	}
	return text[match[2*index]:match[2*index+1]], true
}

// isSpace mirrors Python `str.isspace` for one character.
func isSpace(r rune) bool {
	switch {
	case r >= '\t' && r <= '\r', r >= 0x1c && r <= 0x1f, r == 0x85:
		return true
	}
	return unicode.In(r, unicode.Z)
}

// decimal returns the value of a Unicode decimal digit, or -1.
//
// Decimal digits come in runs of ten from zero, and the range tables
// never split a run, so a range always starts on a zero.
func decimal(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	if r < 0x80 || !unicode.Is(unicode.Nd, r) {
		return -1
	}
	for _, rg := range unicode.Nd.R16 {
		if r >= rune(rg.Lo) && r <= rune(rg.Hi) {
			return int(r-rune(rg.Lo)) % 10
		}
	}
	for _, rg := range unicode.Nd.R32 {
		if r >= rune(rg.Lo) && r <= rune(rg.Hi) {
			return int(r-rune(rg.Lo)) % 10
		}
	}
	return -1
}

// Int mirrors Python `int(text, base)` for a str and base 0 or 2..36.
// ok is false where Python raises ValueError (or the value overflows a
// Go int).
func Int(text string, base int) (value int, ok bool) {
	// Python first turns Unicode digits into ASCII and trims whitespace
	var b strings.Builder
	for _, r := range text {
		if d := decimal(r); d >= 0 {
			b.WriteByte(byte('0' + d))
			continue
		}
		b.WriteRune(r)
	}
	s := strings.TrimFunc(b.String(), isSpace)

	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if len(s) >= 2 && s[0] == '0' {
		prefix := map[byte]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[s[1]]
		if prefix != 0 && (base == 0 || base == prefix) {
			base = prefix
			s = strings.TrimPrefix(s[2:], "_")
		}
	}
	auto := base == 0
	if auto {
		base = 10
	}
	if base < 2 || base > 36 || s == "" {
		return 0, false
	}
	// Underscores only between digits
	if s[0] == '_' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	digits := strings.ReplaceAll(s, "_", "")
	// Base 0 forbids leading zeros on a non-zero decimal
	if auto && len(digits) > 1 && digits[0] == '0' && strings.Trim(digits, "0") != "" {
		return 0, false
	}
	const limit = int(^uint(0) >> 1)
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'z':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'Z':
			d = int(c-'A') + 10
		default:
			return 0, false
		}
		if d >= base {
			return 0, false
		}
		if value > (limit-d)/base {
			return 0, false
		}
		value = value*base + d
	}
	if neg {
		value = -value
	}
	return value, true
}
