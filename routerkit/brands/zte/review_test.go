package zte

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A temporary lock answered on (re-)login must not disable automatic
// re-login for good: once lockingTime has passed, the next read logs in.
func TestTemporaryLockIsNotSticky(t *testing.T) {
	fake, c, _ := newFake(t)
	ctx := context.Background()
	fake.LockSeconds = 1
	fake.LockNextLogin()

	if _, err := c.DeviceInfo(ctx); !errors.Is(err, ErrLocked) {
		t.Fatalf("first read: want ErrLocked, got %v", err)
	}
	// Still inside the lock window: no new login attempt is sent
	before := fake.Stats().Requests
	if _, err := c.DeviceInfo(ctx); !errors.Is(err, ErrLocked) {
		t.Fatalf("inside lock window: want ErrLocked, got %v", err)
	}
	if got := fake.Stats().Requests; got != before {
		t.Errorf("sent %d requests while locked, want 0", got-before)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := c.DeviceInfo(ctx); err != nil {
		t.Fatalf("after the lock expired: %v", err)
	}
	if fake.Stats().Logins != 1 {
		t.Errorf("logins = %d, want 1", fake.Stats().Logins)
	}
}

// Refused credentials stay sticky (no retry storm into the lockout).
func TestRefusedCredentialsAreSticky(t *testing.T) {
	fake, c, _ := newFake(t)
	fake.Pass = "other"
	ctx := context.Background()
	if _, err := c.DeviceInfo(ctx); !errors.Is(err, ErrAuth) {
		t.Fatalf("want ErrAuth, got %v", err)
	}
	if _, err := c.DeviceInfo(ctx); !errors.Is(err, ErrAuth) {
		t.Fatalf("want sticky ErrAuth, got %v", err)
	}
	if got := fake.Stats().FailedLogins; got != 1 {
		t.Errorf("failed logins = %d, want 1", got)
	}
}

// One transient page failure must not mark the tag unavailable forever.
func TestTransientViewFailureIsRetried(t *testing.T) {
	fake, c, _ := newFake(t)
	ctx := context.Background()
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	fake.FailView[viewWAN] = 1
	if _, err := c.WANStatus(ctx); err != nil {
		t.Fatalf("read after one transient failure: %v", err)
	}
	if _, err := c.WANStatus(ctx); err != nil {
		t.Fatalf("second read: %v", err)
	}
	// A page that keeps failing is still reported as unavailable
	fake.FailView[viewWAN] = 100
	c.curView = ""
	if _, err := c.WANStatus(ctx); !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("persistent failure: want ErrNotAvailable, got %v", err)
	}
}
