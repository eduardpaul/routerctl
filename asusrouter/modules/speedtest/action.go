package speedtest

import (
	"context"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// now is the clock of the start-time stamp (injectable for tests)
var now = time.Now

// ARSpeedTestAction mirrors `ARSpeedTestAction`: run a speedtest.
type ARSpeedTestAction struct {
	ServerID *string
	Iface    *string
}

// NewARSpeedTestAction initializes the action with an optional server
// (int or str) and interface.
func NewARSpeedTestAction(serverID any, iface *string) *ARSpeedTestAction {
	return &ARSpeedTestAction{ServerID: NormalizeServerID(serverID), Iface: iface}
}

// ActionName returns the Python class name.
func (*ARSpeedTestAction) ActionName() string { return "ARSpeedTestAction" }

// ActionKey keys by the chosen server and interface.
func (a *ARSpeedTestAction) ActionKey() []any { return []any{deref(a.ServerID), deref(a.Iface)} }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// post sends a request raw when possible: async_fetch returns nil on
// failure and the (ignored) body on success, while async_read would
// collapse a failed fetch to {} and hide it. It reports whether the post
// returned anything.
func post(ctx context.Context, c *registry.Context, ep endpoint.AREndpoint, request string) (bool, error) {
	if c.FetchRaw != nil {
		data, err := c.FetchRaw(ctx, ep, request)
		return data != nil, err
	}
	data, err := c.Read(ctx, ep, request)
	return data != nil, err
}

// RunAction mirrors `run_action`: trigger a speedtest run.
func RunAction(ctx context.Context, c *registry.Context, a action.ARAction) (any, error) {
	act := a.(*ARSpeedTestAction)

	// The start time is only stored for the WebUI display; result not checked
	if _, err := post(ctx, c, endpoint.AREndpointSetSpeedtestStartTime, BuildStartTimeRequest(now().UnixMilli())); err != nil {
		return nil, err
	}
	ok, err := post(ctx, c, endpoint.AREndpointRunSpeedtest, BuildRunRequest(ExeTypeRun, act.ServerID, act.Iface))
	if err != nil {
		return nil, err
	}
	return service.ARServiceResult{Success: ok}, nil
}

func init() {
	registry.RegisterAction(registry.ActionEntry{
		Name: "ARSpeedTestAction",
		Run:  RunAction,
	})
}
