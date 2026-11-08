// Package servers mirrors asusrouter/modules/speedtest/servers/.
package servers

import (
	"context"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/poll"
)

// The router populates the server list asynchronously after the trigger,
// so poll the hook until it returns a usable list
const (
	minServers   = 2
	pollAttempts = 15
)

// pollInterval is a variable so tests can shorten it
var pollInterval = 1 * time.Second

var serversRequest = endpoint.HookRequest(endpoint.ARHookOoklaSpeedtestServers)

// Sources and actions

// ARSpeedTestServersSource mirrors `ARSpeedTestServersSource`: the
// AsusRouter speedtest servers data source.
type ARSpeedTestServersSource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARSpeedTestServersSource) SourceName() string { return "ARSpeedTestServersSource" }

// ARSpeedTestServersSourceUniversal is the universal instance - preferred.
var ARSpeedTestServersSourceUniversal = &ARSpeedTestServersSource{}

// Fetch

func asyncFetch(ctx context.Context, read registry.ReadFunc) (any, error) {
	// Fetch the raw server list from the hook
	data, err := read(ctx, endpoint.AREndpointFetchData, serversRequest)
	if err != nil {
		return nil, err
	}
	return endpoint.HookValue(data, endpoint.ARHookOoklaSpeedtestServers), nil
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the speedtest server list; with `refresh`, ask for a fresh one
	if c.Identity == nil || !support.SupportAvailable(c.Identity.Support(), support.ARSupportTypeSpeedtest) {
		return map[string]any{}, nil
	}

	if c.Kwargs.Bool("refresh") {
		// Ask the router to rebuild the list, then wait for it to fill
		if _, err := c.Read(ctx, endpoint.AREndpointRunSpeedtest, speedtest.BuildRunRequest(speedtest.ExeTypeList, nil, nil)); err != nil {
			return nil, err
		}
		fresh, err := poll.AsyncPollUntil(
			ctx,
			func(ctx context.Context) (any, error) { return asyncFetch(ctx, c.Read) },
			func(value any) bool {
				list, ok := value.([]any)
				return ok && len(list) >= minServers
			},
			pollInterval,
			pollAttempts,
		)
		if err != nil {
			return nil, err
		}
		if fresh == nil {
			return map[string]any{}, nil
		}
		return *fresh, nil
	}

	servers, err := asyncFetch(ctx, c.Read)
	if err != nil {
		return nil, err
	}
	if servers == nil {
		return map[string]any{}, nil
	}
	return servers, nil
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate the raw server list into servers
	return speedtest.ServersFromList(raw)
}

// Registration

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSpeedTestServersSource",
		Default:   func() source.DataSource { return &ARSpeedTestServersSource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
