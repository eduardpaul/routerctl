// Package query mirrors asusrouter/modules/log/query/.
package query

// Query dimensions over processed log events.

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/eduardpaul/routerctl/asusrouter/modules/common"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/modules/wifi"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// Source field key -> queryable axis. A field absent here is not
// queryable. Several fields may feed one axis (`SERVICES` -> `SERVICE`)
var facetable = map[log.AREventKey]log.AREventKey{
	log.AREventKeyCaller:    log.AREventKeyCaller,
	log.AREventKeyClientIp:  log.AREventKeyClientIp,
	log.AREventKeyClientMac: log.AREventKeyClientMac,
	log.AREventKeyFrequency: log.AREventKeyFrequency,
	log.AREventKeyGateway:   log.AREventKeyGateway,
	log.AREventKeyHostname:  log.AREventKeyHostname,
	log.AREventKeyInterface: log.AREventKeyInterface,
	log.AREventKeyLocalIp:   log.AREventKeyLocalIp,
	log.AREventKeyNodeMac:   log.AREventKeyNodeMac,
	log.AREventKeyService:   log.AREventKeyService,
	log.AREventKeyServices:  log.AREventKeyService,
	log.AREventKeyTarget:    log.AREventKeyTarget,
	log.AREventKeyWlId:      log.AREventKeyWlId,
}

// A facet value collection is expanded per item (but never a string):
// Python tuple, list, set and frozenset are a Go slice
func isCollection(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}

// A dimension already readable from an untranslated entry
var entryGetters = map[log.AREventKey]func(*log.ARLogEntry) any{
	log.AREventKeyProgram:     func(e *log.ARLogEntry) any { return translate.ResolveProgram(e) },
	log.AREventKeyProgramName: func(e *log.ARLogEntry) any { return optional(e.ProgramName()) },
}

// A dimension whose value lives on the envelope, not in the facet set
var envelope = map[log.AREventKey]func(*ARLogEvent) any{
	log.AREventKeyEventType:   func(e *ARLogEvent) any { return e.EventType },
	log.AREventKeyProgram:     func(e *ARLogEvent) any { return e.Program },
	log.AREventKeyProgramName: func(e *ARLogEvent) any { return optional(e.ProgramName) },
}

// orNone turns a nil pointer result into None (an untyped nil).
func orNone[T any](f func(any) *T) func(any) any {
	return func(v any) any {
		if r := f(v); r != nil {
			return r
		}
		return nil
	}
}

