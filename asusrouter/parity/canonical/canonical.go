// Package canonical mirrors `canonical()` in parity/py_harness.py: it
// turns any library value into a JSON-able tree that is identical for the
// Python and the Go library. See asusrouter/PARITY.md "Canonical form".
package canonical

import (
	"encoding"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
)

// Valuer lets a type define its canonical value directly.
type Valuer interface {
	CanonicalValue() any
}

var (
	timeType     = reflect.TypeOf(time.Time{})
	durationType = reflect.TypeOf(time.Duration(0))
	valuerType   = reflect.TypeOf((*Valuer)(nil)).Elem()
	textType     = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
)

// Of returns the canonical form of v: nil, bool, string, int64, float64,
// []any or map[string]any.
func Of(v any) any {
	return of(reflect.ValueOf(v))
}

// JSON renders the canonical form of v (sorted keys, one-space indent).
func JSON(v any) ([]byte, error) {
	return json.MarshalIndent(Of(v), "", " ")
}

func of(rv reflect.Value) any {
	if !rv.IsValid() {
		return nil
	}
	t := rv.Type()

	// Interfaces and pointers: unwrap (nil -> null)
	if t.Kind() == reflect.Interface || t.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		if t.Kind() == reflect.Pointer && t.Implements(valuerType) {
			return Of(rv.Interface().(Valuer).CanonicalValue())
		}
		return of(rv.Elem())
	}

	if t.Implements(valuerType) {
		return Of(rv.Interface().(Valuer).CanonicalValue())
	}
	if t == timeType {
		return isoformat(rv.Interface().(time.Time))
	}
	if t == durationType {
		return number(rv.Interface().(time.Duration).Seconds())
	}

	switch t.Kind() {
	case reflect.Bool:
		return rv.Bool()
	case reflect.String:
		return rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return number(rv.Float())
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[key(iter.Key())] = of(iter.Value())
		}
		return out
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !t.Elem().Implements(valuerType) {
			return base64.StdEncoding.EncodeToString(rv.Bytes())
		}
		fallthrough
	case reflect.Array:
		// Identifier types backed by arrays (MAC) render via Stringer
		if t.Kind() == reflect.Array && t.Implements(stringerType) {
			return rv.Interface().(fmt.Stringer).String()
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = of(rv.Index(i))
		}
		return out
	case reflect.Struct:
		if t.Implements(textType) {
			b, err := rv.Interface().(encoding.TextMarshaler).MarshalText()
			if err == nil {
				return string(b)
			}
		}
		return structOf(rv)
	}
	if t.Implements(stringerType) {
		return rv.Interface().(fmt.Stringer).String()
	}
	return fmt.Sprintf("%v", rv.Interface())
}

func structOf(rv reflect.Value) map[string]any {
	out := map[string]any{}
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			tagName, _, _ := strings.Cut(tag, ",")
			if tagName == "-" {
				continue
			}
			if tagName != "" {
				name = tagName
			}
		} else if f.Anonymous && f.Type.Kind() == reflect.Struct {
			// Embedded struct without a tag: promote its fields
			for k, v := range structOf(rv.Field(i)) {
				out[k] = v
			}
			continue
		}
		out[name] = of(rv.Field(i))
	}
	return out
}

// key renders a mapping key as a string, like `_key` in the harness.
func key(rv reflect.Value) string {
	c := of(rv)
	switch v := c.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case int64:
		return strconv.FormatInt(v, 10)
	}
	b, _ := json.Marshal(sortedCopy(c))
	return string(b)
}

func sortedCopy(v any) any { return v } // encoding/json sorts map keys already

// number: integral floats become ints, others are rounded to 6 places
// (correctly rounded, like Python round()).
func number(f float64) any {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return int64(f)
	}
	r, _ := strconv.ParseFloat(strconv.FormatFloat(f, 'f', 6, 64), 64)
	if r == math.Trunc(r) && math.Abs(r) < 1e15 {
		return int64(r)
	}
	return r
}

// isoformat mirrors Python `datetime.isoformat()`: naive times (in
// consts.NaiveLocation) get no offset; microseconds only when non-zero.
func isoformat(t time.Time) string {
	s := t.Format("2006-01-02T15:04:05")
	if us := t.Nanosecond() / 1000; us != 0 {
		s += fmt.Sprintf(".%06d", us)
	}
	if t.Location() == consts.NaiveLocation {
		return s
	}
	_, offset := t.Zone()
	sign := '+'
	if offset < 0 {
		sign = '-'
		offset = -offset
	}
	s += fmt.Sprintf("%c%02d:%02d", sign, offset/3600, (offset%3600)/60)
	if sec := offset % 60; sec != 0 {
		s += fmt.Sprintf(":%02d", sec)
	}
	return s
}

// SortedSet canonicalizes a Go slice that represents a Python set: the
// canonical items sorted by their JSON form.
func SortedSet[T any](items []T) []any {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = Of(it)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := json.Marshal(out[i])
		b, _ := json.Marshal(out[j])
		return string(a) < string(b)
	})
	return out
}
