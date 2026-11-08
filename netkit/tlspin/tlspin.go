// Package tlspin holds the certificate checks shared by every brand
// library (Go-only; see asusrouter/PARITY.md "Deliberate deviations"): SHA-256
// fingerprint pins and custom CA pools, alone or combined.
package tlspin

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// PinError reports a certificate that does not match the pinned
// fingerprint.
type PinError struct {
	// Got is the hex SHA-256 of the presented leaf ("" when none)
	Got string
}

func (e *PinError) Error() string {
	if e.Got == "" {
		return "no server certificate to check against the pinned SHA-256 fingerprint"
	}
	return "server certificate does not match the pinned SHA-256 fingerprint (got " + e.Got + ")"
}

// ParseFingerprint parses a SHA-256 certificate fingerprint written as
// hex, with or without `:` / space separators and an optional `sha256=`
// prefix (the formats browsers and `openssl x509 -fingerprint -sha256`
// print).
func ParseFingerprint(s string) ([]byte, error) {
	clean := strings.ToLower(strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(s)))
	clean = strings.TrimPrefix(clean, "sha256=")
	b, err := hex.DecodeString(clean)
	if err != nil || len(b) != sha256.Size {
		return nil, fmt.Errorf("invalid SHA-256 fingerprint %q", s)
	}
	return b, nil
}

// VerifyPin checks the leaf of rawCerts against pin.
func VerifyPin(rawCerts [][]byte, pin []byte) error {
	if len(rawCerts) == 0 {
		return &PinError{}
	}
	sum := sha256.Sum256(rawCerts[0])
	if subtle.ConstantTimeCompare(sum[:], pin) != 1 {
		return &PinError{Got: hex.EncodeToString(sum[:])}
	}
	return nil
}

// Apply configures cfg for the requested checks:
//
//   - roots only: standard verification (chain, hostname, expiry) against
//     roots instead of the system pool
//   - pin only: the leaf must match the pin (self-signed router certs)
//   - roots and pin: both - chain, hostname and expiry against roots, and
//     the leaf must match the pin
//   - neither: insecure decides whether verification is skipped
//
// Checks run in VerifyConnection, so they also apply to resumed sessions.
func Apply(cfg *tls.Config, roots *x509.CertPool, pin []byte, insecure bool) {
	switch {
	case len(pin) == 0 && roots != nil:
		cfg.RootCAs = roots
		cfg.InsecureSkipVerify = false
	case len(pin) > 0:
		// The default verifier is replaced by VerifyConnection below
		cfg.InsecureSkipVerify = true //nolint:gosec // verified in VerifyConnection
		cfg.RootCAs = roots
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if roots != nil {
				if err := verifyChain(cs, roots, cfg.ServerName); err != nil {
					return err
				}
			}
			raw := make([][]byte, 0, len(cs.PeerCertificates))
			for _, c := range cs.PeerCertificates {
				raw = append(raw, c.Raw)
			}
			return VerifyPin(raw, pin)
		}
	default:
		cfg.InsecureSkipVerify = insecure //nolint:gosec // explicit user/library choice
	}
}

func verifyChain(cs tls.ConnectionState, roots *x509.CertPool, serverName string) error {
	if len(cs.PeerCertificates) == 0 {
		return errors.New("no server certificate")
	}
	name := serverName
	if name == "" {
		name = cs.ServerName
	}
	inter := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		DNSName:       name,
	})
	return err
}
