// Go-only helpers reproducing the Python builtins the converters rely on
// (`str.strip`, `int(str, base)`, `float(str)`, `str(x)`, `repr(x)`).
//
// They have no Python counterpart file; they exist so the Go converters
// keep the exact Python semantics.

package converters

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PyIsSpace mirrors Python `str.isspace` for a single character (this is
// also what Python's `\s` matches in str patterns).
func PyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// PyStrip mirrors Python `str.strip()` without arguments.
func PyStrip(s string) string {
	return strings.TrimFunc(s, PyIsSpace)
}

// PySplit mirrors Python `str.split()` without arguments.
func PySplit(s string) []string {
	return strings.FieldsFunc(s, PyIsSpace)
}

// PyWhitespaceClass is a regexp character class matching Python's `\s`
// (RE2's `\s` is ASCII-only and misses `\v`).
const PyWhitespaceClass = `[\t\n\v\f\r \x1c-\x1f\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`

// PyWordClass is a regexp character class matching Python's Unicode `\w`.
const PyWordClass = `[\p{L}\p{N}_]`

// parsePyInt mirrors Python `int(text, base)` for bases 0 and 2..36.
//
// Only ASCII digits are accepted (Python also takes other Unicode decimal
// digits). Values outside the Go int range fail.
func parsePyInt(text string, base int) (int, bool) {
	s := PyStrip(text)
	neg := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}

	prefixed := false
	if len(s) >= 2 && s[0] == '0' {
		p := unicode.ToLower(rune(s[1]))
		switch {
		case p == 'x' && (base == 16 || base == 0):
			base, prefixed = 16, true
		case p == 'o' && (base == 8 || base == 0):
			base, prefixed = 8, true
		case p == 'b' && (base == 2 || base == 0):
			base, prefixed = 2, true
		}
		if prefixed {
			s = s[2:]
			// One underscore is allowed right after the prefix
			if strings.HasPrefix(s, "_") {
				s = s[1:]
			}
		}
	}
	decimalAuto := false
	if base == 0 {
		base, decimalAuto = 10, true
	}
	if base < 2 || base > 36 || s == "" {
		return 0, false
	}

	// Underscores only between digits
	if s[0] == '_' || s[len(s)-1] == '_' || strings.Contains(s, "__") {
		return 0, false
	}
	digits := strings.ReplaceAll(s, "_", "")
	for _, c := range digits {
		if digitValue(c) >= base {
			return 0, false
		}
	}
	// Base 0 forbids leading zeros in non-zero decimals
	if decimalAuto && len(digits) > 1 && digits[0] == '0' &&
		strings.Trim(digits, "0") != "" {
		return 0, false
	}

	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		if u > 1<<63 {
			return 0, false
		}
		return int(-int64(u)), true
	}
	if u > math.MaxInt64 {
		return 0, false
	}
	return int(u), true
}

func digitValue(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

var pyFloatRe = regexp.MustCompile(
	`^[+-]?(?:(?:\d(?:_?\d)*)?\.\d(?:_?\d)*(?:[eE][+-]?\d(?:_?\d)*)?` +
		`|\d(?:_?\d)*\.?(?:[eE][+-]?\d(?:_?\d)*)?)$`,
)

// parsePyFloat mirrors Python `float(text)`.
//
// Only ASCII digits are accepted (Python also takes other Unicode decimal
// digits).
func parsePyFloat(text string) (float64, bool) {
	s := PyStrip(text)
	unsigned := strings.ToLower(strings.TrimLeft(s, "+-"))
	if len(s)-len(unsigned) <= 1 {
		neg := strings.HasPrefix(s, "-")
		switch unsigned {
		case "inf", "infinity":
			if neg {
				return math.Inf(-1), true
			}
			return math.Inf(1), true
		case "nan":
			return math.NaN(), true
		}
	}
	if !pyFloatRe.MatchString(s) {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64)
	if err != nil {
		// Out of range: Python gives inf (or 0.0), as ParseFloat does
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f, true
		}
		return 0, false
	}
	return f, true
}

// FloatRepr mirrors Python `repr(float)`.
func FloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64)
	exp, _ := strconv.Atoi(e[strings.IndexByte(e, 'e')+1:])
	if exp >= -4 && exp < 16 {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.ContainsAny(s, ".") {
			s += ".0"
		}
		return s
	}
	return e
}

// PyStr mirrors Python `str(value)` for the values the library handles
// (None, bool, numbers, str, lists, dicts). Other values use their
// `String()`/`Error()` or `fmt.Sprint`.
func PyStr(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case *string:
		if v != nil {
			return *v
		}
	}
	return pyFormat(value, false)
}

// PyRepr mirrors Python `repr(value)` for the same values as `PyStr`.
func PyRepr(value any) string {
	return pyFormat(value, true)
}

func pyFormat(value any, repr bool) string {
	value = deref(value)
	switch v := value.(type) {
	case nil:
		return "None"
	case bool:
		if v {
			return "True"
		}
		return "False"
	case string:
		if repr {
			return strRepr(v)
		}
		return v
	case float64:
		return FloatRepr(v)
	case float32:
		return FloatRepr(float64(v))
	case fmt.Stringer:
		return v.String()
	case error:
		return v.Error()
	}
	if i, ok := asInt(value); ok {
		return strconv.Itoa(i)
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if b, ok := value.([]byte); ok {
			return bytesRepr(b)
		}
		parts := make([]string, rv.Len())
		for i := range parts {
			parts[i] = pyFormat(rv.Index(i).Interface(), true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case reflect.Map:
		keys := rv.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = pyFormat(k.Interface(), true) + ": " +
				pyFormat(rv.MapIndex(k).Interface(), true)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(value)
}

// strRepr mirrors Python `repr(str)`.
func strRepr(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == rune(quote) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r == utf8.RuneError || !unicode.IsPrint(r) && r != ' ':
			switch {
			case r <= 0xff:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r <= 0xffff:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// bytesRepr mirrors Python `repr(bytes)`.
func bytesRepr(data []byte) string {
	quote := byte('\'')
	if strings.IndexByte(string(data), '\'') >= 0 &&
		strings.IndexByte(string(data), '"') < 0 {
		quote = '"'
	}
	var b strings.Builder
	b.WriteString("b")
	b.WriteByte(quote)
	for _, c := range data {
		switch {
		case c == quote || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c >= 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
}
