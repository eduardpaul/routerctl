// Network action for AsusRouter.

package network

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// FindHandleBySsid mirrors `find_handle_by_ssid`: return the handle of the
// network matching `ssid` from read data. `ssid` is an `identifiers.Ssid`
// (value or pointer) or a plain string.
//
// Python walks the dict in insertion order; Go walks the network types in
// their enum definition order so the first match is deterministic.
func FindHandleBySsid(networks map[ARNetworkType][]map[ARNetworkField]any, ssid any) *ARNetworkHandle {
	want, ok := ssidText(ssid)
	if !ok {
		return nil
	}
	for _, netType := range ARNetworkTypeMembers {
		for _, profile := range networks[netType] {
			if got, ok := ssidText(profile[ARNetworkFieldSsid]); !ok || got != want {
				continue
			}
			switch handle := profile[ARNetworkFieldHandle].(type) {
			case ARNetworkHandle:
				return &handle
			case *ARNetworkHandle:
				if handle != nil {
					return handle
				}
			}
		}
	}
	return nil
}

// ssidText reads the raw text of an SSID-like value (`Ssid.__eq__` compares
// the raw value with another SSID or a plain string).
func ssidText(value any) (string, bool) {
	switch v := value.(type) {
	case nil:
		return "", false
	case string:
		return v, true
	case interface{ CanonicalValue() any }:
		s, ok := v.CanonicalValue().(string)
		return s, ok
	}
	return "", false
}

// ARNetworkAction mirrors `ARNetworkAction`: enable or disable a network
// profile.
type ARNetworkAction struct {
	Handle ARNetworkHandle
	State  bool
}

// NewARNetworkAction mirrors `ARNetworkAction.__init__`.
func NewARNetworkAction(handle ARNetworkHandle, state bool) *ARNetworkAction {
	return &ARNetworkAction{Handle: handle, State: state}
}

// ActionName returns the Python class name.
func (*ARNetworkAction) ActionName() string { return "ARNetworkAction" }

// ActionKey mirrors `_key`: key by the target profile and desired state.
func (a *ARNetworkAction) ActionKey() []any { return []any{a.Handle, a.State} }

// buildPayload mirrors `_build_payload`: build the `(rc_service,
// arguments)` for the backend; ok is false for Python `None`.
func buildPayload(
	ctx context.Context,
	c *registry.Context,
	handle ARNetworkHandle,
	state bool,
) (string, [][2]any, bool, error) {
	if handle.Backend == ARNetworkBackendSdn {
		rawSdnRl, err := nvramsource.AsyncGetValue(ctx, c, nvram.ARNvramTypeSdnRl, nil)
		if err != nil {
			return "", nil, false, err
		}
		if rawSdnRl == nil {
			return "", nil, false, nil
		}
		rcService, arguments := SdnBuildTogglePayload(rawSdnRl, handle, state)
		return rcService, arguments, true, nil
	}

	rcService, arguments := LegacyBuildTogglePayload(handle, state)
	return rcService, arguments, true, nil
}

// runAction mirrors `run_action`: toggle the action's network on its
// backend.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARNetworkAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	rcService, arguments, ok, err := buildPayload(ctx, c, act.Handle, act.State)
	if err != nil {
		return nil, err
	}
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	return service.AsyncRunService(
		ctx,
		c.Read,
		rcService,
		arguments,
		common.ARActionModeApply,
		c.FetchRaw,
	)
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARNetworkAction",
		Run:  runAction,
	})
}
