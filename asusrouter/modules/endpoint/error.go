package endpoint

import (
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// HandleAccessError mirrors `handle_access_error`. It returns nil on
// success and the matching AsusRouter error otherwise.
func HandleAccessError(e AREndpoint, status int, content string) error {
	// Read the page as json
	message := readers.ReadJSONContent(content)

	// Get error code
	errorStatus := consts.UnknownMember
	if v := converters.RawToInt(message["error_status"]); v != nil && *v != 0 {
		errorStatus = *v
	}
	// Formatting errors
	if errorStatus == int(consts.HTTPStatusJsonBadFormat) || errorStatus == int(consts.HTTPStatusJsonBadRequest) {
		return arerror.NewRequestFormatError("JSON format error")
	}

	accessErr, ok := ARAccessErrorFromValueOK(errorStatus)
	if !ok {
		accessErr = ARAccessErrorUnknown
	}

	// Success
	if accessErr == ARAccessErrorSuccess {
		return nil
	}

	slog.Debug("Access error message", "message", message)

	// Additional values
	attributes := map[string]any{}

	// Handle logout code (even though it's not an error)
	if accessErr == ARAccessErrorLogout {
		return arerror.NewLogoutError("Session is logged out")
	}

	// Try again later error
	if accessErr == ARAccessErrorTryAgain {
		if timeout := converters.RawToInt(message["remaining_lock_time"]); timeout != nil && *timeout != 0 {
			attributes["timeout"] = *timeout
		}
	}

	return arerror.NewAccessError("Access error", int(accessErr), attributes)
}
