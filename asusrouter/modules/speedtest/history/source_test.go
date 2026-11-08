package history

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/device"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/speedtest"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func identity(speedtestSupport bool) *device.ARDeviceIdentity {
	return device.ARDeviceIdentityBuild(device.IdentityData{
		source.IDOf(support.ARSupportSourceUniversal): map[support.ARSupportType]any{support.ARSupportTypeSpeedtest: speedtestSupport},
	})
}

var hook = string(endpoint.ARHookOoklaSpeedtestHistory)

func history() []any {
	return []any{
		map[string]any{"type": "result", "result": map[string]any{"id": "a"}},
		map[string]any{"type": "result", "result": map[string]any{"id": "b"}},
		map[string]any{},
	}
}

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func reader(calls *[]call, reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return reply, nil
	}
}

func writes(calls []call) []call {
	var out []call
	for _, c := range calls {
		if c.endpoint == endpoint.AREndpointWriteSpeedtestHistory {
			out = append(out, c)
		}
	}
	return out
}

func storedIDs(t *testing.T, request string) []string {
	t.Helper()
	_, body, _ := strings.Cut(request, "=")
	text, err := url.QueryUnescape(body)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, entry["result"].(map[string]any)["id"].(string))
	}
	return ids
}

func TestSource(t *testing.T) {
	if e := registry.SourceByName("ARSpeedTestHistorySource"); e == nil || e.Fetch == nil {
		t.Error("not registered")
	}
	if source.IDOf(&ARSpeedTestHistorySource{}) != source.IDOf(ARSpeedTestHistorySourceUniversal) {
		t.Error("equal by type")
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	var calls []call
	got, _ := fetchState(ctx, &registry.Context{Read: reader(&calls, map[string]any{hook: history()}), Identity: identity(true)}, ARSpeedTestHistorySourceUniversal)
	if !reflect.DeepEqual(got, history()) || calls[0].endpoint != endpoint.AREndpointFetchData {
		t.Errorf("reads_history: %v", got)
	}

	got, _ = fetchState(ctx, &registry.Context{Read: reader(&calls, nil), Identity: identity(true)}, ARSpeedTestHistorySourceUniversal)
	if m, ok := got.(map[string]any); !ok || len(m) != 0 {
		t.Errorf("non_dict: %v", got)
	}

	calls = nil
	got, _ = fetchState(ctx, &registry.Context{Read: reader(&calls, nil), Identity: identity(false)}, ARSpeedTestHistorySourceUniversal)
	if m, ok := got.(map[string]any); !ok || len(m) != 0 || len(calls) != 0 {
		t.Errorf("unsupported_skips_fetch: %v", got)
	}
}

func TestTranslateState(t *testing.T) {
	results := translateState(history(), nil).([]speedtest.ARSpeedTestResult)
	if len(results) != 2 || *results[0].ResultID != "a" || *results[1].ResultID != "b" {
		t.Errorf("got %+v", results)
	}
}

func TestLatestForServer(t *testing.T) {
	ctx := context.Background()
	var calls []call
	entries := []any{
		map[string]any{"type": "result", "server": map[string]any{"id": int64(54112)}, "result": map[string]any{"id": "n"}},
		map[string]any{"type": "result", "server": map[string]any{"id": int64(54112)}, "result": map[string]any{"id": "o"}},
		map[string]any{"type": "result", "server": map[string]any{"id": int64(23314)}, "result": map[string]any{"id": "x"}},
	}
	entry, _ := AsyncLatestForServer(ctx, reader(&calls, map[string]any{hook: entries}), "54112")
	if entry == nil || entry["result"].(map[string]any)["id"] != "n" {
		t.Errorf("returns_newest_match: %v", entry)
	}

	noMatch := []any{map[string]any{"type": "result", "server": map[string]any{"id": int64(23314)}}}
	if entry, _ := AsyncLatestForServer(ctx, reader(&calls, map[string]any{hook: noMatch}), "54112"); entry != nil {
		t.Errorf("no_match: %v", entry)
	}
	if entry, _ := AsyncLatestForServer(ctx, reader(&calls, nil), "54112"); entry != nil {
		t.Errorf("history_not_a_list: %v", entry)
	}
}

func TestSaveResult(t *testing.T) {
	ctx := context.Background()

	t.Run("no_id", func(t *testing.T) {
		var calls []call
		if ok, _ := AsyncSaveResult(ctx, reader(&calls, nil), map[string]any{"result": map[string]any{}}); ok || len(calls) != 0 {
			t.Error("saved without an id")
		}
	})

	t.Run("none_result", func(t *testing.T) {
		var calls []call
		if ok, _ := AsyncSaveResult(ctx, reader(&calls, nil), nil); ok {
			t.Error("saved nil")
		}
	})

	t.Run("duplicate_skips_write", func(t *testing.T) {
		var calls []call
		ok, _ := AsyncSaveResult(ctx, reader(&calls, map[string]any{hook: history()}), map[string]any{"type": "result", "result": map[string]any{"id": "a"}})
		if !ok || len(writes(calls)) != 0 {
			t.Errorf("ok %v, writes %v", ok, writes(calls))
		}
	})

	t.Run("prepends_new", func(t *testing.T) {
		var calls []call
		ok, _ := AsyncSaveResult(ctx, reader(&calls, map[string]any{hook: history()}), map[string]any{"type": "result", "result": map[string]any{"id": "c"}, "packetLoss": int64(1)})
		// Newest first, old entries kept, trailing empty dropped
		if !ok || !reflect.DeepEqual(storedIDs(t, writes(calls)[0].request), []string{"c", "a", "b"}) {
			t.Errorf("writes %v", writes(calls))
		}
	})

	t.Run("caps_history", func(t *testing.T) {
		var calls []call
		var long []any
		for i := 0; i < MaxHistory+10; i++ {
			long = append(long, map[string]any{"type": "result", "result": map[string]any{"id": strconv.Itoa(i)}})
		}
		_, _ = AsyncSaveResult(ctx, reader(&calls, map[string]any{hook: long}), map[string]any{"type": "result", "result": map[string]any{"id": "new"}})
		if n := len(storedIDs(t, writes(calls)[0].request)); n != MaxHistory {
			t.Errorf("%d lines", n)
		}
	})

	t.Run("history_not_a_list", func(t *testing.T) {
		var calls []call
		ok, _ := AsyncSaveResult(ctx, reader(&calls, nil), map[string]any{"type": "result", "result": map[string]any{"id": "x"}})
		if !ok || len(writes(calls)) != 1 {
			t.Errorf("ok %v, writes %v", ok, writes(calls))
		}
	})
}
