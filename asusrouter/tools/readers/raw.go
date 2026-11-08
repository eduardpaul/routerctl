// Readers that parse raw device payloads into shaped data (raw.py).

package readers

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

const ws = converters.PyWhitespaceClass

// Fixups for the malformed JSON some endpoints emit
var (
	jsonDoubleComma   = regexp.MustCompile(ws + `*,` + ws + `*,`)
	jsonLeadingComma  = regexp.MustCompile(`^` + ws + `*\{` + ws + `*,`)
	jsonTrailingComma = regexp.MustCompile(`,` + ws + `*\}` + ws + `*$`)
	jsonEmptyValue    = regexp.MustCompile(`:` + ws + `*(,|\})`)
)

// Legacy firmware emits the get_clientlist value without wrapping braces,
// so its entries leak to the top level; wrap them back into an object
var jsonClientlist = regexp.MustCompile(
	`(?s)("get_clientlist":)` + ws + `*(".*?)` +
		`(` + ws + `*,` + ws + `*"get_clientlist_from_json_database")`,
)

// Consider a 200 with redirect meta-refresh as a bounce page, not real data
var htmlRedirect = regexp.MustCompile(
	`(?i)http-equiv` + ws + `*=` + ws + `*["']?` + ws + `*refresh`,
)

// IsRedirectPage mirrors `is_redirect_page`: whether the content is an
// HTML meta-refresh bounce, not real data.
func IsRedirectPage(content string) bool {
	text := converters.RawToStr(content)
	return text != nil && htmlRedirect.MatchString(*text)
}

// IsTrueInDict mirrors `is_true_in_dict`: check if the value exists in
// the dict and is truthy.
func IsTrueInDict(value string, data map[string]any) bool {
	b := converters.RawToBool(data[value])
	return b != nil && *b
}

// ReadJSONContent mirrors `read_json_content`: get the json content,
// repairing the malformed JSON some devices emit. An empty content
// (Python `None`) gives an empty map.
func ReadJSONContent(content string) map[string]any {
	cleaned := converters.RawToStr(content)
	if cleaned == nil {
		return map[string]any{}
	}

	// Drop control characters that break json
	text := strings.Map(func(r rune) rune {
		if r < 10 {
			return -1
		}
		return r
	}, *cleaned)

	// Handle missing values in JSON
	text = jsonDoubleComma.ReplaceAllLiteralString(text, ", ")
	text = jsonLeadingComma.ReplaceAllLiteralString(text, "{")
	text = jsonTrailingComma.ReplaceAllLiteralString(text, "}")

	// Handle keys without values
	text = jsonEmptyValue.ReplaceAllString(text, ": null${1}")

	// Rewrap an unwrapped get_clientlist value
	text = jsonClientlist.ReplaceAllString(text, "${1}{${2}}${3}")

	jsonData, err := PyJSONLoads(text)
	if err != nil {
		slog.Error(
			"Unable to decode json content with exception. "+
				"Please, copy this and fill in a bug report",
			"error", err, "content", text,
		)
		return map[string]any{}
	}
	if data, ok := jsonData.(map[string]any); ok {
		return data
	}
	return map[string]any{}
}
