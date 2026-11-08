package asusrouter

import "github.com/eduardpaul/routerctl/netkit/tlspin"

// ParseFingerprint parses a SHA-256 certificate fingerprint (hex, with
// or without `:` separators). See netkit/tlspin.
func ParseFingerprint(s string) ([]byte, error) { return tlspin.ParseFingerprint(s) }
