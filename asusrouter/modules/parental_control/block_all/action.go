package blockall

import (
	"context"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// ARBlockAllAction mirrors `ARBlockAllAction`: toggle the
// block-all-devices kill switch.
type ARBlockAllAction struct {
	action.Base

	State bool
}

// ActionName returns the Python class name.
func (*ARBlockAllAction) ActionName() string { return "ARBlockAllAction" }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RunAction mirrors `run_action`: apply the block-all toggle via
// `restart_firewall`.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act := a.(*ARBlockAllAction)

	result, err := service.AsyncRunService(
		ctx,
		c.Read,
		common.ARServiceFirewallRestart,
		[][2]any{{string(KeyBlockAll), boolInt(act.State)}},
		common.ARActionModeApply,
		c.FetchRaw,
	)
	if err != nil {
		return nil, err
	}

	// Drop the now-stale cached switch so the next read refetches
	if result.Success && c.Expire != nil {
		c.Expire(ctx, ARBlockAllSourceUniversal)
		nvramsource.AsyncExpireValues(ctx, c.Expire, KeyBlockAll)
	}

	return result, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARBlockAllAction",
		Run:  RunAction,
	})
}
