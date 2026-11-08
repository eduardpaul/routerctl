package asusrouter

// Identity wiring of asusrouter.py: `_commit_data_state`, `_sync_clock`,
// LED reassertion after a reboot and the dump recorder hook.

import (
	"context"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/config"
	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	aimeshsource "github.com/eduardpaul/routerctl/asusrouter/modules/aimesh/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/clock"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	devicesource "github.com/eduardpaul/routerctl/asusrouter/modules/device/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/led"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/tools/dump"
)

func deviceSourceUniversal() source.DataSource { return devicesource.ARDeviceSourceUniversal }
func aimeshSourceUniversal() source.DataSource { return aimeshsource.ARAiMeshSourceUniversal }
func clockSourceUniversal() source.DataSource  { return clock.ARClockSourceUniversal }

// seededBoottime reads the `BOOTTIME` instance config (a seeded boot time).
func seededBoottime(cfg *config.ARInstanceConfig) *time.Time {
	return config.SafeBoottimeConfig(cfg.Get(config.ARConfigKeyBoottime))
}

// syncIdentity mirrors the identity part of `_commit_data_state`: keep
// the live AiMesh topology and clock in sync.
func syncIdentity(identity *device.ARDeviceIdentity, src source.DataSource, value any) {
	switch v := value.(type) {
	case *aimesh.ARAiMeshTopology:
		if v != nil {
			identity.UpdateAimesh(v)
		}
		return
	case aimesh.ARAiMeshTopology:
		identity.UpdateAimesh(&v)
		return
	}
	if _, isClock := src.(*clock.ARClockSource); !isClock {
		return
	}
	fields, ok := value.(map[clock.ARClockField]any)
	if !ok {
		return
	}
	syncClock(identity, fields)
}

// syncClock mirrors `_sync_clock`.
func syncClock(identity *device.ARDeviceIdentity, value map[clock.ARClockField]any) {
	switch b := value[clock.ARClockFieldBoottime].(type) {
	case *clock.ARBoottime:
		if b != nil {
			t := b.Time
			identity.UpdateBoottime(&t)
		}
	case clock.ARBoottime:
		t := b.Time
		identity.UpdateBoottime(&t)
	}

	// `bool` is an `int`; only a real count says anything here
	switch u := value[clock.ARClockFieldUptime].(type) {
	case int:
		identity.UpdateUptime(&u)
	case *int:
		if u != nil {
			identity.UpdateUptime(u)
		}
	}

	var deviceTime *time.Time
	switch d := value[clock.ARClockFieldDeviceTime].(type) {
	case time.Time:
		deviceTime = &d
	case *time.Time:
		deviceTime = d
	}
	if deviceTime != nil && deviceTime.Location() != consts.NaiveLocation {
		identity.UpdateDeviceTime(deviceTime)
	}
}

// ledActionState returns the target state of an LED action.
func ledActionState(a action.ARAction) (bool, bool) {
	if l, ok := a.(*led.ARLedAction); ok && l != nil {
		return l.State, true
	}
	return false, false
}

// recoverLedState mirrors `async_recover_state` (Merlin turns the LED back
// on after a reboot; reassert the last commanded state).
func recoverLedState(ctx context.Context, r *AsusRouter, desired *bool) {
	if desired == nil {
		return
	}
	_, _ = led.AsyncRecoverState(ctx, func(ctx context.Context, a action.ARAction) (any, error) {
		return r.RunAction(ctx, a, nil)
	}, desired, r.Description())
}

// recordDump mirrors the `active_recorder()` hook in `async_fetch`.
func recordDump(ctx context.Context, ep endpoint.AREndpoint, rt consts.RequestType, payload *string, content string) {
	rec := dump.ActiveRecorder(ctx)
	if rec == nil {
		return
	}
	c := content
	rec.Record(ep, rt, payload, &c)
}
