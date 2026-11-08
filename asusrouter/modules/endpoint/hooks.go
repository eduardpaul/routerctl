package endpoint

import "strings"

// ARHookCall is a hook with its exact argument string.
type ARHookCall struct {
	Hook ARHook
	Args string
}

// ARHookItem is an item that renders itself as a single appGet hook call.
type ARHookItem interface {
	AsHook() ARHookCall
}

// AsHook lets a plain hook be used as an item (no arguments).
func (h ARHook) AsHook() ARHookCall { return ARHookCall{Hook: h} }

// AsHook lets a call be used as an item.
func (c ARHookCall) AsHook() ARHookCall { return c }

// HookRequest mirrors `hook_request`: build an appGet `hook=` request
// string from one or more hooks.
func HookRequest(items ...ARHookItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		call := item.AsHook()
		parts = append(parts, string(call.Hook)+"("+call.Args+")")
	}
	return "hook=" + strings.Join(parts, ";")
}

// HookValue mirrors `hook_value`: read one hook item's value from a
// parsed response, or nil. Plain hooks key the response by their name;
// argument-carrying items (e.g. `nvram_get`) key it by the argument.
func HookValue(data any, item ARHookItem) any {
	m, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	if h, isHook := item.(ARHook); isHook {
		return m[string(h)]
	}
	return m[item.AsHook().Args]
}
