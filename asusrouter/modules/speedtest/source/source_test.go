package source

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	arsource "github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func init() {
	// Skip the real waits
	pollInterval = time.Millisecond
	startDelay = time.Millisecond
}

func identity(speedtestSupport bool) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		arsource.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{support.ARSupportTypeSpeedtest: speedtestSupport},
	})
}

var hook = string(endpoint.ARHookOoklaSpeedtestResult)
var historyHook = string(endpoint.ARHookOoklaSpeedtestHistory)

func events(resultID string) []any {
	return []any{map[string]any{"type": "result", "result": map[string]any{"id": resultID}}, map[string]any{}}
}

func reply(resultID string) map[string]any { return map[string]any{hook: events(resultID)} }

func eventsSrv(resultID string, serverID int64) []any {
	return []any{
		map[string]any{"type": "result", "result": map[string]any{"id": resultID}, "server": map[string]any{"id": serverID}},
		map[string]any{},
	}
}

// sequence answers reads with the given replies (the last one repeats).
func sequence(calls *int, replies ...any) registry.ReadFunc {
	return func(context.Context, endpoint.AREndpoint, string) (any, error) {
		i := *calls
		*calls++
		if i >= len(replies) {
			i = len(replies) - 1
		}
		return replies[i], nil
	}
}

func runner(ran *[]action.ARAction, result any) action.RunActionFunc {
	return func(_ context.Context, a action.ARAction) (any, error) {
		*ran = append(*ran, a)
		return result, nil
	}
}

func isEmpty(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 0
}

