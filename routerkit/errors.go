package routerkit

import (
	"context"
	"errors"
	"net"
)

// The error taxonomy of the contract. Drivers return (or wrap, see
// Classify) these so callers can tell failures apart with errors.Is,
// whatever the brand.
var (
	// ErrUnsupported: the device (or this driver) cannot provide a part,
	// an action or a source
	ErrUnsupported = errors.New("not supported by this device")
	// ErrAuth: the device refused the credentials
	ErrAuth = errors.New("authentication failed")
	// ErrUnreachable: no answer from the device (refused, timed out, DNS)
	ErrUnreachable = errors.New("device unreachable")
	// ErrBusy: the device's single session is held elsewhere
	ErrBusy = errors.New("device busy (session held elsewhere)")
	// ErrLockedOut: the device locked the login after failed attempts
	ErrLockedOut = errors.New("login locked out")
	// ErrInvalidArgs: action arguments do not match the action spec
	ErrInvalidArgs = errors.New("invalid action arguments")
	// ErrNotFound: unknown source or action name
	ErrNotFound = errors.New("unknown source or action")
	// ErrClosed: the driver was closed; Connect opens it again
	ErrClosed = errors.New("driver closed (connect again)")
)

// classified keeps the message of err and adds a contract error kind.
type classified struct{ err, kind error }

func (c *classified) Error() string   { return c.err.Error() }
func (c *classified) Unwrap() []error { return []error{c.err, c.kind} }

// Classify returns err with kind (one of the Err* values above) added for
// errors.Is, keeping err's message and its own chain. It returns nil for
// a nil err and err itself when it already is of that kind.
func Classify(err, kind error) error {
	if err == nil || kind == nil || errors.Is(err, kind) {
		return err
	}
	return &classified{err: err, kind: kind}
}

// kinds are the error kinds of the taxonomy, most specific first.
var kinds = []error{ErrClosed, ErrInvalidArgs, ErrNotFound, ErrUnsupported, ErrLockedOut, ErrAuth, ErrBusy, ErrUnreachable}

// KindOf returns the taxonomy error (ErrAuth, ErrUnreachable, ...) err
// is, or nil: to re-classify a new error built from err's message.
func KindOf(err error) error {
	for _, k := range kinds {
		if errors.Is(err, k) {
			return k
		}
	}
	if IsNetwork(err) {
		return ErrUnreachable
	}
	return nil
}

// IsNetwork reports whether err is a transport failure below the
// protocol: refused or reset connections, DNS errors, dial or read
// timeouts. Drivers classify these as ErrUnreachable.
func IsNetwork(err error) bool {
	var oe *net.OpError
	var de *net.DNSError
	if errors.As(err, &oe) || errors.As(err, &de) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// ErrCode is the JSON-friendly kind of a part error.
type ErrCode string

// Part error codes.
const (
	CodeUnsupported ErrCode = "unsupported"
	CodeAuth        ErrCode = "auth"
	CodeUnreachable ErrCode = "unreachable"
	CodeTimeout     ErrCode = "timeout"
	CodeBusy        ErrCode = "busy"
	CodeParse       ErrCode = "parse"
	CodeDevice      ErrCode = "device"
)

// CodeOf maps an error to its code (CodeDevice when nothing more
// specific applies).
func CodeOf(err error) ErrCode {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnsupported), errors.Is(err, ErrNotFound):
		return CodeUnsupported
	case errors.Is(err, ErrAuth), errors.Is(err, ErrLockedOut):
		return CodeAuth
	case errors.Is(err, ErrBusy):
		return CodeBusy
	case errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout
	case errors.Is(err, ErrUnreachable), IsNetwork(err):
		return CodeUnreachable
	}
	return CodeDevice
}

// PartError is the failure of one part of a poll.
type PartError struct {
	Code    ErrCode `json:"code"`
	Message string  `json:"message"`
}

// NewPartError builds the PartError of err.
func NewPartError(err error) PartError {
	if err == nil {
		return PartError{}
	}
	return PartError{Code: CodeOf(err), Message: err.Error()}
}

// Error implements error.
func (e PartError) Error() string { return e.Message }
