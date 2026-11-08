// Raw value converters for AsusRouter.
//
// Port of `asusrouter/tools/converters/raw.py`. Every converter returns a
// pointer: nil means Python `None`.
//
// Accepted inputs mirror what Python receives from the readers: nil,
// string, bool, Go integer types, float32/float64, json.Number, []any,
// map[string]any, and pointers to the basic types (a nil pointer is
// `None`).

package converters

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"time"
)

// This is a literal BOM character
const bom = "\uFEFF"

var strToBool = map[string]bool{
	"true":     true,
	"allow":    true,
	"enable":   true,
	"enabled":  true,
	"yes":      true,
	"1":        true,
	"on":       true,
	"false":    false,
	"block":    false,
	"disable":  false,
	"disabled": false,
	"no":       false,
	"0":        false,
	"off":      false,
}

// RawConvert mirrors `raw_convert`: convert a raw value, treating
// empty/absent as no value.
//
// A nil result, a nil pointer or an empty slice from the converter is
// returned as nil. A non-nil pointer result is dereferenced, so the
// result holds plain values (`int`, `string`, ...) like Python does.
func RawConvert(raw any, converter func(any) any) any {
	raw = deref(raw)
	if raw == nil {
		return nil
	}
	if s, ok := raw.(string); ok && s == "" {
		return nil
	}
	value := converter(raw)
	return noValue(value)
}

// noValue normalizes a converter result: nil pointers and empty slices
// (Python `None` and `[]`) become nil, pointers are dereferenced.
func noValue(value any) any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return rv.Elem().Interface()
	case reflect.Slice:
		if rv.Len() == 0 {
			return nil
		}
	}
	return value
}

// AsAny adapts a typed converter (`RawToInt`, `RawToStr`, ...) to the
// `func(any) any` shape used by `RawConvert` and translation tables.
func AsAny[T any](converter func(any) *T) func(any) any {
	return func(value any) any {
		if result := converter(value); result != nil {
			return *result
		}
		return nil
	}
}

// RawToBool mirrors `raw_to_bool`: convert raw value to bool or None.
func RawToBool(value any) *bool {
	value = deref(value)
	switch v := value.(type) {
	case bool:
		return &v
	case json.Number:
		if f, ok := numberFloat(v); ok {
			b := f != 0
			return &b
		}
		return nil
	}
	if f, ok := asNumber(value); ok {
		b := f != 0
		return &b
	}
	cleaned := RawToStr(value)
	if cleaned == nil {
		return nil
	}
	if b, ok := strToBool[strings.ToLower(*cleaned)]; ok {
		return &b
	}
	return nil
}

// RawToDatetime mirrors `raw_to_datetime`: convert raw value to datetime
// or None.
//
// A naive Python datetime is returned in `NaiveLocation`; an aware one
// in a fixed zone (`time.UTC` for a zero offset).
func RawToDatetime(value any) *time.Time {
	cleaned := RawToStr(value)
	if cleaned == nil {
		return nil
	}

	if t, ok := fromISOFormat(*cleaned); ok {
		return &t
	}
	if t, ok := strptimeRFC1123(*cleaned); ok {
		return &t
	}
	return nil
}

// RawToFloat mirrors `raw_to_float`: convert raw value to float or None.
func RawToFloat(value any) *float64 {
	value = deref(value)
	switch v := value.(type) {
	case float64:
		return &v
	case float32:
		f := float64(v)
		return &f
	case bool:
		f := 0.0
		if v {
			f = 1
		}
		return &f
	case json.Number:
		return RawToFloat(string(v))
	}
	if i, ok := asInt(value); ok {
		f := float64(i)
		return &f
	}
	if u, ok := value.(uint64); ok {
		f := float64(u)
		return &f
	}
	cleaned := RawToStr(value)
	if cleaned == nil {
		return nil
	}
	if f, ok := parsePyFloat(*cleaned); ok {
		return &f
	}
	return nil
}

// RawToInt mirrors `raw_to_int` with the default base 10.
func RawToInt(value any) *int {
	return RawToIntBase(value, 10)
}

// RawToIntBase mirrors `raw_to_int(value, base)`: convert raw value to
// int or None.
//
// Integers that do not fit a Go `int` (Python has no limit) give nil.
func RawToIntBase(value any, base int) *int {
	value = deref(value)
	switch v := value.(type) {
	case bool:
		i := 0
		if v {
			i = 1
		}
		return &i
	case float64:
		return floatToInt(v)
	case float32:
		return floatToInt(float64(v))
	case json.Number:
		return RawToIntBase(string(v), base)
	}
	if i, ok := asInt(value); ok {
		return &i
	}
	if _, ok := value.(uint64); ok {
		// Larger than any Go int
		return nil
	}
	cleaned := RawToStr(value)
	if cleaned != nil {
		if i, ok := parsePyInt(*cleaned, base); ok {
			return &i
		}
		if f, ok := parsePyFloat(*cleaned); ok {
			return floatToInt(f)
		}
	}
	return nil
}

// RawToStr mirrors `raw_to_str`: convert raw value to a clean string or
// None.
func RawToStr(value any) *string {
	value = deref(value)
	text, ok := value.(string)
	if !ok {
		return nil
	}
	clean := PyStrip(strings.TrimLeft(text, bom))
	if clean == "" {
		return nil
	}
	return &clean
}

// RawToStrList mirrors `raw_to_str_list`: split a raw value into a list
// of strings by delimiter (Python default: " ").
//
// Like Python's `str.split`, an empty delimiter is a programming error
// and panics.
func RawToStrList(value any, delimiter string) []string {
	cleaned := RawToStr(value)
	if cleaned == nil {
		return []string{}
	}
	if delimiter == "" {
		panic("converters.RawToStrList: empty separator")
	}

	return strings.Split(*cleaned, delimiter)
}

// floatToInt mirrors Python `int(float)`: truncation, None for
// non-finite values (and, in Go, for values outside the int range).
func floatToInt(f float64) *int {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	t := math.Trunc(f)
	if t < math.MinInt64 || t >= math.MaxInt64 {
		return nil
	}
	i := int(t)
	return &i
}

// deref unwraps pointers to basic types; a nil pointer becomes nil.
func deref(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case *string:
		if v == nil {
			return nil
		}
		return *v
	case *bool:
		if v == nil {
			return nil
		}
		return *v
	case *int:
		if v == nil {
			return nil
		}
		return *v
	case *int64:
		if v == nil {
			return nil
		}
		return *v
	case *int32:
		if v == nil {
			return nil
		}
		return *v
	case *float64:
		if v == nil {
			return nil
		}
		return *v
	case *float32:
		if v == nil {
			return nil
		}
		return *v
	case *json.Number:
		if v == nil {
			return nil
		}
		return *v
	case *uint:
		if v == nil {
			return nil
		}
		return *v
	case *uint64:
		if v == nil {
			return nil
		}
		return *v
	}
	return value
}

// asInt returns the value of any Go integer type that fits an int.
func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint:
		if uint64(v) <= math.MaxInt64 {
			return int(v), true
		}
	case uint64:
		if v <= math.MaxInt64 {
			return int(v), true
		}
	}
	return 0, false
}

// asNumber returns the value of any Go int or float type (not bool).
func asNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	}
	if i, ok := asInt(value); ok {
		return float64(i), true
	}
	return 0, false
}

// numberFloat returns the numeric value of a json.Number.
func numberFloat(n json.Number) (float64, bool) {
	return parsePyFloat(PyStrip(string(n)))
}
