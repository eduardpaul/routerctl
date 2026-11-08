package query

// Processed log event envelope for AsusRouter.

import (
	"sync"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// ARLogEvent mirrors `ARLogEvent`: a translated log entry with its
// metadata and queryable facets.
type ARLogEvent struct {
	Timestamp *time.Time    `json:"timestamp"`
	Program   log.ARProgram `json:"program"`
	// raw syslog tag
	ProgramName *string           `json:"program_name"`
	EventType   translate.StrEnum `json:"event_type"`
	// plain (string) only when the translator read the message in full,
	// otherwise a security.SensitiveText classified by what it holds,
	// down to UNSAFE for an unread one
	Raw  any                    `json:"raw"`
	Data map[log.AREventKey]any `json:"data"`

	// `facets` derived from `data` on first query, never set by the caller
	facetsOnce sync.Once
	facets     *FacetSet
}

// RawText mirrors `raw_text`: the message as text, whatever level `raw`
// is exposed at.
func (e *ARLogEvent) RawText() string {
	switch raw := e.Raw.(type) {
	case security.SensitiveText:
		return raw.Value
	case string:
		return raw
	}
	return ""
}

// Facets mirrors `facets`: the queryable `(axis, value)` facets, derived
// once on demand.
func (e *ARLogEvent) Facets() *FacetSet {
	e.facetsOnce.Do(func() {
		e.facets = ExtractFacets(e.Data)
	})
	return e.facets
}

// BuildEvent mirrors `build_event`: translate one entry into an envelope
// with metadata and facets.
func BuildEvent(entry *log.ARLogEntry) *ARLogEvent {
	program := translate.ResolveProgram(entry)
	// The translator hands over a fresh map, so the metadata is moved out
	// of it instead of copying the rest into a second one
	data := translate.TranslateEntry(entry, &program)
	eventType, _ := data[log.AREventKeyEventType].(translate.StrEnum)
	raw := data[log.AREventKeyRaw]
	delete(data, log.AREventKeyEventType)
	delete(data, log.AREventKeyRaw)

	return &ARLogEvent{
		Timestamp:   entry.Timestamp,
		Program:     program,
		ProgramName: entry.ProgramName(),
		EventType:   eventType,
		Raw:         raw,
		Data:        data,
	}
}

// BuildEvents mirrors `build_events`: parse, timestamp-resolve and
// translate a raw log into envelopes. anchor may be nil.
func BuildEvents(content any, anchor *time.Time) []*ARLogEvent {
	entries := log.ResolveDatetimes(log.ParseLog(content, nil), anchor)

	events := make([]*ARLogEvent, 0, len(entries))
	for _, entry := range entries {
		events = append(events, BuildEvent(entry))
	}
	return events
}
