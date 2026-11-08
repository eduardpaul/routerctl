package nvram

import (
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// AsHook renders the member as an `nvram_get` hook call.
func (e ARNvramType) AsHook() endpoint.ARHookCall {
	return endpoint.ARHookCall{Hook: endpoint.ARHookNvramGet, Args: string(e)}
}

// Key resolves the template into a raw NVRAM key.
func (e ARNvramIndexType) Key(index any) string {
	return strings.Replace(string(e), "{}", converters.PyStr(index), 1)
}
