package asusrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

func newTestConn(t *testing.T, opts ConnectionOptions) *Connection {
	t.Helper()
	c, err := NewConnection("192.0.2.1", "admin", "pw", opts)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A pinned / CA-verified connection never falls back to plaintext HTTP
func TestFallbackNeverDowngradesPinned(t *testing.T) {
	for name, opts := range map[string]ConnectionOptions{
		"pin": {UseSSL: true, PinnedCertSHA256: make([]byte, 32)},
	} {
		c := newTestConn(t, opts)
		if _, _, err := c.nextFallbackConfig(); !errors.Is(err, arerror.ErrFallbackForbidden) {
			t.Errorf("%s: want FallbackForbidden, got %v", name, err)
		}
	}
	// Unpinned keeps the Python matrix: HTTPS@default -> HTTP@default
	c := newTestConn(t, ConnectionOptions{UseSSL: true})
	useSSL, port, err := c.nextFallbackConfig()
	if err != nil || useSSL || port != consts.DefaultPortHTTP {
		t.Errorf("unpinned: got (%v, %d, %v)", useSSL, port, err)
	}
}

// A request that failed on a config another request already replaced
// retries on the current config instead of advancing the fallback again
func TestFallbackTransitionStaleGeneration(t *testing.T) {
	c := newTestConn(t, ConnectionOptions{UseSSL: true, Port: 9443})
	gen := c.fallbackGen

	// First failing request: HTTPS@custom -> HTTPS@default
	retry, err := c.transition(gen, c.nextFallbackConfig)
	if err != nil || retry {
		t.Fatalf("first transition: retry=%v err=%v", retry, err)
	}
	if c.Port() != consts.DefaultPortHTTPS || c.HTTP() != "https" {
		t.Fatalf("after first transition: %s:%d", c.HTTP(), c.Port())
	}

	// Second request started before the first transition (stale gen)
	retry, err = c.transition(gen, c.nextFallbackConfig)
	if err != nil || !retry {
		t.Fatalf("stale transition: retry=%v err=%v", retry, err)
	}
	// Still HTTPS: the stale request must not push it down to HTTP
	if c.HTTP() != "https" || c.Port() != consts.DefaultPortHTTPS {
		t.Errorf("stale request advanced the fallback: %s:%d", c.HTTP(), c.Port())
	}
	if v, _ := c.Config().Get(config.ARConnectionConfigKeyUseSsl).(bool); !v {
		t.Error("use_ssl flipped by a stale request")
	}
}

func TestLoginContext(t *testing.T) {
	ctx := context.Background()
	if inLoginContext(ctx) {
		t.Error("plain context marked as login")
	}
	if !inLoginContext(context.WithValue(ctx, loginCtxKey{}, true)) {
		t.Error("login context not detected")
	}
}
