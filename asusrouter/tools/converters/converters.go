// Package converters ports `asusrouter/tools/converters`.
//
// raw.go mirrors `raw.py`, int.go mirrors `int.py`. pystr.go and
// datetime.go are Go-only helpers that reproduce the Python builtins
// (`str.strip`, `int()`, `float()`, `str()`, `datetime.fromisoformat`,
// `datetime.strptime`) the converters depend on.
package converters
