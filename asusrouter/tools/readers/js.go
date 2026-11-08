// Readers for ASUS JS-variable payloads (js.py).

package readers

import (
	"regexp"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// A JS `name = value;` assignment (value may be a quoted string).
//
// Python anchors the name with `\b`, which RE2 cannot express with a
// Unicode `\w`. It is not needed: a leftmost match of `\w+\s*=` always
// starts at the beginning of a word, and every search resumes right after
// a `;`, so the boundary always holds.
var jsVarRe = regexp.MustCompile(
	`(?m)` +
		// variable name and assignment
		`(` + converters.PyWordClass + `+)` + ws + `*=` + ws + `*` +
		`(` +
		`(?:` +
		// double-quoted string
		`"(?:[^"\\]|\\.)*"` +
		// single-quoted string
		`|'(?:[^'\\]|\\.)*'` +
		// or anything up to semicolon
		`|[^;]*?` +
		`)` +
		`)` +
		// semicolon ends the assignment
		ws + `*;`,
)

var arrayIndexRe = regexp.MustCompile(`\[\p{Nd}+\]`)

// ReadJSVariables mirrors `read_js_variables`: get all the JS variables
// from the content.
func ReadJSVariables(content string) map[string]any {
	text := ""
	if cleaned := converters.RawToStr(content); cleaned != nil {
		text = *cleaned
	}
	jsVariables := map[string]any{}

	for _, match := range jsVarRe.FindAllStringSubmatch(text, -1) {
		key, value := match[1], match[2]

		// Clean value from the array indexes
		value = arrayIndexRe.ReplaceAllLiteralString(value, "")

		// Try JSON
		if parsed, err := PyJSONLoads(value); err == nil {
			jsVariables[key] = parsed
			continue
		}
		// Try python literal eval
		if parsed, err := literalEval(value); err == nil {
			jsVariables[key] = parsed
			continue
		}
		// Clean the value of quotes if it starts and ends with them
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") ||
			strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			value = sliceRunes(value, 1, -1)
		}
		jsVariables[key] = value
	}

	return jsVariables
}

// sliceRunes mirrors Python `s[start:end]` (code points, negative end).
func sliceRunes(s string, start, end int) string {
	r := []rune(s)
	if end < 0 {
		end += len(r)
	}
	if start > len(r) {
		start = len(r)
	}
	if end < start {
		return ""
	}
	return string(r[start:end])
}

// ReadJSSection mirrors `read_js_section`: read a section from
// JS-variable data.
//
// ASUS JS variables wrap their payload in a single-element list cover
// (`name = [VALUE][0];`); after parsing, the value is `[VALUE]`. Return
// its first element, or nil when absent. Only a list (`[]any`, not a
// Tuple) counts, as in Python.
func ReadJSSection(data any, key string) any {
	var value any
	switch d := data.(type) {
	case map[string]any:
		value = d[key]
	case map[any]any:
		value = d[key]
	default:
		return nil
	}
	if list, ok := value.([]any); ok && len(list) > 0 {
		return list[0]
	}
	return nil
}
