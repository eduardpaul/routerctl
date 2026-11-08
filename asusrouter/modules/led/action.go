package led

// LED action for AsusRouter.

import (
	"context"
	"log/slog"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/firmware"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	nvramsource "github.com/eduardpaul/routerctl/asusrouter/modules/nvram/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/poll"
)

// start_ctrl_led restarts the LED daemon; nvram led_val reads blank for a
// moment, so poll until the new state reads back
var pollInterval = 500 * time.Millisecond

const pollAttempts = 6

// ARLedAction mirrors `ARLedAction`: turn the device LED on or off.
type ARLedAction struct {
	State bool `json:"state"`
}

// ActionName returns the Python class name.
func (*ARLedAction) ActionName() string { return "ARLedAction" }

// ActionKey keys by the target state.
func (a *ARLedAction) ActionKey() []any { return []any{a.State} }

// asyncWaitSettled polls the LED state until it reports the given state.
func asyncWaitSettled(ctx context.Context, fetchData registry.FetchDataFunc, state bool) (bool, error) {
	probe := func(ctx context.Context) (any, error) {
		return fetchData(ctx, ARLedSourceUniversal, nil)
	}

	ready := func(result any) bool {
		data, ok := result.(map[ARLedField]any)
		if !ok {
			return false
		}
		value, ok := data[ARLedFieldState].(bool)
		return ok && value == state
	}

	settled, err := poll.AsyncPollUntil(ctx, probe, ready, pollInterval, pollAttempts)
	if err != nil {
		return false, err
	}
	return settled != nil, nil
}

// runAction mirrors `run_action`: apply the LED action.
func runAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act, ok := a.(*ARLedAction)
	if !ok {
		return service.ARServiceResult{Success: false}, nil
	}

	value := 0
	if act.State {
		value = 1
	}
	arguments := [][2]any{{string(nvram.ARNvramTypeLed), value}}
	result, err := service.AsyncRunService(
		ctx, c.Read, common.ARServiceLedControl, arguments, common.ARActionModeApply, c.FetchRaw,
	)
	if err != nil {
		return nil, err
	}

	if !result.Success {
		return result, nil
	}

	// Wait until state is settled
	if c.FetchData != nil {
		settled, err := asyncWaitSettled(ctx, c.FetchData, act.State)
		if err != nil {
			return nil, err
		}
		if !settled {
			slog.Debug("LED state did not settle after the switch")
		}
	} else if c.Expire != nil {
		// Without a data callback, drop the stale cache for the next read
		c.Expire(ctx, ARLedSourceUniversal)
		nvramsource.AsyncExpireValues(ctx, c.Expire, LedRequest)
	}

	return result, nil
}

// AsyncRecoverState mirrors `async_recover_state`: reassert a user-set
// LED off-state after a reboot. desired nil is Python None.
//
// Merlin-like firmware turns the LED back on at boot, so an off-state is
// lost on reboot. Only an off-state needs recovery, and only there: the
// fix is a quick on/off toggle, which is what re-applies the off-state.
func AsyncRecoverState(ctx context.Context, runActionCallback action.RunActionFunc, desired *bool, identity *device.ARDeviceIdentity) (bool, error) {
	if desired == nil || *desired {
		return false, nil
	}

	if identity == nil {
		return false, nil
	}
	if _, merlin := firmware.ARFwMerlinLike[identity.Firmware().FirmwareType()]; !merlin {
		return false, nil
	}

	slog.Debug("Recovering the LED off-state after a reboot")
	if _, err := runActionCallback(ctx, &ARLedAction{State: true}); err != nil {
		return false, err
	}
	if _, err := runActionCallback(ctx, &ARLedAction{State: false}); err != nil {
		return false, err
	}
	return true, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{Name: "ARLedAction", Run: runAction})
}
