package endpoint

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// ReadWanLanStatus mirrors `read_wan_lan_status`.
//
// The endpoint wraps the JSON in a `get_wan_lan_status = {...};`
// assignment, so the prefix and trailing semicolon are stripped first.
func ReadWanLanStatus(content string) map[string]any {
	text := ""
	if s := converters.RawToStr(content); s != nil {
		text = *s
	}
	text = strings.ReplaceAll(text, "get_wan_lan_status = ", "")
	text = strings.ReplaceAll(text, ";", "")
	return readers.ReadJSONContent(text)
}
