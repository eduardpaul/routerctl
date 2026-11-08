package support

import (
	"reflect"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// TableEntry is one `key: value` item of an ordered translation table
// (a Python dict literal, iterated in insertion order).
type TableEntry[T any] struct {
	Key   string
	Value T
}

// MakeBoolTranslator creates a bool translator for the given support key.
//
// A nil map stands for Python's non-dict input and reads as false even
// when negated (an empty, non-nil map is `{}`).
func MakeBoolTranslator(key string, negate bool) func(data map[string]any) bool {
	return func(data map[string]any) bool {
		if data == nil {
			return false
		}
		result := readers.IsTrueInDict(key, data)
		if negate {
			return !result
		}
		return result
	}
}

// MakeEnumTranslator creates a translator returning the first matching
// value or default.
func MakeEnumTranslator[T any](table []TableEntry[T], def T) func(data map[string]any) T {
	return func(data map[string]any) T {
		for _, entry := range table {
			if readers.IsTrueInDict(entry.Key, data) {
				return entry.Value
			}
		}
		return def
	}
}

// MakeIntTranslator creates an int translator for the given support key.
func MakeIntTranslator(key string) func(data map[string]any) int {
	return func(data map[string]any) int {
		if v := converters.RawToInt(data[key]); v != nil {
			return *v
		}
		return 0
	}
}

// MakeListTranslator creates a list translator for the given support
// value table.
func MakeListTranslator[T any](table []TableEntry[T]) func(data map[string]any) []T {
	return func(data map[string]any) []T {
		out := []T{}
		for _, entry := range table {
			if readers.IsTrueInDict(entry.Key, data) {
				out = append(out, entry.Value)
			}
		}
		return out
	}
}

// SupportAvailable returns true if the bool support flag is set.
func SupportAvailable(support map[ARSupportType]any, key ARSupportType) bool {
	v, ok := support[key].(bool)
	return ok && v
}

// SupportAvailableIn returns true if item is present in the list or dict
// under the key.
func SupportAvailableIn(support map[ARSupportType]any, key ARSupportType, item any) bool {
	value := support[key]
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if equalAny(rv.Index(i).Interface(), item) {
				return true
			}
		}
	case reflect.Map:
		iv := reflect.ValueOf(item)
		if !iv.IsValid() {
			return false
		}
		kt := rv.Type().Key()
		if iv.Type().AssignableTo(kt) {
			return rv.MapIndex(iv).IsValid()
		}
		for _, k := range rv.MapKeys() {
			if equalAny(k.Interface(), item) {
				return true
			}
		}
	}
	return false
}

func equalAny(a, b any) bool {
	defer func() { _ = recover() }()
	return a == b
}

// SupportCapability returns a capability's value from the capabilities
// dict under the key.
func SupportCapability(support map[ARSupportType]any, key ARSupportType, capability any) any {
	value := support[key]
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Map {
		return nil
	}
	cv := reflect.ValueOf(capability)
	if !cv.IsValid() || !cv.Type().AssignableTo(rv.Type().Key()) {
		return nil
	}
	got := rv.MapIndex(cv)
	if !got.IsValid() {
		return nil
	}
	return got.Interface()
}

// SupportValue returns the support value for the given key.
func SupportValue(support map[ARSupportType]any, key ARSupportType) any {
	return support[key]
}
