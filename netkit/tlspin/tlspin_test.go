package tlspin

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseFingerprint(t *testing.T) {
	hexs := strings.Repeat("ab", 32)
	colon := strings.ToUpper(strings.Join(strings.SplitAfter(hexs, "")[0:0], ""))
	_ = colon
	for _, in := range []string{hexs, "sha256=" + hexs, "AB:" + strings.Repeat("AB:", 30) + "AB", " " + hexs + " "} {
		if _, err := ParseFingerprint(in); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
	for _, in := range []string{"", "zz", strings.Repeat("ab", 31)} {
		if _, err := ParseFingerprint(in); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

func get(t *testing.T, srv *httptest.Server, roots *x509.CertPool, pin []byte) error {
	t.Helper()
	cfg := &tls.Config{}
	Apply(cfg, roots, pin, false)
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
	resp, err := c.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
	}
	return err
}

func TestApply(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cert := srv.Certificate()
	sum := sha256.Sum256(cert.Raw)
	good, _ := ParseFingerprint(hex.EncodeToString(sum[:]))
	bad := make([]byte, 32)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	other := x509.NewCertPool()

	if err := get(t, srv, nil, good); err != nil {
		t.Errorf("pin only, good: %v", err)
	}
	var pe *PinError
	if err := get(t, srv, nil, bad); !errors.As(err, &pe) {
		t.Errorf("pin only, bad: want PinError, got %v", err)
	}
	if err := get(t, srv, roots, nil); err != nil {
		t.Errorf("roots only, good: %v", err)
	}
	if err := get(t, srv, other, nil); err == nil {
		t.Error("roots only, wrong pool: want error")
	}
	if err := get(t, srv, roots, good); err != nil {
		t.Errorf("roots+pin, good: %v", err)
	}
	// Both configured: a matching pin must not bypass the CA check
	if err := get(t, srv, other, good); err == nil {
		t.Error("roots+pin, wrong pool: want error (pin must not disable the CA check)")
	}
	if err := get(t, srv, roots, bad); !errors.As(err, &pe) {
		t.Errorf("roots+pin, bad pin: want PinError, got %v", err)
	}
}
