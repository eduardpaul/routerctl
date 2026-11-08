package readers

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

var testTable = []TableEntry[string, string]{
	{"alpha", "FIELD_A", converters.AsAny(converters.RawToStr)},
	{"beta", "FIELD_B", converters.AsAny(converters.RawToInt)},
	{"gamma", "FIELD_C", converters.AsAny(converters.RawToInt)},
}

func TestReadTableReadsFields(t *testing.T) {
	// Present values convert onto their fields; absent ones skip
	data := map[string]any{"alpha": "x", "beta": "5"}
	if got := ReadTable(data, testTable, nil); !reflect.DeepEqual(got, map[string]any{"FIELD_A": "x", "FIELD_B": 5}) {
		t.Fatalf("got %#v", got)
	}
}

func TestReadTableSkipsEmptyValues(t *testing.T) {
	// Empty raw values and failed conversions are skipped
	data := map[string]any{"alpha": "", "beta": "not-an-int", "gamma": "0"}
	if got := ReadTable(data, testTable, nil); !reflect.DeepEqual(got, map[string]any{"FIELD_C": 0}) {
		t.Fatalf("got %#v", got)
	}
}

func TestReadTableKeyResolvesRawKey(t *testing.T) {
	// A key callable maps table keys to raw data keys
	data := map[string]any{"alpha_1": "x", "beta_1": "5"}
	got := ReadTable(data, testTable, func(kind string) string { return kind + "_1" })
	if !reflect.DeepEqual(got, map[string]any{"FIELD_A": "x", "FIELD_B": 5}) {
		t.Fatalf("got %#v", got)
	}
}

func TestReadTableEmptyData(t *testing.T) {
	// No matching data yields an empty dict
	if got := ReadTable(map[string]any{}, testTable, nil); len(got) != 0 {
		t.Fatalf("got %#v", got)
	}
}
