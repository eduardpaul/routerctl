// Package identifiers ports `asusrouter/tools/identifiers`: the
// sensitive identifier types (MAC and IP addresses, hostnames, serials,
// SSIDs, credentials) and the WiFi interface identifier.
//
// Every sensitive type implements `security.ARSensitive`, `String()` (the
// raw value, even where Python's `__str__` redacts), `CanonicalValue()`
// (the raw value used in parity output), `slog.LogValuer` (rendered at
// the log security level) and `encoding.TextMarshaler` (raw).
package identifiers

import "reflect"

// isNone reports whether a value stands for Python `None` (nil or a nil
// pointer).
func isNone(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
