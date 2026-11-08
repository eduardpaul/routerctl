// Package source mirrors asusrouter/modules/source/__init__.py.
//
// Python distinguishes `ARDataSource` (class instances) from `ARDataType`
// (enum members used as requests). Go folds both into the DataSource
// interface: anything with a source name and a key can be requested.
package source

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// DataSource is the Go form of `ARDataSource` / `ARDataType`.
//
// SourceName is the Python class name (for example "ARWiFiSource"); it is
// what the registry and the parity output are keyed by. SourceKey is the
// Python `_key()` - the defining fields; router-global sources return nil.
type DataSource interface {
	SourceName() string
	SourceKey() []any
}

// Base is embedded by router-global sources (empty `_key`).
type Base struct{}

// SourceKey returns the empty key of a router-global source.
func (Base) SourceKey() []any { return nil }

// ID is a comparable identity for a DataSource: equal by exact type
// (source name) and key, like Python `__eq__` / `__hash__`.
type ID string

// IDOf returns the identity of a source.
func IDOf(src DataSource) ID {
	if src == nil {
		return ""
	}
	key := src.SourceKey()
	if len(key) == 0 {
		return ID(src.SourceName())
	}
	parts := make([]string, len(key))
	for i, k := range key {
		parts[i] = keyString(k)
	}
	return ID(src.SourceName() + "|" + strings.Join(parts, "|"))
}

func keyString(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case fmt.Stringer:
		return t.String()
	default:
		// Dereference pointers so equal values give equal IDs
		return fmt.Sprintf("%v", deref(v))
	}
}

func deref(v any) any {
	switch t := v.(type) {
	case *string:
		if t == nil {
			return "None"
		}
		return *t
	case *int:
		if t == nil {
			return "None"
		}
		return *t
	case *bool:
		if t == nil {
			return "None"
		}
		return *t
	}
	return v
}

// Repr mirrors `ARDataSource.__repr__`.
func Repr(src DataSource) string {
	key := src.SourceKey()
	if len(key) == 0 {
		return "<" + src.SourceName() + ">"
	}
	parts := make([]string, len(key))
	for i, k := range key {
		parts[i] = keyString(k)
	}
	return fmt.Sprintf("<%s (%s)>", src.SourceName(), strings.Join(parts, ", "))
}

// ARDataTypeGenericUnknown mirrors `ARDataTypeGeneric.UNKNOWN`.
// (The enum itself is generated in source_enums_gen.go.)

// ARDataCollection mirrors `ARDataCollection`: several sources managed
// together.
type ARDataCollection []DataSource

// ARDataCollectionFromValue mirrors `ARDataCollection.from_value`: a single
// source, or a slice of sources (non-sources are dropped with a warning).
func ARDataCollectionFromValue(value any) ARDataCollection {
	switch v := value.(type) {
	case nil:
		return nil
	case DataSource:
		return ARDataCollection{v}
	case ARDataCollection:
		return v
	case []DataSource:
		return ARDataCollection(v)
	case []any:
		var valid ARDataCollection
		var invalid []any
		for _, item := range v {
			if s, ok := item.(DataSource); ok {
				valid = append(valid, s)
			} else {
				invalid = append(invalid, item)
			}
		}
		if len(invalid) > 0 {
			slog.Warn("Ignored invalid items for ARDataCollection", "items", invalid)
		}
		return valid
	}
	return nil
}

// ARDataState mirrors `ARDataState`: the cached content of one source and
// the time of its last update, plus an in-flight refresh marker.
type ARDataState struct {
	mu         sync.Mutex
	source     DataSource
	content    any
	lastUpdate time.Time
	refreshing chan struct{}
}

// NewARDataState creates the state for a source.
func NewARDataState(src DataSource) *ARDataState {
	return &ARDataState{source: src}
}

// Source returns the state's data source.
func (s *ARDataState) Source() DataSource { return s.source }

// Update stores new content and stamps the update time.
func (s *ARDataState) Update(content any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.content = content
	s.lastUpdate = time.Now()
}

// Expire marks the cached content stale so the next read refetches it.
func (s *ARDataState) Expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUpdate = time.Time{}
}

// IsFresh reports whether the content is younger than threshold.
func (s *ARDataState) IsFresh(threshold time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastUpdate.IsZero() {
		return false
	}
	return time.Since(s.lastUpdate) < threshold
}

// Content returns the cached content.
func (s *ARDataState) Content() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.content
}

// LastUpdate returns the time of the last update (zero if never).
func (s *ARDataState) LastUpdate() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUpdate
}

// BeginRefresh marks the state as being refreshed. It returns false when
// another refresh is already in flight (the caller should wait instead).
func (s *ARDataState) BeginRefresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshing != nil {
		return false
	}
	s.refreshing = make(chan struct{})
	return true
}

// EndRefresh finishes a refresh and wakes any waiters.
func (s *ARDataState) EndRefresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshing != nil {
		close(s.refreshing)
		s.refreshing = nil
	}
}

// Refreshing reports whether a refresh is in flight.
func (s *ARDataState) Refreshing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshing != nil
}

// WaitRefresh returns a channel closed when the in-flight refresh ends
// (an already-closed channel when idle).
func (s *ARDataState) WaitRefresh() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshing == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return s.refreshing
}
