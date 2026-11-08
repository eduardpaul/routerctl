// Package arerror ports `asusrouter/error.py` (named to avoid clashing
// with the stdlib `errors`).
//
// Every Python exception class is a class name on one Go type,
// `*AsusRouterError`. The hierarchy is kept for `errors.Is` through the
// `Err*` sentinels: an error is the sentinel of its own class and of every
// parent class, so a fallback-forbidden error matches
// `ErrFallbackForbidden`, `ErrFallback` and `ErrAsusRouter`.
package arerror

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Python class names
const (
	ClassAsusRouterError                  = "AsusRouterError"
	ClassAsusRouter404Error               = "AsusRouter404Error"
	ClassAsusRouterAccessError            = "AsusRouterAccessError"
	ClassAsusRouterConnectionError        = "AsusRouterConnectionError"
	ClassAsusRouterFallbackError          = "AsusRouterFallbackError"
	ClassAsusRouterFallbackForbiddenError = "AsusRouterFallbackForbiddenError"
	ClassAsusRouterFallbackLoopError      = "AsusRouterFallbackLoopError"
	ClassAsusRouterLogoutError            = "AsusRouterLogoutError"
	ClassAsusRouterRequestFormatError     = "AsusRouterRequestFormatError"
	ClassAsusRouterSSLCertificateError    = "AsusRouterSSLCertificateError"
	ClassAsusRouterTimeoutError           = "AsusRouterTimeoutError"
)

// parents maps each class to its Python base class.
var parents = map[string]string{
	ClassAsusRouter404Error:               ClassAsusRouterError,
	ClassAsusRouterAccessError:            ClassAsusRouterError,
	ClassAsusRouterConnectionError:        ClassAsusRouterError,
	ClassAsusRouterFallbackError:          ClassAsusRouterError,
	ClassAsusRouterFallbackForbiddenError: ClassAsusRouterFallbackError,
	ClassAsusRouterFallbackLoopError:      ClassAsusRouterFallbackError,
	ClassAsusRouterLogoutError:            ClassAsusRouterError,
	ClassAsusRouterRequestFormatError:     ClassAsusRouterError,
	ClassAsusRouterSSLCertificateError:    ClassAsusRouterError,
	ClassAsusRouterTimeoutError:           ClassAsusRouterError,
}

// AsusRouterError mirrors `AsusRouterError` and, through Class, all its
// subclasses.
type AsusRouterError struct {
	class string
	// Args holds the positional Python arguments (`ex.args` without the
	// trailing `message`): `Args[0]` is usually the message.
	Args []any
	// Message mirrors the `message` keyword argument.
	Message *string
	// Err is the wrapped cause (Python `raise ... from ex`).
	Err      error
	sentinel bool
}

// Class returns the Python class name.
func (e *AsusRouterError) Class() string { return e.class }

// Error returns the first argument (or the class name when there is none).
func (e *AsusRouterError) Error() string {
	text := e.class
	switch {
	case len(e.Args) > 0:
		text = fmt.Sprint(e.Args[0])
	case e.Message != nil:
		text = *e.Message
	}
	if e.Err != nil {
		text += ": " + e.Err.Error()
	}
	return text
}

// Unwrap returns the cause.
func (e *AsusRouterError) Unwrap() error { return e.Err }

// Is reports whether target is the sentinel of this class or of one of
// its parent classes (Python `isinstance`).
func (e *AsusRouterError) Is(target error) bool {
	t, ok := target.(*AsusRouterError)
	if !ok || !t.sentinel {
		return false
	}
	return IsSubclass(e.class, t.class)
}

// IsSubclass mirrors Python `issubclass` for the error class names.
func IsSubclass(class, parent string) bool {
	for c := class; c != ""; c = parents[c] {
		if c == parent {
			return true
		}
	}
	return false
}

// New creates an error of the given Python class with the positional
// arguments. Unknown classes panic.
func New(class string, args ...any) *AsusRouterError {
	if _, ok := parents[class]; !ok && class != ClassAsusRouterError {
		panic("arerror: unknown class " + class)
	}
	return &AsusRouterError{class: class, Args: args}
}

func sentinel(class string) *AsusRouterError {
	e := New(class)
	e.sentinel = true
	return e
}

