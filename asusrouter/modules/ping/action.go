// Package ping mirrors asusrouter/modules/ping/.
package ping

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// ARPingAction mirrors `ARPingAction`: run a DNS ping against the
// configured targets.
type ARPingAction struct {
	action.Base
}

// ActionName returns the Python class name.
func (*ARPingAction) ActionName() string { return "ARPingAction" }

// RunAction mirrors `run_action`: run a DNS ping against the configured
// targets.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	data, err := c.Read(ctx, endpoint.AREndpointRunPing, "")
	if err != nil {
		return nil, err
	}
	var code any
	if m, ok := data.(map[string]any); ok {
		code = m[common.StatusCodeKey]
	}
	return service.ARServiceResult{
		Success: common.ARStatusCodeFromValue(code) == common.ARStatusCodeSuccess,
	}, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARPingAction",
		Run:  RunAction,
	})
}
