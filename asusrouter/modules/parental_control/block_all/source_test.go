package blockall

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/service"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func blockAll(value any) map[source.ID]any {
	return map[source.ID]any{source.IDOf(KeyBlockAll): value}
}

func TestFetchState(t *testing.T) {
	ctx := context.Background()
	var requested []source.DataSource
	c := &registry.Context{FetchDataMulti: func(_ context.Context, srcs []source.DataSource, _ registry.Kwargs) (map[source.ID]any, error) {
		requested = srcs
		return blockAll("1"), nil
	}}
	got, _ := fetchState(ctx, c, ARBlockAllSourceUniversal)
	if m := got.(map[source.ID]any); m[source.IDOf(KeyBlockAll)] != "1" || len(requested) != 1 || requested[0] != source.DataSource(KeyBlockAll) {
		t.Errorf("fetches_via_nvram: %v %v", got, requested)
	}
	if got, _ := fetchState(ctx, &registry.Context{}, ARBlockAllSourceUniversal); len(got.(map[source.ID]any)) != 0 {
		t.Errorf("no_callback: %v", got)
	}
}

func TestTranslateState(t *testing.T) {
	for raw, want := range map[any]bool{"1": true, "0": false, nil: false} {
		if got := translateState(blockAll(raw), nil); got != want {
			t.Errorf("%v: %v", raw, got)
		}
	}
	for _, data := range []any{nil, "text", []any{}} {
		if got := translateState(data, nil); got != false {
			t.Errorf("%v: %v", data, got)
		}
	}
}

type call struct {
	endpoint endpoint.AREndpoint
	request  string
}

func poster(calls *[]call, reply any) registry.ReadFunc {
	return func(_ context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return reply, nil
	}
}

func TestRunAction(t *testing.T) {
	ctx := context.Background()
	ok := map[string]any{"run_service": "restart_firewall"}

	for state, want := range map[bool]float64{true: 1, false: 0} {
		var calls []call
		result, err := RunAction(ctx, &registry.Context{Read: poster(&calls, ok)}, &ARBlockAllAction{State: state})
		if err != nil || !result.(service.ARServiceResult).Success {
			t.Fatalf("toggle: %v %v", result, err)
		}
		var payload map[string]any
		if calls[0].endpoint != endpoint.AREndpointPushData || json.Unmarshal([]byte(calls[0].request), &payload) != nil {
			t.Fatalf("calls %v", calls)
		}
		if payload["rc_service"] != "restart_firewall" || payload["MULTIFILTER_BLOCK_ALL"] != want {
			t.Errorf("payload %v", payload)
		}
	}

	var calls []call
	expired := map[source.DataSource]bool{}
	expire := func(_ context.Context, src source.DataSource) { expired[src] = true }
	_, _ = RunAction(ctx, &registry.Context{Read: poster(&calls, ok), Expire: expire}, &ARBlockAllAction{State: true})
	if !expired[ARBlockAllSourceUniversal] || !expired[KeyBlockAll] {
		t.Errorf("expire_on_success: %v", expired)
	}

	expired = map[source.DataSource]bool{}
	_, _ = RunAction(ctx, &registry.Context{Read: poster(&calls, nil), Expire: expire}, &ARBlockAllAction{State: true})
	if len(expired) != 0 {
		t.Errorf("no_expire_on_failure: %v", expired)
	}
}
