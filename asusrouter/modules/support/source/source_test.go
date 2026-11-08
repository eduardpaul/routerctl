package source

import (
	"context"
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/endpoint"
	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
	"github.com/eduardpaul/routerctl/asusrouter/registry"
)

func TestARSupportSource(t *testing.T) {
	// ARSupportSource is a data source (Python: subclass of ARDataSource)
	var src source.DataSource = &support.ARSupportSource{}
	if src.SourceName() != "ARSupportSource" || src.SourceKey() != nil {
		t.Errorf("source name/key: %q %v", src.SourceName(), src.SourceKey())
	}
	// ARSupportSourceUniversal is a module-level instance
	if support.ARSupportSourceUniversal == nil {
		t.Error("universal instance missing")
	}
}

// The translation table has an entry for every non-UNKNOWN ARSupportType.
func TestTranslationTableCoversAllSupportTypes(t *testing.T) {
	for _, m := range support.ARSupportTypeMembers {
		if m == support.ARSupportTypeUnknown {
			continue
		}
		if _, ok := translationTable[m]; !ok {
			t.Errorf("missing %v", m)
		}
	}
}

// mockRead mirrors `AsyncMock(side_effect=[...])` (or a fixed
// return_value when only one response is given).
func mockRead(responses ...any) (registry.ReadFunc, *int) {
	calls := 0
	return func(ctx context.Context, ep endpoint.AREndpoint, request string) (any, error) {
		i := calls
		calls++
		if i >= len(responses) {
			i = len(responses) - 1
		}
		return responses[i], nil
	}, &calls
}

// fetch_state returns the ui_support dict when present.
func TestFetchStateReturnsUiSupport(t *testing.T) {
	read, _ := mockRead(map[string]any{"get_ui_support": map[string]any{"foo": "bar"}})
	result, err := fetchState(context.Background(), &registry.Context{Read: read}, support.ARSupportSourceUniversal)
	if err != nil || !reflect.DeepEqual(result, map[string]any{"foo": "bar"}) {
		t.Errorf("got %v, %v", result, err)
	}
}

// fetch_state returns empty dict when response lacks ui_support.
func TestFetchStateReturnsEmpty(t *testing.T) {
	for i, response := range []any{
		map[string]any{},
		nil,
		map[string]any{"get_ui_support": "not_a_dict"},
		map[string]any{"get_ui_support": nil},
	} {
		read, _ := mockRead(response)
		result, err := fetchState(context.Background(), &registry.Context{Read: read}, support.ARSupportSourceUniversal)
		if err != nil || !reflect.DeepEqual(result, map[string]any{}) {
			t.Errorf("case %d: got %v, %v", i, result, err)
		}
	}
}

// Empty ui_support falls back to the rc_support token list.
func TestFetchStateRcSupportFallback(t *testing.T) {
	read, calls := mockRead(
		map[string]any{"get_ui_support": map[string]any{}},
		map[string]any{"rc_support": "2.4G 5G usbX"},
	)
	result, err := fetchState(context.Background(), &registry.Context{Read: read}, support.ARSupportSourceUniversal)
	want := map[string]any{"2.4G": true, "5G": true, "usbX": true}
	if err != nil || !reflect.DeepEqual(result, want) {
		t.Errorf("got %v, %v", result, err)
	}
	if *calls != 2 {
		t.Errorf("calls = %d", *calls)
	}
}

// translate_state output has all translation table keys.
func TestTranslateStateReturnsAllTranslationTableKeys(t *testing.T) {
	result := TranslateState(map[string]any{})
	for key := range translationTable {
		if _, ok := result[key]; !ok {
			t.Errorf("missing %v", key)
		}
	}
}

// The module registers its callables on import (Python patches
// register_source and reloads; Go checks the registry entry).
func TestModuleRegistersCallables(t *testing.T) {
	entry := registry.SourceByName("ARSupportSource")
	if entry == nil {
		t.Fatal("ARSupportSource not registered")
	}
	if entry.Fetch == nil || entry.Translate == nil || entry.Multi() {
		t.Error("expected single fetch + translate")
	}
	if reflect.ValueOf(entry.Fetch).Pointer() != reflect.ValueOf(fetchState).Pointer() {
		t.Error("fetch is not fetchState")
	}
	if src := entry.Default(); src.SourceName() != "ARSupportSource" {
		t.Errorf("default = %v", src.SourceName())
	}
}
