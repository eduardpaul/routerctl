// Package translate mirrors asusrouter/modules/log/translate/.
//
// All Python modules of the package share one Go package, so the
// module-level names of each file carry the file's name: Python
// `kernel.PATTERNS` is Go `KernelPatterns`, and the private `_CLIENT` of
// `band_steering_daemon.py` is `bandSteeringClient`.
package translate

// Shared regex fragments for log event translation.

// MacPattern mirrors `MAC_PATTERN`.
// A MAC address as the firmware prints it (colon-separated, unpadded)
const MacPattern = `[0-9A-Fa-f:]+`

// WlPattern mirrors `WL_PATTERN`.
// A `wl<unit>[.<subunit>]` interface token, read as the `wl_id` field
const WlPattern = `(?P<wl_id>wl[\d.]+)`

// RadioPattern mirrors `RADIO_PATTERN`.
// A radio named either way
const RadioPattern = `(?:` + WlPattern + `|(?P<interface>eth\d+))`
