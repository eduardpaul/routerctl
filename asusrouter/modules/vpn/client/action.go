package client

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client/classic"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/client/fusion"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// ARVpnClientAction mirrors `ARVpnClientAction`: enable or disable a VPN
// client of a given protocol and unit.
//
// `Unit` is the per-protocol client unit (`wgc{unit}` / `vpn_client
// {unit}`), the same value the read source reports as `SERVER`.
type ARVpnClientAction struct {
	Protocol vpn.ARVpnProtocol
	State    bool
	Unit     int
}

// NewARVpnClientAction mirrors `ARVpnClientAction(protocol, state, *,
// unit=1)`; unit <= 0 uses the default 1.
func NewARVpnClientAction(protocol vpn.ARVpnProtocol, state bool, unit int) *ARVpnClientAction {
	if unit <= 0 {
		unit = 1
	}
	return &ARVpnClientAction{Protocol: protocol, State: state, Unit: unit}
}

// ActionName returns the Python class name.
func (*ARVpnClientAction) ActionName() string { return "ARVpnClientAction" }

// ActionKey mirrors `_key`: key by the target protocol, unit and desired
// state.
func (a *ARVpnClientAction) ActionKey() []any {
	return []any{a.Protocol, a.Unit, a.State}
}

// runAction mirrors `run_action`: toggle the action's VPN client on
// whichever backend the device uses.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARVpnClientAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	// The clientlist decides the backend; without it no safe push is possible
	if c.FetchDataMulti == nil {
		return service.ARServiceResult{Success: false}, nil
	}

	// A None clientlist means a non-Fusion device
	value, err := nvramsource.AsyncGetValue(ctx, c, nvram.ARNvramTypeVpncClientlist, nil)
	if err != nil {
		return nil, err
	}
	clientlist := converters.RawToStr(value)
	var (
		services  []service.ARServiceInput
		arguments [][2]any
	)
	if clientlist != nil {
		services, arguments, ok = fusion.BuildTogglePayload(
			*clientlist, act.Protocol, act.Unit, act.State,
		)
	} else {
		services, arguments, ok = classic.BuildTogglePayload(
			act.Protocol, act.Unit, act.State,
		)
	}

	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	return service.AsyncRunService(ctx, c.Read, services, arguments, common.ARActionModeApply, c.FetchRaw)
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARVpnClientAction",
		Run:  runAction,
	})
}