// Sentinels for errors.Is
var (
	ErrAsusRouter        error = sentinel(ClassAsusRouterError)
	Err404               error = sentinel(ClassAsusRouter404Error)
	ErrAccess            error = sentinel(ClassAsusRouterAccessError)
	ErrConnection        error = sentinel(ClassAsusRouterConnectionError)
	ErrFallback          error = sentinel(ClassAsusRouterFallbackError)
	ErrFallbackForbidden error = sentinel(ClassAsusRouterFallbackForbiddenError)
	ErrFallbackLoop      error = sentinel(ClassAsusRouterFallbackLoopError)
	ErrLogout            error = sentinel(ClassAsusRouterLogoutError)
	ErrRequestFormat     error = sentinel(ClassAsusRouterRequestFormatError)
	ErrSSLCertificate    error = sentinel(ClassAsusRouterSSLCertificateError)
	ErrTimeout           error = sentinel(ClassAsusRouterTimeoutError)
)

// NewAsusRouterError mirrors `AsusRouterError(msg)`.
func NewAsusRouterError(msg string) error { return New(ClassAsusRouterError, msg) }

// New404Error mirrors `AsusRouter404Error(msg)`.
func New404Error(msg string) error { return New(ClassAsusRouter404Error, msg) }

// NewAccessError mirrors `AsusRouterAccessError(msg, code, attributes)`;
// `code` is the `ARAccessError` value (`ex.args[1]` in Python).
func NewAccessError(msg string, code int, attrs map[string]any) error {
	return New(ClassAsusRouterAccessError, msg, code, attrs)
}

// NewConnectionError mirrors `AsusRouterConnectionError(msg)`.
func NewConnectionError(msg string) error { return New(ClassAsusRouterConnectionError, msg) }

// NewFallbackError mirrors `AsusRouterFallbackError(msg)`.
func NewFallbackError(msg string) error { return New(ClassAsusRouterFallbackError, msg) }

// NewFallbackForbiddenError mirrors `AsusRouterFallbackForbiddenError(msg)`.
func NewFallbackForbiddenError(msg string) error {
	return New(ClassAsusRouterFallbackForbiddenError, msg)
}

// NewFallbackLoopError mirrors `AsusRouterFallbackLoopError(msg)`.
func NewFallbackLoopError(msg string) error { return New(ClassAsusRouterFallbackLoopError, msg) }

// NewLogoutError mirrors `AsusRouterLogoutError(msg)`.
func NewLogoutError(msg string) error { return New(ClassAsusRouterLogoutError, msg) }

// NewRequestFormatError mirrors `AsusRouterRequestFormatError(msg)`.
func NewRequestFormatError(msg string) error { return New(ClassAsusRouterRequestFormatError, msg) }

// NewSSLCertificateError mirrors `AsusRouterSSLCertificateError(msg)`.
func NewSSLCertificateError(msg string) error {
	return New(ClassAsusRouterSSLCertificateError, msg)
}

// NewTimeoutError mirrors `AsusRouterTimeoutError(msg)`.
func NewTimeoutError(msg string) error { return New(ClassAsusRouterTimeoutError, msg) }

// WithCause sets the cause of an AsusRouter error (Python
// `raise err from cause`) and returns it. Other errors are returned as-is.
func WithCause(err error, cause error) error {
	var e *AsusRouterError
	if errors.As(err, &e) && !e.sentinel {
		e.Err = cause
	}
	return err
}

// AccessCode returns the access error code (`ex.args[1]`).
func AccessCode(err error) (int, bool) {
	var e *AsusRouterError
	if !errors.As(err, &e) || !IsSubclass(e.class, ClassAsusRouterAccessError) || len(e.Args) < 2 {
		return 0, false
	}
	code, ok := e.Args[1].(int)
	return code, ok
}

// AccessAttributes returns the access error attributes (`ex.args[2]`).
func AccessAttributes(err error) map[string]any {
	var e *AsusRouterError
	if !errors.As(err, &e) || !IsSubclass(e.class, ClassAsusRouterAccessError) || len(e.Args) < 3 {
		return nil
	}
	attrs, _ := e.Args[2].(map[string]any)
	return attrs
}

// Name returns the Python class name of an error, for the parity
// harness: the AsusRouter class, "TimeoutError" for deadline errors,
// "" for nil and "Exception" otherwise.
func Name(err error) string {
	if err == nil {
		return ""
	}
	var e *AsusRouterError
	if errors.As(err, &e) {
		return e.class
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return "TimeoutError"
	}
	return "Exception"
}
