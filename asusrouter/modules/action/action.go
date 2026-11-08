// Package action mirrors asusrouter/modules/action/__init__.py.
package action

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// The router needs a moment to spin up a run after the trigger returns;
// reading sooner still sees the previous run and stale/absent results
const runStartDelay = 1 * time.Second

// ARAction is the Go form of `ARAction`: an action to run on the device.
//
// ActionName is the Python class name; ActionKey mirrors `_key()` (nil for
// router-global actions). Actions are equal by exact type and key.
type ARAction interface {
	ActionName() string
	ActionKey() []any
}

// Base is embedded by router-global actions (empty `_key`).
type Base struct{}

// ActionKey returns the empty key of a router-global action.
func (Base) ActionKey() []any { return nil }

// Repr mirrors `ARAction.__repr__`.
func Repr(a ARAction) string {
	key := a.ActionKey()
	if len(key) == 0 {
		return "<" + a.ActionName() + ">"
	}
	parts := make([]string, len(key))
	for i, k := range key {
		parts[i] = fmt.Sprintf("%v", k)
	}
	return fmt.Sprintf("<%s (%s)>", a.ActionName(), strings.Join(parts, ", "))
}

// RunActionFunc is the `run_action_callback` handed to fetchers/actions.
type RunActionFunc func(ctx context.Context, a ARAction) (any, error)

// AsyncStartRun mirrors `async_start_run`: trigger an action run and give
// the device time to start it. delay <= 0 uses the default.
func AsyncStartRun(ctx context.Context, run RunActionFunc, a ARAction, delay time.Duration) (bool, error) {
	if run == nil {
		return false, nil
	}
	result, err := run(ctx, a)
	if err != nil {
		return false, err
	}
	if !truthy(result) {
		slog.Debug("action did not start; dropping results", "action", Repr(a))
		return false, nil
	}
	if delay <= 0 {
		delay = runStartDelay
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-time.After(delay):
	}
	return true, nil
}

// truthy mirrors Python truthiness for action results.
func truthy(v any) bool {
	// Result types with Python __bool__ implement Bool()
	if b, ok := v.(interface{ Bool() bool }); ok {
		return b.Bool()
	}
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case *bool:
		return t != nil && *t
	case int:
		return t != 0
	case string:
		return t != ""
	case map[string]any:
		return len(t) > 0
	case []any:
		return len(t) > 0
	}
	return true
}
