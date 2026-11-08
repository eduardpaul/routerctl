package server

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/server/openvpn"
	"github.com/eduardpaul/routerctl/asusrouter/modules/vpn/server/wireguard"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// ARVpnServerAction mirrors `ARVpnServerAction`: enable or disable a VPN
// server unit of a given protocol.
type ARVpnServerAction struct {
	Protocol vpn.ARVpnProtocol
	State    bool
	Unit     int
}

// NewARVpnServerAction mirrors `ARVpnServerAction(protocol, state, *,
// unit=1)`; unit <= 0 uses the default 1.
func NewARVpnServerAction(protocol vpn.ARVpnProtocol, state bool, unit int) *ARVpnServerAction {
	if unit <= 0 {
		unit = 1
	}
	return &ARVpnServerAction{Protocol: protocol, State: state, Unit: unit}
}

// ActionName returns the Python class name.
func (*ARVpnServerAction) ActionName() string { return "ARVpnServerAction" }

// ActionKey mirrors `_key`: key by the target protocol, unit and desired
// state.
func (a *ARVpnServerAction) ActionKey() []any {
	return []any{a.Protocol, a.Unit, a.State}
}

// buildPayload mirrors `_build_payload`: the `(services, arguments)` for
// the action's protocol, or ok=false.
func buildPayload(a *ARVpnServerAction, identity *device.ARDeviceIdentity) ([]service.ARServiceInput, [][2]any, bool) {
	switch a.Protocol {
	case vpn.ARVpnProtocolWireguard:
		services, arguments := wireguard.BuildTogglePayload(a.Unit, a.State)
		return services, arguments, true
	case vpn.ARVpnProtocolOpenvpn:
		services, arguments := openvpn.BuildTogglePayload(a.Unit, a.State, identity)
		return services, arguments, true
	}
	return nil, nil, false
}

// runAction mirrors `run_action`: toggle the action's VPN server on its
// protocol backend.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARVpnServerAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	services, arguments, ok := buildPayload(act, c.Identity)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	return service.AsyncRunService(ctx, c.Read, services, arguments, common.ARActionModeApply, c.FetchRaw)
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARVpnServerAction",
		Run:  runAction,
	})
}
