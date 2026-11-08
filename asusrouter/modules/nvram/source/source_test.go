package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/nvram"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func id(src source.DataSource) source.ID { return source.IDOf(src) }

type call struct {
	ep      endpoint.AREndpoint
	request string
}

// reader returns a fixed response and records the calls.
func reader(response any, calls *[]call) registry.ReadFunc {
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		*calls = append(*calls, call{ep, request})
		return response, nil
	}
}

func fetch(t *testing.T, response any, srcs ...source.DataSource) (map[source.ID]any, []call) {
	t.Helper()
	var calls []call
	result, err := fetchState(context.Background(), &registry.Context{Read: reader(response, &calls)}, srcs)
	if err != nil {
		t.Fatal(err)
	}
	return result, calls
}

func TestFetchState(t *testing.T) {
	t.Run("FiltersInvalidKeys", func(t *testing.T) {
		result, calls := fetch(t, map[string]any{"label_mac": "00:11:22:33:44:55", "bad_key": "x"}, nvram.ARNvramTypeMac)
		if want := (map[source.ID]any{id(nvram.ARNvramTypeMac): "00:11:22:33:44:55"}); !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
		if len(calls) != 1 {
			t.Errorf("calls = %d, want 1", len(calls))
		}
	})
	t.Run("FiltersUnknownMember", func(t *testing.T) {
		result, _ := fetch(t, map[string]any{"label_mac": "00:11:22:33:44:55", "unknown": "x"}, nvram.ARNvramTypeMac)
		if _, ok := result[id(nvram.ARNvramTypeUnknown)]; ok {
			t.Error("UNKNOWN in result")
		}
		if want := (map[source.ID]any{id(nvram.ARNvramTypeMac): "00:11:22:33:44:55"}); !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	})
	for _, tt := range []struct {
		name     string
		response any
	}{
		{"NonDictResponseReturnsEmpty/None", nil},
		{"NonDictResponseReturnsEmpty/String", "string"},
		{"NonDictResponseReturnsEmpty/Int", 42},
		{"NonDictResponseReturnsEmpty/List", []any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if result, _ := fetch(t, tt.response, nvram.ARNvramTypeMac); len(result) != 0 {
				t.Errorf("result = %v, want {}", result)
			}
		})
	}
	t.Run("ListSource", func(t *testing.T) {
		result, _ := fetch(t, map[string]any{"label_mac": "00:11:22:33:44:55", "productid": "RT-AX88U"},
			nvram.ARNvramTypeMac, nvram.ARNvramTypeModel)
		want := map[source.ID]any{
			id(nvram.ARNvramTypeMac):   "00:11:22:33:44:55",
			id(nvram.ARNvramTypeModel): "RT-AX88U",
		}
		if !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	})
	t.Run("RequestBuiltFromHooks", func(t *testing.T) {
		_, calls := fetch(t, map[string]any{}, nvram.ARNvramTypeMac, nvram.ARNvramTypeModel)
		want := []call{{endpoint.AREndpointFetchData, "hook=nvram_get(label_mac);nvram_get(productid)"}}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("calls = %v, want %v", calls, want)
		}
	})
}

func TestFetchStateIndexed(t *testing.T) {
	t.Run("SingleIndexed", func(t *testing.T) {
		src := nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWanIpaddr, 0)
		result, _ := fetch(t, map[string]any{"wan0_ipaddr": "1.2.3.4"}, src)
		if want := (map[source.ID]any{id(src): "1.2.3.4"}); !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	})
	t.Run("MixedFlatAndIndexed", func(t *testing.T) {
		flat := nvram.ARNvramTypeMac
		indexed := nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWanState, 1)
		result, _ := fetch(t, map[string]any{"label_mac": "00:11:22:33:44:55", "wan1_state_t": "2"}, flat, indexed)
		want := map[source.ID]any{id(flat): "00:11:22:33:44:55", id(indexed): "2"}
		if !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	})
	t.Run("UnrequestedKeyDropped", func(t *testing.T) {
		src := nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWanIpaddr, 0)
		result, _ := fetch(t, map[string]any{"wan0_ipaddr": "1.2.3.4", "other": "x"}, src)
		if want := (map[source.ID]any{id(src): "1.2.3.4"}); !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	})
}

