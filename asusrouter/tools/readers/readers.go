// Package readers ports `asusrouter/tools/readers`: parsers turning raw
// device payloads into untyped data shaped like Python's `json.loads`
// output (`map[string]any`, `[]any`, `string`, `int64`, `float64`,
// `bool`, `nil`).
//
// raw.go, js.go, netdev.go, nvram_list.go and table.go mirror the Python
// files. pyjson.go (Python `json.loads`) and literal.go (Python
// `ast.literal_eval`) are Go-only ports of the stdlib parsers they use.
//
// The Python fixup regexes are ported to RE2: `\s` and `\w` become
// `converters.PyWhitespaceClass` / `converters.PyWordClass` to keep the
// Unicode semantics, and matching runs in linear time.
package readers
