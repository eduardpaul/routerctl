package ping

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/action"
	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func init() {
	// Skip the real waits
	pollInterval = time.Millisecond
	startDelay = time.Millisecond
}

// A single finished ping run, in response/content order
func row() []any {
	return []any{"1.1.1.1", "Cloudflare", "1", "8.44", "10.12", "12.36", "5", "5", "0.0", "1782998188"}
}

type statusCalls struct {
	n     int
	force []bool
}

// statusFetch is a FetchDataMulti returning the given ping status.
func statusFetch(calls *statusCalls, value any) registry.FetchDataMultiFunc {
	return func(_ context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
		calls.n++
		calls.force = append(calls.force, kw.Bool("force"))
		if len(srcs) != 1 || srcs[0] != source.DataSource(nvram.ARNvramTypeDnsPingStatus) {
			return nil, nil
		}
		if value == nil {
			return nil, nil
		}
		return map[source.ID]any{source.IDOf(nvram.ARNvramTypeDnsPingStatus): value}, nil
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

func isEmpty(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 0
}

func TestSource(t *testing.T) {
	if source.IDOf(&ARPingSource{}) != source.IDOf(ARPingSourceUniversal) {
		t.Error("equal by type")
	}
	if e := registry.SourceByName("ARPingSource"); e == nil || e.Fetch == nil || e.Translate == nil {
		t.Error("not registered")
	}
}

func TestBuildRequest(t *testing.T) {
	request := buildRequest()
	for _, part := range []string{"db=dns_ping", "dns_ip", "data_time"} {
		if !strings.Contains(request, part) {
			t.Errorf("%q misses %q", request, part)
		}
	}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()

	t.Run("no_data_callback", func(t *testing.T) {
		var calls []call
		got, _ := fetchState(ctx, &registry.Context{Read: reader(&calls, nil)}, ARPingSourceUniversal)
		if !isEmpty(got) || len(calls) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("default_reads_finished_results", func(t *testing.T) {
		var calls []call
		var status statusCalls
		var ran []action.ARAction
		c := &registry.Context{
			Read:           reader(&calls, map[string]any{"contents": []any{row()}}),
			FetchDataMulti: statusFetch(&status, "3"),
			RunAction: func(_ context.Context, a action.ARAction) (any, error) {
				ran = append(ran, a)
				return true, nil
			},
		}
		got, _ := fetchState(ctx, c, ARPingSourceUniversal)
		if !reflect.DeepEqual(got, []any{row()}) || len(ran) != 0 || status.force[len(status.force)-1] != true {
			t.Errorf("got %v, ran %v, status %+v", got, ran, status)
		}
	})

	t.Run("waits_for_in_progress_run", func(t *testing.T) {
		var calls []call
		var status statusCalls
		c := &registry.Context{Read: reader(&calls, nil), FetchDataMulti: statusFetch(&status, "1")}
		got, _ := fetchState(ctx, c, ARPingSourceUniversal)
		if !isEmpty(got) || len(calls) != 0 || status.n != pollAttempts || !status.force[0] {
			t.Errorf("got %v, calls %v, status %+v", got, calls, status)
		}
	})

	t.Run("empty_status_stops_at_once", func(t *testing.T) {
		var calls []call
		var status statusCalls
		c := &registry.Context{Read: reader(&calls, nil), FetchDataMulti: statusFetch(&status, "")}
		got, _ := fetchState(ctx, c, ARPingSourceUniversal)
		// Read once, then stop - no retry loop
		if !isEmpty(got) || len(calls) != 0 || status.n != 1 {
			t.Errorf("got %v, calls %v, status %+v", got, calls, status)
		}
	})

	t.Run("status_non_dict", func(t *testing.T) {
		var calls []call
		var status statusCalls
		c := &registry.Context{Read: reader(&calls, nil), FetchDataMulti: statusFetch(&status, nil)}
		if got, _ := fetchState(ctx, c, ARPingSourceUniversal); !isEmpty(got) {
			t.Errorf("got %v", got)
		}
	})

	for name, reply := range map[string]any{"empty_contents": map[string]any{"contents": []any{}}, "non_dict_data": nil} {
		t.Run(name, func(t *testing.T) {
			var calls []call
			var status statusCalls
			c := &registry.Context{Read: reader(&calls, reply), FetchDataMulti: statusFetch(&status, "3")}
			if got, _ := fetchState(ctx, c, ARPingSourceUniversal); !isEmpty(got) {
				t.Errorf("got %v", got)
			}
		})
	}

	t.Run("refresh_no_run_action_callback", func(t *testing.T) {
		var calls []call
		var status statusCalls
		c := &registry.Context{Read: reader(&calls, nil), FetchDataMulti: statusFetch(&status, "3"), Kwargs: registry.Kwargs{"refresh": true}}
		if got, _ := fetchState(ctx, c, ARPingSourceUniversal); !isEmpty(got) || len(calls) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("refresh_run_not_started", func(t *testing.T) {
		var calls []call
		var status statusCalls
		c := &registry.Context{
			Read:           reader(&calls, nil),
			FetchDataMulti: statusFetch(&status, "3"),
			RunAction: func(context.Context, action.ARAction) (any, error) {
				return service.ARServiceResult{Success: false}, nil
			},
			Kwargs: registry.Kwargs{"refresh": true},
		}
		if got, _ := fetchState(ctx, c, ARPingSourceUniversal); !isEmpty(got) || len(calls) != 0 || status.n != 0 {
			t.Errorf("got %v, calls %v, status %+v", got, calls, status)
		}
	})

	t.Run("refresh_runs_then_reads", func(t *testing.T) {
		var calls []call
		var status statusCalls
		var ran []action.ARAction
		c := &registry.Context{
			Read:           reader(&calls, map[string]any{"contents": []any{row()}}),
			FetchDataMulti: statusFetch(&status, "3"),
			RunAction: func(_ context.Context, a action.ARAction) (any, error) {
				ran = append(ran, a)
				return true, nil
			},
			Kwargs: registry.Kwargs{"refresh": true},
		}
		got, _ := fetchState(ctx, c, ARPingSourceUniversal)
		if !reflect.DeepEqual(got, []any{row()}) || len(ran) != 1 || ran[0].ActionName() != "ARPingAction" ||
			calls[len(calls)-1].endpoint != endpoint.AREndpointFetchDiagnosticsData {
			t.Errorf("got %v, ran %v, calls %v", got, ran, calls)
		}
	})
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }
func ip(i int) *int       { return &i }

func TestTranslateState(t *testing.T) {
	one := *identifiers.IpAddressFromValueSafe("1.1.1.1")

	want := map[identifiers.IpAddress]ARPingResult{one: {
		Alias:     sp("Cloudflare"),
		Valid:     bp(true),
		Timestamp: ip(1782998188),
		Metrics: map[common.ARMetricType]any{
			common.ARMetricTypeLatencyMin:      8.44,
			common.ARMetricTypeLatencyAvg:      10.12,
			common.ARMetricTypeLatencyMax:      12.36,
			common.ARMetricTypePacketsSent:     5,
			common.ARMetricTypePacketsReceived: 5,
			common.ARMetricTypePacketLoss:      0.0,
		},
	}}
	if got := translateState([]any{row()}, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("full_row: %+v", got)
	}

	short := map[identifiers.IpAddress]ARPingResult{one: {Metrics: map[common.ARMetricType]any{}}}
	if got := translateState([]any{[]any{"1.1.1.1"}}, nil); !reflect.DeepEqual(got, short) {
		t.Errorf("short_row: %+v", got)
	}

	if got := translateState(map[string]any{"contents": []any{}}, nil).(map[identifiers.IpAddress]ARPingResult); len(got) != 0 {
		t.Errorf("non_list: %v", got)
	}

	got := translateState([]any{[]any{}, "not-a-row", []any{"not-an-ip"}, row()}, nil).(map[identifiers.IpAddress]ARPingResult)
	if _, ok := got[one]; len(got) != 1 || !ok {
		t.Errorf("skips_malformed_rows: %v", got)
	}
}
