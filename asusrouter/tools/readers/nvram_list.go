// Readers for `<`-row / `>`-field encoded NVRAM lists (nvram_list.py).

package readers

import "strings"

// Decode mirrors `decode`: decode the char-encoded nvram separators to
// `<`/`>`. A non-string gives "".
func Decode(raw any) string {
	var text string
	switch v := raw.(type) {
	case string:
		text = v
	case *string:
		if v == nil {
			return ""
		}
		text = *v
	default:
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "&#60", "<"), "&#62", ">")
}

// SplitRows mirrors `split_rows`: decode a nvram list and split it into
// its `<`-delimited rows.
//
// Rows are kept verbatim (including empty ones), so positional lists
// stay aligned and can be re-serialized without loss.
func SplitRows(raw any) []string {
	text := Decode(raw)
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "<")
}

// GetField mirrors `get_field`: return a `>`-split field, or nil when
// absent.
func GetField(parts []string, index int) *string {
	if 0 <= index && index < len(parts) {
		return &parts[index]
	}
	return nil
}
