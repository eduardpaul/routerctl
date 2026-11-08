package query

import (
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

func mustMac(s string) identifiers.MacAddress {
	m, err := identifiers.NewMacAddress(s)
	if err != nil {
		panic(err)
	}
	return m
}

func naive(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, consts.NaiveLocation)
}

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func timePtr(t time.Time) *time.Time { return &t }

// eventOpts are the overrides of the Python `_event(**overrides)` helpers.
type eventOpts struct {
	timestamp   *time.Time
	program     log.ARProgram
	programName *string
	eventType   translate.StrEnum
	raw         any
	data        map[log.AREventKey]any
}

func newEvent(o eventOpts) *ARLogEvent {
	if o.data == nil {
		o.data = map[log.AREventKey]any{}
	}
	return &ARLogEvent{
		Timestamp:   o.timestamp,
		Program:     o.program,
		ProgramName: o.programName,
		EventType:   o.eventType,
		Raw:         o.raw,
		Data:        o.data,
	}
}

// containsKey reports whether a resolved value list holds a value (by
// Python equality).
func containsKey(values []any, want any) bool {
	for _, v := range values {
		if pyEqual(v, want) {
			return true
		}
	}
	return false
}
