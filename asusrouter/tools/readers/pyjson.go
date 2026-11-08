// Go-only port of Python `json.loads` (CPython C scanner, strict=True).
//
// It exists so parsed router data has exactly the Python shape:
// `map[string]any`, `[]any`, `string`, `int64` (integral literals),
// `float64`, `bool`, `nil`. Differences with Python, both unreachable
// from real router payloads:
//   - integers outside the int64 range become float64 (Python: big int);
//   - a lone surrogate escape (`\ud800`) becomes U+FFFD (Go strings are
//     UTF-8).

package readers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// PyJSONError mirrors `json.JSONDecodeError`.
type PyJSONError struct {
	msg string
	pos int
}

func (e *PyJSONError) Error() string {
	return fmt.Sprintf("%s: char %d", e.msg, e.pos)
}

type pyJSONParser struct {
	s string
	i int
}

// PyJSONLoads mirrors `json.loads(s)` for a str argument: objects →
// map[string]any, arrays → []any, integral numbers → int64, other numbers
// → float64. Errors are *PyJSONError (Python `json.JSONDecodeError`).
func PyJSONLoads(s string) (any, error) {
	if strings.HasPrefix(s, "\uFEFF") {
		return nil, &PyJSONError{"Unexpected UTF-8 BOM (decode using utf-8-sig)", 0}
	}
	p := &pyJSONParser{s: s}
	p.ws()
	value, err := p.value()
	if err != nil {
		return nil, err
	}
	p.ws()
	if p.i != len(p.s) {
		return nil, &PyJSONError{"Extra data", p.i}
	}
	return value, nil
}

func (p *pyJSONParser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *pyJSONParser) fail(msg string) error {
	return &PyJSONError{msg, p.i}
}

func (p *pyJSONParser) literal(word string, value any) (any, bool) {
	if strings.HasPrefix(p.s[p.i:], word) {
		p.i += len(word)
		return value, true
	}
	return nil, false
}

func (p *pyJSONParser) value() (any, error) {
	if p.i >= len(p.s) {
		return nil, p.fail("Expecting value")
	}
	switch c := p.s[p.i]; c {
	case '"':
		p.i++
		return p.str()
	case '{':
		p.i++
		return p.object()
	case '[':
		p.i++
		return p.array()
	case 'n':
		if v, ok := p.literal("null", nil); ok {
			return v, nil
		}
	case 't':
		if v, ok := p.literal("true", true); ok {
			return v, nil
		}
	case 'f':
		if v, ok := p.literal("false", false); ok {
			return v, nil
		}
	case 'N':
		if v, ok := p.literal("NaN", math.NaN()); ok {
			return v, nil
		}
	case 'I':
		if v, ok := p.literal("Infinity", math.Inf(1)); ok {
			return v, nil
		}
	case '-':
		if v, ok := p.literal("-Infinity", math.Inf(-1)); ok {
			return v, nil
		}
		return p.number()
	default:
		if c >= '0' && c <= '9' {
			return p.number()
		}
	}
	return nil, p.fail("Expecting value")
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// number mirrors the scanner's `-?(0|[1-9]\d*)(\.\d+)?([eE][-+]?\d+)?`.
func (p *pyJSONParser) number() (any, error) {
	start := p.i
	i := p.i
	if i < len(p.s) && p.s[i] == '-' {
		i++
	}
	switch {
	case i < len(p.s) && p.s[i] == '0':
		i++
	case i < len(p.s) && p.s[i] >= '1' && p.s[i] <= '9':
		for i < len(p.s) && isDigit(p.s[i]) {
			i++
		}
	default:
		return nil, p.fail("Expecting value")
	}
	isFloat := false
	if i+1 < len(p.s) && p.s[i] == '.' && isDigit(p.s[i+1]) {
		isFloat = true
		i += 2
		for i < len(p.s) && isDigit(p.s[i]) {
			i++
		}
	}
	if i < len(p.s) && (p.s[i] == 'e' || p.s[i] == 'E') {
		j := i + 1
		if j < len(p.s) && (p.s[j] == '+' || p.s[j] == '-') {
			j++
		}
		if j < len(p.s) && isDigit(p.s[j]) {
			for j < len(p.s) && isDigit(p.s[j]) {
				j++
			}
			isFloat = true
			i = j
		}
	}
	p.i = i
	return parseNumber(p.s[start:i], isFloat), nil
}

// parseNumber converts a validated number literal: int64 when integral,
// float64 otherwise (and for integers beyond int64).
func parseNumber(text string, isFloat bool) any {
	if !isFloat {
		if v, err := strconv.ParseInt(text, 10, 64); err == nil {
			return v
		}
	}
	// ParseFloat returns ±Inf on overflow, as Python float() does
	f, _ := strconv.ParseFloat(text, 64)
	return f
}

func hexValue(s string) (rune, bool) {
	if len(s) != 4 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil || strings.ContainsAny(s, "+-_") {
		return 0, false
	}
	return rune(v), true
}

// str parses a string body after the opening quote.
func (p *pyJSONParser) str() (string, error) {
	var b strings.Builder
	for {
		if p.i >= len(p.s) {
			return "", p.fail("Unterminated string starting at")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), nil
		case c < 0x20:
			return "", p.fail("Invalid control character at")
		case c != '\\':
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			b.WriteRune(r)
			p.i += size
			continue
		}
		// Escape
		p.i++
		if p.i >= len(p.s) {
			return "", p.fail("Unterminated string starting at")
		}
		esc := p.s[p.i]
		p.i++
		switch esc {
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case '/':
			b.WriteByte('/')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, ok := hexValue(p.s[p.i:min(p.i+4, len(p.s))])
			if !ok {
				return "", p.fail("Invalid \\uXXXX escape")
			}
			p.i += 4
			if utf16.IsSurrogate(r) && r < 0xdc00 &&
				strings.HasPrefix(p.s[p.i:], "\\u") {
				if r2, ok := hexValue(p.s[p.i+2 : min(p.i+6, len(p.s))]); ok &&
					r2 >= 0xdc00 && r2 <= 0xdfff {
					r = utf16.DecodeRune(r, r2)
					p.i += 6
				}
			}
			// Lone surrogates cannot be held by a Go string
			b.WriteRune(r)
		default:
			p.i -= 2
			return "", p.fail("Invalid \\escape")
		}
	}
}

func (p *pyJSONParser) object() (map[string]any, error) {
	result := map[string]any{}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return result, nil
	}
	for {
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, p.fail("Expecting property name enclosed in double quotes")
		}
		p.i++
		key, err := p.str()
		if err != nil {
			return nil, err
		}
		p.ws()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, p.fail("Expecting ':' delimiter")
		}
		p.i++
		p.ws()
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		result[key] = value
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return result, nil
		}
		if p.i >= len(p.s) || p.s[p.i] != ',' {
			return nil, p.fail("Expecting ',' delimiter")
		}
		p.i++
		p.ws()
	}
}

func (p *pyJSONParser) array() ([]any, error) {
	result := []any{}
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return result, nil
	}
	for {
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		result = append(result, value)
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return result, nil
		}
		if p.i >= len(p.s) || p.s[p.i] != ',' {
			return nil, p.fail("Expecting ',' delimiter")
		}
		p.i++
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			return nil, p.fail("Illegal trailing comma before end of array")
		}
	}
}
