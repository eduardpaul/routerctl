// Package source mirrors asusrouter/modules/speedtest/source.py: the
// `ARSpeedTestSource` data source.
//
// It is a subpackage (not part of `speedtest`) because it imports the
// `speedtest/history` subpackage, which imports `speedtest` itself.
package source

import (
	"context"
	"log/slog"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	arsource "github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest/history"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/poll"
)

// ARSpeedTestSource mirrors `ARSpeedTestSource`: the AsusRouter speedtest
// data source.
type ARSpeedTestSource struct {
	ServerID *string
	Iface    *string
}

// NewARSpeedTestSource initializes the source with an optional server
// (int or str) and interface.
func NewARSpeedTestSource(serverID any, iface *string) *ARSpeedTestSource {
	return &ARSpeedTestSource{ServerID: speedtest.NormalizeServerID(serverID), Iface: iface}
}

// SourceName returns the Python class name.
func (*ARSpeedTestSource) SourceName() string { return "ARSpeedTestSource" }

// SourceKey keys by the chosen server and interface.
func (s *ARSpeedTestSource) SourceKey() []any { return []any{deref(s.ServerID), deref(s.Iface)} }

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// ARSpeedTestSourceUniversal is the universal instance (auto server, WAN)
// - preferred for a default run.
var ARSpeedTestSourceUniversal = NewARSpeedTestSource(nil, nil)

// Fetch

// Wait out a run before reading; the WebUI caps a test at 120s
const pollAttempts = 90

// pollInterval is a variable so tests can shorten it
var pollInterval = 2 * time.Second

// startDelay is the AsyncStartRun delay (<= 0 uses the default)
var startDelay time.Duration

var resultRequest = endpoint.HookRequest(endpoint.ARHookOoklaSpeedtestResult)

func asyncFetchEvents(ctx context.Context, read registry.ReadFunc) (any, error) {
	// Fetch the raw Ookla result stream
	data, err := read(ctx, endpoint.AREndpointFetchData, resultRequest)
	if err != nil {
		return nil, err
	}
	return endpoint.HookValue(data, endpoint.ARHookOoklaSpeedtestResult), nil
}

func resultID(events any) any {
	// Return the id of the final result row, or nil if there is none yet
	raw := speedtest.RawResultFromEvents(events)
	result, ok := raw["result"].(map[string]any)
	if !ok {
		return nil
	}
	return result["id"]
}

func logStream(ctx context.Context, events any) {
	// Log the stream length and the last typed row, for live diagnosis

	// Skip the stream walk entirely when debug logging is off
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}

	list, ok := events.([]any)
	if !ok {
		// Do not dump the payload - its contents are unknown and may be
		// sensitive; the type alone is enough to diagnose
		slog.Debug("Speedtest stream: not a list", "type", typeName(events))
		return
	}

	var lastType any
	for i := len(list) - 1; i >= 0; i-- {
		if m, ok := list[i].(map[string]any); ok {
			if t := m["type"]; t != nil && t != "" {
				lastType = t
				break
			}
		}
	}

	slog.Debug("Speedtest stream", "items", len(list), "last_type", lastType, "result_id", resultID(events))
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case map[string]any:
		return "dict"
	case string:
		return "str"
	}
	return "unknown"
}

func asyncWaitResult(ctx context.Context, read registry.ReadFunc, priorID any) (any, error) {
	// Poll the result stream until a fresh completed result appears
	probe := func(ctx context.Context) (any, error) {
		events, err := asyncFetchEvents(ctx, read)
		if err != nil {
			return nil, err
		}
		logStream(ctx, events)
		return events, nil
	}

	ready := func(events any) bool {
		// Stop early on a terminal error (bad server, disconnect)
		if speedtest.StreamHasError(events) {
			return true
		}
		rid := resultID(events)
		return rid != nil && rid != priorID
	}

	result, err := poll.AsyncPollUntil(ctx, probe, ready, pollInterval, pollAttempts)
	if err != nil || result == nil {
		return nil, err
	}
	return *result, nil
}

func asyncBareRead(ctx context.Context, read registry.ReadFunc, src *ARSpeedTestSource) (any, error) {
	// Read the last result
	events, err := asyncFetchEvents(ctx, read)
	if err != nil {
		return nil, err
	}
	if src.ServerID == nil {
		return events, nil
	}

	last := speedtest.RawResultFromEvents(events)
	if last != nil {
		if id := speedtest.ResultServerID(last); id != nil && *id == *src.ServerID {
			return events, nil
		}
	}

	// The last run was for another server; fall back to stored history
	stored, err := history.AsyncLatestForServer(ctx, read, *src.ServerID)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, nil
	}
	return []any{stored}, nil
}

func asyncRefreshResult(ctx context.Context, c *registry.Context, src *ARSpeedTestSource, save bool) (any, error) {
	// Run a fresh test against the source's server/interface and read it
	if c.RunAction == nil {
		return map[string]any{}, nil
	}
	act := &speedtest.ARSpeedTestAction{ServerID: src.ServerID, Iface: src.Iface}
	// Note the current result id, so a stale read is not taken as ours
	current, err := asyncFetchEvents(ctx, c.Read)
	if err != nil {
		return nil, err
	}
	priorID := resultID(current)
	started, err := action.AsyncStartRun(ctx, c.RunAction, act, startDelay)
	if err != nil {
		return nil, err
	}
	if !started {
		return map[string]any{}, nil
	}
	events, err := asyncWaitResult(ctx, c.Read, priorID)
	if err != nil {
		return nil, err
	}
	if events == nil {
		slog.Debug("Speedtest run did not finish; dropping result")
		return map[string]any{}, nil
	}
	if speedtest.StreamHasError(events) {
		message := "unknown error"
		if m := speedtest.StreamErrorMessage(events); m != nil && *m != "" {
			message = *m
		}
		slog.Debug("Speedtest run failed", "error", message)
		return map[string]any{}, nil
	}
	// Persist the fresh run to history, if asked
	if save {
		saved, err := history.AsyncSaveResult(ctx, c.Read, speedtest.RawResultFromEvents(events))
		if err != nil {
			return nil, err
		}
		if !saved {
			slog.Debug("Speedtest result not saved to history")
		}
	}
	return events, nil
}

// FetchState mirrors `fetch_state`: fetch the last speedtest result.
func FetchState(ctx context.Context, c *registry.Context, src arsource.DataSource) (any, error) {
	s := src.(*ARSpeedTestSource)

	if c.Identity == nil || !support.SupportAvailable(c.Identity.Support(), support.ARSupportTypeSpeedtest) {
		return map[string]any{}, nil
	}

	if c.Kwargs.Bool("refresh") {
		return asyncRefreshResult(ctx, c, s, c.Kwargs.Bool("save"))
	}

	result, err := asyncBareRead(ctx, c.Read, s)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

// TranslateState mirrors `translate_state`: the raw result stream into
// the final result.
func TranslateState(raw any, tc *registry.TranslateContext) any {
	return speedtest.ResultFromEvents(raw)
}

// Registration

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSpeedTestSource",
		Default:   func() arsource.DataSource { return NewARSpeedTestSource(nil, nil) },
		Fetch:     FetchState,
		Translate: TranslateState,
	})
}
