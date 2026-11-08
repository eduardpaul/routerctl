// Package consts ports `asusrouter/const.py` (`const` is a Go keyword).
package consts

import (
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/enum"
)

// Version information
const (
	MajorVersion = 2
	MinorVersion = 0
	PatchVersion = "0.dev0"
	// Version mirrors `__version__`.
	Version = "2.0.0.dev0"
)

// Unknown member constants
const (
	UnknownMember    = -999
	UnknownMemberStr = "unknown"
)

// NaiveLocation marks a `time.Time` standing for a Python naive datetime.
// It is the same value as `converters.NaiveLocation` (the converters
// cannot import this package without a cycle).
var NaiveLocation = converters.NaiveLocation

// Enums

// RequestType mirrors `RequestType` (StrEnum).
// Request type enum.
type RequestType string

const (
	RequestTypeGet  RequestType = "get"
	RequestTypePost RequestType = "post"
)

// RequestTypeMembers lists the members in definition order.
var RequestTypeMembers = []RequestType{RequestTypeGet, RequestTypePost}

var requestTypeNames = map[RequestType]string{
	RequestTypeGet:  "GET",
	RequestTypePost: "POST",
}

// Name returns the Python member name.
func (e RequestType) Name() string { return requestTypeNames[e] }

// Valid reports whether e is a defined member.
func (e RequestType) Valid() bool { _, ok := requestTypeNames[e]; return ok }

// HTTPStatus mirrors `HTTPStatus` (FromIntMixin, IntEnum).
// HTTP status codes.
type HTTPStatus int

const (
	HTTPStatusUnknown HTTPStatus = UnknownMember

	HTTPStatusOk                  HTTPStatus = 200
	HTTPStatusCreated             HTTPStatus = 201
	HTTPStatusAccepted            HTTPStatus = 202
	HTTPStatusNoContent           HTTPStatus = 204
	HTTPStatusBadRequest          HTTPStatus = 400
	HTTPStatusUnauthorized        HTTPStatus = 401
	HTTPStatusForbidden           HTTPStatus = 403
	HTTPStatusNotFound            HTTPStatus = 404
	HTTPStatusMethodNotAllowed    HTTPStatus = 405
	HTTPStatusConflict            HTTPStatus = 409
	HTTPStatusInternalServerError HTTPStatus = 500
	HTTPStatusJsonBadFormat       HTTPStatus = 4002
	HTTPStatusJsonBadRequest      HTTPStatus = 4003
)

// HTTPStatusMembers lists the members in definition order.
var HTTPStatusMembers = []HTTPStatus{
	HTTPStatusUnknown,
	HTTPStatusOk,
	HTTPStatusCreated,
	HTTPStatusAccepted,
	HTTPStatusNoContent,
	HTTPStatusBadRequest,
	HTTPStatusUnauthorized,
	HTTPStatusForbidden,
	HTTPStatusNotFound,
	HTTPStatusMethodNotAllowed,
	HTTPStatusConflict,
	HTTPStatusInternalServerError,
	HTTPStatusJsonBadFormat,
	HTTPStatusJsonBadRequest,
}

var hTTPStatusByName = map[string]HTTPStatus{
	"UNKNOWN":               HTTPStatusUnknown,
	"OK":                    HTTPStatusOk,
	"CREATED":               HTTPStatusCreated,
	"ACCEPTED":              HTTPStatusAccepted,
	"NO_CONTENT":            HTTPStatusNoContent,
	"BAD_REQUEST":           HTTPStatusBadRequest,
	"UNAUTHORIZED":          HTTPStatusUnauthorized,
	"FORBIDDEN":             HTTPStatusForbidden,
	"NOT_FOUND":             HTTPStatusNotFound,
	"METHOD_NOT_ALLOWED":    HTTPStatusMethodNotAllowed,
	"CONFLICT":              HTTPStatusConflict,
	"INTERNAL_SERVER_ERROR": HTTPStatusInternalServerError,
	"JSON_BAD_FORMAT":       HTTPStatusJsonBadFormat,
	"JSON_BAD_REQUEST":      HTTPStatusJsonBadRequest,
}

var hTTPStatusNames = func() map[HTTPStatus]string {
	names := make(map[HTTPStatus]string, len(hTTPStatusByName))
	for name, member := range hTTPStatusByName {
		names[member] = name
	}
	return names
}()

// Name returns the Python member name.
func (e HTTPStatus) Name() string { return hTTPStatusNames[e] }

// Valid reports whether e is a defined member.
func (e HTTPStatus) Valid() bool { _, ok := hTTPStatusNames[e]; return ok }

// HTTPStatusFromValueOK mirrors `HTTPStatus.from_value` without the
// UNKNOWN fallback.
func HTTPStatusFromValueOK(value any) (HTTPStatus, bool) {
	return enum.FromIntOK(value, HTTPStatusMembers, hTTPStatusByName)
}

// HTTPStatusFromValue mirrors `HTTPStatus.from_value`: unresolved values
// map to HTTPStatusUnknown.
func HTTPStatusFromValue(value any) HTTPStatus {
	return enum.FromInt(value, HTTPStatusMembers, hTTPStatusByName, HTTPStatusUnknown)
}

// Asus constants
const (
	UserAgent        = "asusrouter-library-DUTUtil-" + Version
	DefaultPortHTTP  = 80
	DefaultPortHTTPS = 8443
)

// AsusRouter definitions
const (
	ARCallFetchState      = "fetch_state"
	ARCallProbeState      = "async_probe_state"
	ARCallRunAction       = "run_action"
	ARCallTranslateAction = "translate_action"
	ARCallTranslateState  = "translate_state"
)

// Library defaults
const (
	DefaultCacheTime       = 2 * time.Second
	DefaultTimeout         = 15 * time.Second
	DefaultTimeoutFallback = 5 * time.Second
)

// Other defaults
const DefaultIdentityBrand = "ASUSTek"