func TestSource(t *testing.T) {
	id := arsource.IDOf
	if id(NewARSpeedTestSource(nil, nil)) != id(ARSpeedTestSourceUniversal) {
		t.Error("bare_equal_by_type")
	}
	if id(NewARSpeedTestSource(54112, nil)) == id(NewARSpeedTestSource(nil, nil)) || id(NewARSpeedTestSource(54112, nil)) == id(NewARSpeedTestSource(11367, nil)) {
		t.Error("bound_is_distinct")
	}
	tun := "tun11"
	if id(NewARSpeedTestSource(54112, nil)) != id(NewARSpeedTestSource("54112", nil)) || id(NewARSpeedTestSource(1, &tun)) != id(NewARSpeedTestSource(1, &tun)) {
		t.Error("bound_equal_by_server_and_iface")
	}
	if s := NewARSpeedTestSource(54112, nil).ServerID; s == nil || *s != "54112" {
		t.Error("normalizes")
	}
	if NewARSpeedTestSource(0, nil).ServerID != nil || NewARSpeedTestSource("nope", nil).ServerID != nil {
		t.Error("validates")
	}
	if e := registry.SourceByName("ARSpeedTestSource"); e == nil || e.Fetch == nil || e.Translate == nil {
		t.Error("not registered")
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	supported := identity(true)

	t.Run("default_reads_result", func(t *testing.T) {
		calls := 0
		got, _ := FetchState(ctx, &registry.Context{Read: sequence(&calls, map[string]any{hook: events("abc")}), Identity: supported}, ARSpeedTestSourceUniversal)
		if !reflect.DeepEqual(got, events("abc")) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("extract_non_dict", func(t *testing.T) {
		calls := 0
		got, _ := FetchState(ctx, &registry.Context{Read: sequence(&calls, nil), Identity: supported}, ARSpeedTestSourceUniversal)
		if !isEmpty(got) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("unsupported_skips_fetch", func(t *testing.T) {
		calls := 0
		got, _ := FetchState(ctx, &registry.Context{Read: sequence(&calls, nil), Identity: identity(false)}, ARSpeedTestSourceUniversal)
		if !isEmpty(got) || calls != 0 {
			t.Errorf("got %v, calls %d", got, calls)
		}
	})

	t.Run("bound_source_uses_matching_last_run", func(t *testing.T) {
		calls := 0
		evs := eventsSrv("r", 54112)
		got, _ := FetchState(ctx, &registry.Context{Read: sequence(&calls, map[string]any{hook: evs}), Identity: supported}, NewARSpeedTestSource(54112, nil))
		if !reflect.DeepEqual(got, evs) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("bound_source_falls_back_to_history", func(t *testing.T) {
		calls := 0
		entry := map[string]any{"type": "result", "server": map[string]any{"id": int64(54112)}, "result": map[string]any{"id": "h"}}
		read := sequence(&calls, map[string]any{hook: eventsSrv("r", 23314)}, map[string]any{historyHook: []any{entry}})
		got, _ := FetchState(ctx, &registry.Context{Read: read, Identity: supported}, NewARSpeedTestSource(54112, nil))
		if !reflect.DeepEqual(got, []any{entry}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("bound_source_no_data", func(t *testing.T) {
		calls := 0
		read := sequence(&calls, map[string]any{hook: eventsSrv("r", 23314)}, map[string]any{historyHook: []any{}})
		got, _ := FetchState(ctx, &registry.Context{Read: read, Identity: supported}, NewARSpeedTestSource(54112, nil))
		if !isEmpty(got) {
			t.Errorf("got %v", got)
		}
	})

	refresh := registry.Kwargs{"refresh": true}

	t.Run("refresh_missing_run_callback", func(t *testing.T) {
		calls := 0
		got, _ := FetchState(ctx, &registry.Context{Read: sequence(&calls, nil), Identity: supported, Kwargs: refresh}, ARSpeedTestSourceUniversal)
		if !isEmpty(got) || calls != 0 {
			t.Errorf("got %v, calls %d", got, calls)
		}
	})

	t.Run("refresh_run_not_started", func(t *testing.T) {
		calls := 0
		var ran []action.ARAction
		c := &registry.Context{Read: sequence(&calls, reply("old")), Identity: supported, Kwargs: refresh, RunAction: runner(&ran, false)}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		// The prior-id read happens first, then the failed trigger
		if !isEmpty(got) || calls != 1 {
			t.Errorf("got %v, calls %d", got, calls)
		}
	})

	t.Run("refresh_times_out", func(t *testing.T) {
		calls := 0
		var ran []action.ARAction
		c := &registry.Context{Read: sequence(&calls, reply("old")), Identity: supported, Kwargs: refresh, RunAction: runner(&ran, true)}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		if !isEmpty(got) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("refresh_ignores_stale_then_reads_fresh", func(t *testing.T) {
		calls := 0
		var ran []action.ARAction
		c := &registry.Context{Read: sequence(&calls, reply("old"), reply("old"), reply("new")), Identity: supported, Kwargs: refresh, RunAction: runner(&ran, true)}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		if !reflect.DeepEqual(got, events("new")) || len(ran) != 1 || action.Repr(ran[0]) != action.Repr(&speedtest.ARSpeedTestAction{}) {
			t.Errorf("got %v, ran %v", got, ran)
		}
	})

	t.Run("refresh_uses_source_server", func(t *testing.T) {
		calls := 0
		var ran []action.ARAction
		tun := "tun11"
		c := &registry.Context{Read: sequence(&calls, reply("old"), reply("new")), Identity: supported, Kwargs: refresh, RunAction: runner(&ran, true)}
		_, _ = FetchState(ctx, c, NewARSpeedTestSource(54112, &tun))
		a := ran[0].(*speedtest.ARSpeedTestAction)
		if *a.ServerID != "54112" || *a.Iface != "tun11" {
			t.Errorf("action %+v", a)
		}
	})

	t.Run("refresh_aborts_on_error", func(t *testing.T) {
		calls := 0
		var ran []action.ARAction
		errorStream := map[string]any{hook: []any{
			map[string]any{"type": "log", "message": "No servers defined", "level": "error"},
			map[string]any{},
		}}
		logs := captureDebug(t)
		c := &registry.Context{Read: sequence(&calls, reply("old"), errorStream), Identity: supported, Kwargs: refresh, RunAction: runner(&ran, true)}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		if !isEmpty(got) || !strings.Contains(logs.String(), "No servers defined") {
			t.Errorf("got %v, logs %s", got, logs)
		}
	})

	t.Run("refresh_save_persists_fresh_result", func(t *testing.T) {
		var requests []endpoint.AREndpoint
		replies := []any{reply("old"), reply("new"), map[string]any{historyHook: []any{}}, map[string]any{}}
		c := &registry.Context{
			Read: func(_ context.Context, ep endpoint.AREndpoint, _ string) (any, error) {
				requests = append(requests, ep)
				r := replies[0]
				if len(replies) > 1 {
					replies = replies[1:]
				}
				return r, nil
			},
			Identity:  supported,
			Kwargs:    registry.Kwargs{"refresh": true, "save": true},
			RunAction: runner(&[]action.ARAction{}, true),
		}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		if !reflect.DeepEqual(got, events("new")) || requests[len(requests)-1] != endpoint.AREndpointWriteSpeedtestHistory {
			t.Errorf("got %v, requests %v", got, requests)
		}
	})

	t.Run("save_ignored_without_refresh", func(t *testing.T) {
		calls := 0
		c := &registry.Context{Read: sequence(&calls, map[string]any{hook: events("abc")}), Identity: supported, Kwargs: registry.Kwargs{"save": true}}
		got, _ := FetchState(ctx, c, ARSpeedTestSourceUniversal)
		if !reflect.DeepEqual(got, events("abc")) || calls != 1 {
			t.Errorf("got %v, calls %d", got, calls)
		}
	})
}

// captureDebug routes the default logger to a buffer at debug level.
func captureDebug(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

func TestLogStream(t *testing.T) {
	ctx := context.Background()

	logs := captureDebug(t)
	logStream(ctx, nil)
	if !strings.Contains(logs.String(), "not a list") {
		t.Errorf("not_a_list: %s", logs)
	}

	logs.Reset()
	logStream(ctx, []any{map[string]any{}, "x"})
	if !strings.Contains(logs.String(), "last_type=<nil>") {
		t.Errorf("no_typed_row: %s", logs)
	}

	logs.Reset()
	logStream(ctx, []any{map[string]any{"type": "download"}, map[string]any{"type": "upload"}, map[string]any{}})
	if !strings.Contains(logs.String(), "items=3") || !strings.Contains(logs.String(), "last_type=upload") {
		t.Errorf("typed_rows: %s", logs)
	}

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	logStream(ctx, []any{map[string]any{"type": "download"}, map[string]any{}})
	if buf.Len() != 0 {
		t.Errorf("skips_work_when_debug_off: %s", buf.String())
	}
}

func TestTranslateState(t *testing.T) {
	r, ok := TranslateState(events("abc"), nil).(*speedtest.ARSpeedTestResult)
	if !ok || r == nil || *r.ResultID != "abc" {
		t.Errorf("translates_final_result: %v", r)
	}
	if r := TranslateState([]any{map[string]any{"type": "download"}}, nil).(*speedtest.ARSpeedTestResult); r != nil {
		t.Errorf("no_result: %v", r)
	}
}
