package led

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/aimesh"
	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

// Ported from tests/modules/led/test_source.py.

func noRead(t *testing.T) registry.ReadFunc {
	return func(context.Context, endpoint.AREndpoint, string) (any, error) {
		t.Error("callback awaited")
		return nil, nil
	}
}

func TestGetState(t *testing.T) {
	t.Run("fetches_via_nvram", func(t *testing.T) {
		values := map[source.ID]any{source.IDOf(nvram.ARNvramTypeLed): "1"}
		c, calls := contextWith(values, nil, nil)
		c.Read = noRead(t)
		result, err := fetchState(context.Background(), c, ARLedSourceUniversal)
		if err != nil || !reflect.DeepEqual(result, values) {
			t.Errorf("got %v, %v", result, err)
		}
		if calls.multi != 1 || len(calls.requests[0]) != 1 || calls.requests[0][0] != nvram.ARNvramTypeLed {
			t.Errorf("requested %v", calls.requests)
		}
	})

	t.Run("no_get_data_callback", func(t *testing.T) {
		result, _ := fetchState(context.Background(), &registry.Context{Read: noRead(t)}, ARLedSourceUniversal)
		if m, ok := result.(map[source.ID]any); !ok || len(m) != 0 {
			t.Errorf("got %v", result)
		}
	})

	t.Run("non_dict_response", func(t *testing.T) {
		c, _ := contextWith(nil, nil, nil)
		result, _ := fetchState(context.Background(), c, ARLedSourceUniversal)
		if m, ok := result.(map[source.ID]any); !ok || len(m) != 0 {
			t.Errorf("got %v", result)
		}
	})
}

func TestAiMeshState(t *testing.T) {
	for _, led := range []bool{true, false} {
		t.Run(fmt.Sprintf("aimesh_wins_%v", led), func(t *testing.T) {
			led := led
			reply := nvramReply()
			c, _ := contextWith(reply, topologyWith(&led), testIdentity(""))
			result, err := fetchState(context.Background(), c, ARLedSourceUniversal)
			want := map[source.ID]any{source.IDOf(nvram.ARNvramTypeLed): led}
			if err != nil || !reflect.DeepEqual(result, want) {
				t.Fatalf("got %v, %v", result, err)
			}
			if state := translateState(result, nil).(map[ARLedField]any)[ARLedFieldState]; state != led {
				t.Errorf("state %v", state)
			}
			// The nvram reply is left as fetched
			if !reflect.DeepEqual(reply, nvramReply()) {
				t.Errorf("reply changed: %v", reply)
			}
		})
	}

	fallbacks := map[string]any{
		"none":         nil,
		"not_topology": map[string]any{},
		"no_node":      aimesh.NewARAiMeshTopology(),
		"no_led":       topologyWith(nil),
	}
	for name, content := range fallbacks {
		t.Run("nvram_fallback_"+name, func(t *testing.T) {
			c, _ := contextWith(nvramReply(), content, testIdentity(""))
			result, _ := fetchState(context.Background(), c, ARLedSourceUniversal)
			if !reflect.DeepEqual(result, nvramReply()) {
				t.Errorf("got %v", result)
			}
		})
	}

	t.Run("no_identity", func(t *testing.T) {
		c, calls := contextWith(nvramReply(), nil, nil)
		result, _ := fetchState(context.Background(), c, ARLedSourceUniversal)
		if !reflect.DeepEqual(result, nvramReply()) || calls.multi != 1 || calls.single != 0 {
			t.Errorf("got %v after %+v", result, calls)
		}
	})
}

func TestTranslateState(t *testing.T) {
	for _, data := range []any{nil, map[source.ID]any{}, "text", []any{}} {
		t.Run(fmt.Sprintf("empty_%v", data), func(t *testing.T) {
			if got := translateState(data, nil).(map[ARLedField]any); len(got) != 0 {
				t.Errorf("got %v", got)
			}
		})
	}

	for _, tc := range []struct {
		raw  any
		want bool
	}{{"1", true}, {"0", false}, {1, true}, {0, false}} {
		t.Run(fmt.Sprintf("state_%v", tc.raw), func(t *testing.T) {
			got := translateState(map[source.ID]any{source.IDOf(nvram.ARNvramTypeLed): tc.raw}, nil)
			if state := got.(map[ARLedField]any)[ARLedFieldState]; state != tc.want {
				t.Errorf("got %v", state)
			}
		})
	}

	t.Run("unparseable", func(t *testing.T) {
		got := translateState(map[source.ID]any{source.IDOf(nvram.ARNvramTypeLed): "maybe"}, nil).(map[ARLedField]any)
		if state, ok := got[ARLedFieldState]; !ok || state != nil {
			t.Errorf("got %v", got)
		}
	})
}
