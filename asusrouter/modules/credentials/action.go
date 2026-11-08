package credentials

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/arerror"
	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ARCredentialsAction mirrors `ARCredentialsAction`: change the router
// login username and/or password.
//
// Python keeps ARAction's field-free repr, so credential values never
// reach logs; String/GoString/LogValue do the same here. Equality is by
// type only (empty key, no secrets in the key).
type ARCredentialsAction struct {
	action.Base

	NewUsername *string
	NewPassword *string
}

// ActionName returns the Python class name.
func (*ARCredentialsAction) ActionName() string { return "ARCredentialsAction" }

// String mirrors the field-free `__repr__`.
func (a *ARCredentialsAction) String() string { return action.Repr(a) }

// GoString keeps `%#v` from printing credential values.
func (a *ARCredentialsAction) GoString() string { return action.Repr(a) }

// LogValue keeps slog from printing credential values.
func (a *ARCredentialsAction) LogValue() slog.Value { return slog.StringValue(action.Repr(a)) }

// capability mirrors `_capability`: read one advertised login capability
// off the identity.
func capability(identity *device.ARDeviceIdentity, cap ARCredentialsCapability) any {
	if identity == nil {
		return nil
	}
	return support.SupportCapability(
		identity.Support(),
		support.ARSupportTypeCredentialsCapabilities,
		cap,
	)
}

// isTrue mirrors Python `value is True`.
func isTrue(value any) bool {
	b, ok := value.(bool)
	return ok && b
}

// policyFromIdentity mirrors `_policy_from_identity`: derive the
// credential rules from the advertised capabilities.
func policyFromIdentity(identity *device.ARDeviceIdentity) CredentialsPolicy {
	usernameMax := converters.RawToInt(
		capability(identity, ARCredentialsCapabilityUsernameMaxLength),
	)
	passwordMax := converters.RawToInt(
		capability(identity, ARCredentialsCapabilityPasswordMaxLength),
	)

	policy := CredentialsPolicy{
		UsernameMax: DefaultMaxLength,
		PasswordMax: DefaultMaxLength,
		Strict:      isTrue(capability(identity, ARCredentialsCapabilitySecureDefault)),
	}
	if usernameMax != nil && *usernameMax != 0 {
		policy.UsernameMax = *usernameMax
	}
	if passwordMax != nil && *passwordMax != 0 {
		policy.PasswordMax = *passwordMax
	}
	return policy
}

// truthy mirrors Python truthiness for a poster response.
func truthy(raw any) bool {
	switch v := raw.(type) {
	case nil:
		return false
	case *string:
		return v != nil && *v != ""
	case string:
		return v != ""
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	case bool:
		return v
	}
	return true
}

// changeAccepted mirrors `_change_accepted`: whether the device accepted
// the change, per backend.
func changeAccepted(useChpass bool, raw any) bool {
	if !useChpass {
		// Should return its page on success; empty/None means failure
		if !truthy(raw) {
			slog.Warn("Login change failed: no response from the device")
			return false
		}
		return true
	}

	status := ReadChpassResult(raw)
	switch status {
	case ARCredentialsStatusSuccess:
		return true
	case ARCredentialsStatusWrongPassword:
		slog.Warn("Login change rejected: wrong current password")
	case ARCredentialsStatusLockedOut:
		slog.Warn("Login change blocked: too many failed attempts")
	default:
		slog.Debug("Login change failed", "status", status)
	}
	return false
}

// or mirrors Python `a or b` for an optional string.
func or(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}

// runAction mirrors `run_action`: change the router login
// username/password.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARCredentialsAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	identity := c.Identity

	if c.CredentialsGet == nil {
		slog.Debug("No credentials getter available; cannot change login")
		return service.ARServiceResult{Success: false}, nil
	}

	if act.NewUsername == nil && act.NewPassword == nil {
		slog.Debug("No new credential value provided; nothing to change")
		return service.ARServiceResult{Success: false}, nil
	}

	curUsername, curPassword := c.CredentialsGet()
	// The credentials the session will use once the change lands
	newUsername := or(act.NewUsername, curUsername)
	newPassword := or(act.NewPassword, curPassword)

	// Reject changes the device is known to refuse, before sending them
	policy := policyFromIdentity(identity)
	if err := ValidateCredentials(
		curUsername,
		curPassword,
		act.NewUsername,
		act.NewPassword,
		&policy,
	); err != nil {
		slog.Warn("Login change rejected: " + *err)
		return service.ARServiceResult{Success: false}, nil
	}

	useChpass := isTrue(capability(identity, ARCredentialsCapabilityChpass))
	var ep endpoint.AREndpoint
	var request string
	if useChpass {
		// New FW
		ep = endpoint.AREndpointChpass
		var err error
		request, err = BuildChpassRequest(
			curUsername,
			curPassword,
			act.NewUsername,
			act.NewPassword,
		)
		if err != nil {
			return nil, err
		}
	} else {
		// Legacy
		ep = endpoint.AREndpointStartApply
		request = BuildLegacyRequest(newUsername, newPassword)
	}

	var raw any
	var err error
	if c.FetchRaw != nil {
		raw, err = c.FetchRaw(ctx, ep, request)
	} else {
		raw, err = c.Read(ctx, ep, request)
	}
	if err != nil {
		if errors.Is(err, arerror.ErrConnection) || errors.Is(err, arerror.ErrTimeout) {
			slog.Warn("Login change could not be confirmed", "error", err)
			return service.ARServiceResult{Success: false}, nil
		}
		return nil, err
	}

	if !changeAccepted(useChpass, raw) {
		return service.ARServiceResult{Success: false}, nil
	}

	// Apply new credentials to our connection
	if c.CredentialsSet != nil {
		if _, err := c.CredentialsSet(ctx, newUsername, newPassword); err != nil {
			return nil, err
		}
	} else {
		slog.Warn(
			"Login changed on the device but the session was not resynced; " +
				"reconnect with the new credentials",
		)
	}

	return service.ARServiceResult{Success: true}, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARCredentialsAction",
		Run:  runAction,
	})
}
