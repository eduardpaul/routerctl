// Package action mirrors asusrouter/modules/wifi/action.py (split out of
// package wifi to break the identity import cycle).
package action

import (
	"context"
	"strconv"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// ARWiFiAction enables or disables a WiFi radio band.
type ARWiFiAction struct {
	Band  wifi.ARWiFiBand
	State bool
}

// NewARWiFiAction initializes the action with the target band and desired
// state.
func NewARWiFiAction(band wifi.ARWiFiBand, state bool) *ARWiFiAction {
	return &ARWiFiAction{Band: band, State: state}
}

// ActionName returns the Python class name.
func (*ARWiFiAction) ActionName() string { return "ARWiFiAction" }

// ActionKey keys by the target band and desired state.
func (a *ARWiFiAction) ActionKey() []any { return []any{a.Band, a.State} }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// runAction toggles the radio of the action's band via `restart_wireless`.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARWiFiAction)
	if !ok || c.Identity == nil {
		return service.ARServiceResult{Success: false}, nil
	}
	wifiMap := c.Identity.Wifi()
	unit, ok := wifiMap[act.Band]
	if len(wifiMap) == 0 || !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	arguments := [][2]any{{"wl" + strconv.Itoa(unit) + "_radio", boolInt(act.State)}}
	return service.AsyncRunService(
		ctx,
		c.Read,
		common.ARServiceWirelessRestart,
		arguments,
		common.ARActionModeApply,
		c.FetchRaw,
	)
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARWiFiAction",
		Run:  runAction,
	})
}