func TestAsyncExpireValues(t *testing.T) {
	ctx := context.Background()
	recorder := func(got *[]source.ID) func(context.Context, source.DataSource) {
		return func(_ context.Context, src source.DataSource) { *got = append(*got, id(src)) }
	}
	t.Run("ExpiresEachItem", func(t *testing.T) {
		var got []source.ID
		AsyncExpireValues(ctx, recorder(&got), []nvram.ARNvramItem{nvram.ARNvramTypeMac, nvram.ARNvramTypeWanUnit})
		want := []source.ID{id(nvram.ARNvramTypeMac), id(nvram.ARNvramTypeWanUnit)}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("expired %v, want %v", got, want)
		}
	})
	t.Run("SingleItem", func(t *testing.T) {
		var got []source.ID
		src := nvram.NewARNvramIndexSource(nvram.ARNvramIndexTypeWanState, 0)
		AsyncExpireValues(ctx, recorder(&got), src)
		if want := []source.ID{id(src)}; !reflect.DeepEqual(got, want) {
			t.Errorf("expired %v, want %v", got, want)
		}
	})
	t.Run("NoCallback", func(t *testing.T) {
		AsyncExpireValues(ctx, nil, nvram.ARNvramTypeMac)
	})
}

func multi(values map[source.ID]any) *registry.Context {
	return &registry.Context{
		FetchDataMulti: func(ctx context.Context, srcs []source.DataSource, kw registry.Kwargs) (map[source.ID]any, error) {
			return values, nil
		},
	}
}

func TestAsyncFetchValues(t *testing.T) {
	ctx := context.Background()
	t.Run("NoCallback", func(t *testing.T) {
		for _, c := range []*registry.Context{nil, {}} {
			got, err := AsyncFetchValues(ctx, c, nvram.ARNvramTypeMac, nil)
			if err != nil || got == nil || len(got) != 0 {
				t.Errorf("got %v, %v; want {}", got, err)
			}
		}
	})
	t.Run("ReturnsDict", func(t *testing.T) {
		values := map[source.ID]any{id(nvram.ARNvramTypeMac): "x"}
		got, err := AsyncFetchValues(ctx, multi(values), nvram.ARNvramTypeMac, nil)
		if err != nil || !reflect.DeepEqual(got, values) {
			t.Errorf("got %v, %v; want %v", got, err, values)
		}
	})
	t.Run("NonDictReturnsEmpty", func(t *testing.T) {
		// The Go callback is typed; Python's non-dict results map to nil
		got, err := AsyncFetchValues(ctx, multi(nil), nvram.ARNvramTypeMac, nil)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("got %v, %v; want {}", got, err)
		}
	})
}

func TestAsyncGetValue(t *testing.T) {
	ctx := context.Background()
	t.Run("ReturnsItemValue", func(t *testing.T) {
		got, err := AsyncGetValue(ctx, multi(map[source.ID]any{id(nvram.ARNvramTypeMac): "aa:bb"}), nvram.ARNvramTypeMac, nil)
		if err != nil || got != "aa:bb" {
			t.Errorf("got %v, %v; want aa:bb", got, err)
		}
	})
	t.Run("MissingItemReturnsNone", func(t *testing.T) {
		got, err := AsyncGetValue(ctx, multi(map[source.ID]any{}), nvram.ARNvramTypeMac, nil)
		if err != nil || got != nil {
			t.Errorf("got %v, %v; want nil", got, err)
		}
	})
}

func TestModuleRegistersCallables(t *testing.T) {
	for _, name := range []string{"ARNvramType", "ARNvramIndexSource"} {
		t.Run(name, func(t *testing.T) {
			e := registry.SourceByName(name)
			if e == nil {
				t.Fatal("not registered")
			}
			if e.FetchMulti == nil || e.TranslateMulti == nil || !e.Multi() {
				t.Error("must register the batched fetch/translate callables")
			}
			if e.Fetch != nil || e.Translate != nil {
				t.Error("must not register single callables")
			}
		})
	}
	// The registered translate is nvram.TranslateState
	data := map[source.ID]any{id(nvram.ARNvramTypeMac): "00:11:22:33:44:55"}
	got := registry.SourceByName("ARNvramType").TranslateMulti(data, nil)
	if !reflect.DeepEqual(got, nvram.TranslateState(data)) {
		t.Errorf("translate = %v", got)
	}
}
