package arerror

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestHierarchy(t *testing.T) {
	cases := []struct {
		err   error
		is    []error
		isNot []error
		name  string
	}{
		{NewFallbackForbiddenError("x"), []error{ErrFallbackForbidden, ErrFallback, ErrAsusRouter}, []error{ErrFallbackLoop, ErrAccess}, "AsusRouterFallbackForbiddenError"},
		{NewFallbackLoopError("x"), []error{ErrFallbackLoop, ErrFallback, ErrAsusRouter}, []error{ErrFallbackForbidden}, "AsusRouterFallbackLoopError"},
		{NewFallbackError("x"), []error{ErrFallback, ErrAsusRouter}, []error{ErrFallbackLoop}, "AsusRouterFallbackError"},
		{New404Error("x"), []error{Err404, ErrAsusRouter}, []error{ErrAccess}, "AsusRouter404Error"},
		{NewAccessError("x", 3, nil), []error{ErrAccess, ErrAsusRouter}, []error{ErrLogout}, "AsusRouterAccessError"},
		{NewConnectionError("x"), []error{ErrConnection, ErrAsusRouter}, []error{ErrTimeout}, "AsusRouterConnectionError"},
		{NewLogoutError("x"), []error{ErrLogout, ErrAsusRouter}, nil, "AsusRouterLogoutError"},
		{NewRequestFormatError("x"), []error{ErrRequestFormat, ErrAsusRouter}, nil, "AsusRouterRequestFormatError"},
		{NewSSLCertificateError("x"), []error{ErrSSLCertificate, ErrAsusRouter}, nil, "AsusRouterSSLCertificateError"},
		{NewTimeoutError("x"), []error{ErrTimeout, ErrAsusRouter}, []error{ErrConnection}, "AsusRouterTimeoutError"},
		{NewAsusRouterError("x"), []error{ErrAsusRouter}, []error{ErrAccess}, "AsusRouterError"},
	}
	for _, c := range cases {
		wrapped := fmt.Errorf("wrapped: %w", c.err)
		for _, target := range c.is {
			if !errors.Is(wrapped, target) {
				t.Errorf("%s should be %v", c.name, target.(*AsusRouterError).Class())
			}
		}
		for _, target := range c.isNot {
			if errors.Is(wrapped, target) {
				t.Errorf("%s should not be %v", c.name, target.(*AsusRouterError).Class())
			}
		}
		if got := Name(wrapped); got != c.name {
			t.Errorf("Name = %q, want %q", got, c.name)
		}
		var as *AsusRouterError
		if !errors.As(wrapped, &as) || as.Class() != c.name {
			t.Errorf("errors.As failed for %s", c.name)
		}
	}
}

func TestDistinctInstancesNotEqual(t *testing.T) {
	if errors.Is(NewLogoutError("a"), NewLogoutError("b")) {
		t.Fatal("non-sentinel errors must not match each other")
	}
}

func TestAccessCode(t *testing.T) {
	err := NewAccessError("Access error", 7, map[string]any{"timeout": 5})
	if code, ok := AccessCode(fmt.Errorf("x: %w", err)); !ok || code != 7 {
		t.Fatalf("AccessCode = %d, %v", code, ok)
	}
	if AccessAttributes(err)["timeout"] != 5 {
		t.Fatal("bad attributes")
	}
	if _, ok := AccessCode(NewLogoutError("x")); ok {
		t.Fatal("logout error has no access code")
	}
	if err.Error() != "Access error" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestWithCause(t *testing.T) {
	cause := errors.New("boom")
	err := WithCause(NewConnectionError("cannot connect"), cause)
	if !errors.Is(err, cause) || !errors.Is(err, ErrConnection) {
		t.Fatal("cause not wrapped")
	}
}

func TestNameOther(t *testing.T) {
	if Name(nil) != "" || Name(errors.New("x")) != "Exception" || Name(context.DeadlineExceeded) != "TimeoutError" {
		t.Fatal("bad names for non-AsusRouter errors")
	}
}
