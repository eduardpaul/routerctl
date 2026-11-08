// Package history mirrors asusrouter/modules/speedtest/history/.
package history

import (
	"context"
	"log/slog"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

var historyRequest = endpoint.HookRequest(endpoint.ARHookOoklaSpeedtestHistory)

// MaxHistory is the number of newest entries the WebUI trims stored
// history to.
const MaxHistory = 50

// Sources and actions

// ARSpeedTestHistorySource mirrors `ARSpeedTestHistorySource`: the
// AsusRouter speedtest history data source (read-only).
type ARSpeedTestHistorySource struct {
	source.Base
}

// SourceName returns the Python class name.
func (*ARSpeedTestHistorySource) SourceName() string { return "ARSpeedTestHistorySource" }

// ARSpeedTestHistorySourceUniversal is the universal instance - preferred.
var ARSpeedTestHistorySourceUniversal = &ARSpeedTestHistorySource{}

// Fetch

func asyncFetchHistory(ctx context.Context, read registry.ReadFunc) (any, error) {
	// Fetch the raw speedtest history
	data, err := read(ctx, endpoint.AREndpointFetchData, historyRequest)
	if err != nil {
		return nil, err
	}
	return endpoint.HookValue(data, endpoint.ARHookOoklaSpeedtestHistory), nil
}

func fetchState(ctx context.Context, c *registry.Context, src source.DataSource) (any, error) {
	// Fetch the raw speedtest history
	if c.Identity == nil || !support.SupportAvailable(c.Identity.Support(), support.ARSupportTypeSpeedtest) {
		return map[string]any{}, nil
	}

	history, err := asyncFetchHistory(ctx, c.Read)
	if err != nil {
		return nil, err
	}
	if history == nil {
		return map[string]any{}, nil
	}
	return history, nil
}

func translateState(raw any, tc *registry.TranslateContext) any {
	// Translate the raw history into results
	return speedtest.HistoryFromList(raw)
}

// AsyncLatestForServer mirrors `async_latest_for_server`: the newest
// stored raw result run against the given server.
func AsyncLatestForServer(ctx context.Context, read registry.ReadFunc, serverID string) (map[string]any, error) {
	current, err := asyncFetchHistory(ctx, read)
	if err != nil {
		return nil, err
	}
	list, ok := current.([]any)
	if !ok {
		return nil, nil
	}
	// History is newest-first, so the first match is the latest run
	for _, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if id := speedtest.ResultServerID(m); id != nil && *id == serverID {
			return m, nil
		}
	}
	return nil, nil
}

// Save

func entryID(entry any) any {
	// Return the `result.id` of a stored history entry, or nil
	m, ok := entry.(map[string]any)
	if !ok {
		return nil
	}
	result, ok := m["result"].(map[string]any)
	if !ok {
		return nil
	}
	return result["id"]
}

// AsyncSaveResult mirrors `async_save_result`: append a fresh run to
// stored history, deduped by result id.
func AsyncSaveResult(ctx context.Context, read registry.ReadFunc, newRaw map[string]any) (bool, error) {
	newID := entryID(newRaw)
	if newRaw == nil || newID == nil {
		slog.Debug("Result has no id; not saving to history")
		return false, nil
	}

	current, err := asyncFetchHistory(ctx, read)
	if err != nil {
		return false, err
	}
	var existing []map[string]any
	if list, ok := current.([]any); ok {
		for _, entry := range list {
			if m, ok := entry.(map[string]any); ok && len(m) > 0 {
				existing = append(existing, m)
			}
		}
	}
	for _, entry := range existing {
		if entryID(entry) == newID {
			slog.Debug("Result already in history; skipping write", "id", newID)
			return true, nil
		}
	}

	entries := append([]map[string]any{newRaw}, existing...)
	if len(entries) > MaxHistory {
		entries = entries[:MaxHistory]
	}
	if _, err := read(ctx, endpoint.AREndpointWriteSpeedtestHistory, speedtest.BuildHistoryRequest(entries)); err != nil {
		return false, err
	}
	return true, nil
}

// Registration

func init() {
	registry.RegisterSource(registry.SourceEntry{
		Name:      "ARSpeedTestHistorySource",
		Default:   func() source.DataSource { return &ARSpeedTestHistorySource{} },
		Fetch:     fetchState,
		Translate: translateState,
	})
}