func optional(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// Query-value coercion per dimension (missing -> value kept as-is)
var coerceFuncs = map[log.AREventKey]func(any) any{
	log.AREventKeyClientIp:  orNone(identifiers.IpAddressFromValueSafe),
	log.AREventKeyClientMac: orNone(identifiers.MacAddressFromValueSafe),
	log.AREventKeyFrequency: func(v any) any { return wifi.ARWiFiFrequencyFromValue(v) },
	log.AREventKeyGateway:   orNone(identifiers.IpAddressFromValueSafe),
	log.AREventKeyLocalIp:   orNone(identifiers.IpAddressFromValueSafe),
	log.AREventKeyNodeMac:   orNone(identifiers.MacAddressFromValueSafe),
	log.AREventKeyProgram:   func(v any) any { return log.ARProgramFromValue(v) },
	log.AREventKeyService:   func(v any) any { return common.ARServiceFromValue(v) },
	log.AREventKeyTarget:    orNone(identifiers.WiFiInterfaceFromValueSafe),
	log.AREventKeyWlId:      orNone(identifiers.WiFiInterfaceFromValueSafe),
}

// QUERYABLE mirrors `QUERYABLE`: every axis a query may name, for a
// caller building its own filters.
var QUERYABLE = func() map[log.AREventKey]struct{} {
	out := map[log.AREventKey]struct{}{}
	for _, axis := range facetable {
		out[axis] = struct{}{}
	}
	for axis := range envelope {
		out[axis] = struct{}{}
	}
	return out
}()

// Coerce mirrors `coerce`: normalize a query value to the dimension's own
// type.
func Coerce(dimension log.AREventKey, value any) any {
	if f, ok := coerceFuncs[dimension]; ok {
		return f(value)
	}

	return value
}

// pyKey normalizes a value to how Python hashes and compares it: a
// `StrEnum` equals its str, an `IntEnum` (or bool) its int, and an
// identifier compares by value rather than by pointer.
func pyKey(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Bool:
		if rv.Bool() {
			return int64(1)
		}
		return int64(0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		if f := rv.Float(); f == float64(int64(f)) {
			return int64(f)
		}
		return rv.Float()
	}
	if rv.Type().Comparable() {
		return rv.Interface()
	}
	return fmt.Sprintf("%T:%v", rv.Interface(), rv.Interface())
}

// pyEqual mirrors Python `==` between two query values.
func pyEqual(a, b any) bool { return pyKey(a) == pyKey(b) }

// Facet is one queryable `(axis, value)` pair.
type Facet struct {
	Axis  log.AREventKey
	Value any
}

type facetKey struct {
	axis  log.AREventKey
	value any
}

// FacetSet is the Go form of the frozenset of facets.
type FacetSet struct {
	items []Facet
	index map[facetKey]struct{}
}

func (s *FacetSet) add(axis log.AREventKey, value any) {
	key := facetKey{axis, pyKey(value)}
	if _, ok := s.index[key]; ok {
		return
	}
	s.index[key] = struct{}{}
	s.items = append(s.items, Facet{axis, value})
}

// Has reports whether the set holds `(axis, value)`.
func (s *FacetSet) Has(axis log.AREventKey, value any) bool {
	if s == nil {
		return false
	}
	_, ok := s.index[facetKey{axis, pyKey(value)}]
	return ok
}

// Items returns the facets (in a stable order; Python sets are unordered).
func (s *FacetSet) Items() []Facet {
	if s == nil {
		return nil
	}
	return s.items
}

// Len returns the number of facets.
func (s *FacetSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.items)
}

// ExtractFacets mirrors `extract_facets`: derive queryable `(axis,
// value)` facets from an event's data.
func ExtractFacets(data map[log.AREventKey]any) *FacetSet {
	facets := &FacetSet{index: map[facetKey]struct{}{}}
	// Python walks the dict in insertion order; Go keys are sorted
	fields := make([]log.AREventKey, 0, len(data))
	for field := range data {
		fields = append(fields, field)
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i] < fields[j] })
	for _, field := range fields {
		value := data[field]
		axis, ok := facetable[field]
		if !ok {
			continue
		}
		if isCollection(value) {
			rv := reflect.ValueOf(value)
			for i := 0; i < rv.Len(); i++ {
				if item := rv.Index(i).Interface(); pyKey(item) != nil {
					facets.add(axis, item)
				}
			}
		} else if pyKey(value) != nil {
			facets.add(axis, value)
		}
	}

	return facets
}

// Has mirrors `has`: whether the event carries a value on a dimension.
func Has(event *ARLogEvent, dimension log.AREventKey, value any) bool {
	if getter, ok := envelope[dimension]; ok {
		return pyEqual(getter(event), value)
	}

	// Facets are stored as `(axis, value)`, so this is a single lookup
	return event.Facets().Has(dimension, value)
}

// Prescreen mirrors `prescreen`: whether an untranslated entry can still
// carry a value.
func Prescreen(entry *log.ARLogEntry, dimension log.AREventKey, value any) bool {
	getter, ok := entryGetters[dimension]
	if !ok {
		return true
	}

	return pyEqual(getter(entry), value)
}

// Resolve mirrors `resolve`: the event's value(s) on a dimension (empty
// when absent), each value once.
func Resolve(event *ARLogEvent, dimension log.AREventKey) []any {
	if getter, ok := envelope[dimension]; ok {
		return []any{getter(event)}
	}

	var out []any
	for _, facet := range event.Facets().Items() {
		if facet.Axis == dimension {
			out = append(out, facet.Value)
		}
	}
	return out
}
