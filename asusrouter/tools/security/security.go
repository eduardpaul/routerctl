// Package security ports `asusrouter/tools/security`: security levels,
// the deterministic masking engine, sensitive value rendering and text
// scrubbing.
//
// The log masking (`log.py`) lives in the `security/log` subpackage: it
// needs `config`, which itself needs `ARSecurityLevel` from here.
package security
