package query

// Predicates for querying processed log events.

import (
	"strings"
	"time"
	"unicode"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

// ARLogPredicate mirrors `ARLogPredicate`: a test applied to a single
// event.
type ARLogPredicate interface {
	// Matches reports whether the event satisfies the predicate
	Matches(event *ARLogEvent) bool
}

// Prescreener is the optional half of `ARLogPredicate` (Python looks it
// up with `getattr(predicate, "prescreen", None)`).
type Prescreener interface {
	// Prescreen reports whether the untranslated entry can still match
	Prescreen(entry *log.ARLogEntry) bool
}

// MayMatch mirrors `may_match`: whether a predicate keeps an entry in
// play before translation.
func MayMatch(predicate ARLogPredicate, entry *log.ARLogEntry) bool {
	check, ok := predicate.(Prescreener)
	if !ok {
		return true
	}

	return check.Prescreen(entry)
}

// ARLogMatch mirrors `ARLogMatch`: match a dimension against a value
// (equality on the query axis). Build it with NewARLogMatch, which
// coerces the value like the Python `__post_init__`.
type ARLogMatch struct {
	Dimension log.AREventKey
	Value     any
}

// NewARLogMatch coerces the query value to the dimension's own type.
func NewARLogMatch(dimension log.AREventKey, value any) ARLogMatch {
	return ARLogMatch{Dimension: dimension, Value: Coerce(dimension, value)}
}

// Matches reports whether the event carries the value on this dimension.
func (m ARLogMatch) Matches(event *ARLogEvent) bool {
	return Has(event, m.Dimension, m.Value)
}

// Prescreen reports whether the entry can carry the value before
// translation.
func (m ARLogMatch) Prescreen(entry *log.ARLogEntry) bool {
	return Prescreen(entry, m.Dimension, m.Value)
}

// sameFrame reads a window bound in the same frame as the timestamp it
// meets (Python `bound.replace(tzinfo=timestamp.tzinfo)`).
func sameFrame(bound *time.Time, timestamp time.Time) *time.Time {
	if bound == nil || isNaive(*bound) == isNaive(timestamp) {
		return bound
	}

	b := *bound
	moved := time.Date(b.Year(), b.Month(), b.Day(), b.Hour(), b.Minute(), b.Second(), b.Nanosecond(), timestamp.Location())
	return &moved
}

// ARLogBetween mirrors `ARLogBetween`: match events whose timestamp
// falls in an inclusive window (nil bounds are open).
type ARLogBetween struct {
	After  *time.Time
	Before *time.Time
}

// Matches reports whether the event's timestamp lies within the window.
func (b ARLogBetween) Matches(event *ARLogEvent) bool {
	return b.holds(event.Timestamp)
}

// Prescreen reports whether the entry's timestamp lies within the window.
func (b ARLogBetween) Prescreen(entry *log.ARLogEntry) bool {
	return b.holds(entry.Timestamp)
}

// holds reports whether a timestamp lies within the window.
func (b ARLogBetween) holds(timestamp *time.Time) bool {
	if timestamp == nil {
		return false
	}

	after := sameFrame(b.After, *timestamp)
	before := sameFrame(b.Before, *timestamp)
	if after != nil && timestamp.Before(*after) {
		return false
	}

	return before == nil || !timestamp.After(*before)
}

// ARLogContains mirrors `ARLogContains`: match events whose raw content
// holds a substring (case-folded). Build it with NewARLogContains.
type ARLogContains struct {
	Text string
}

// NewARLogContains case-folds the query text once.
func NewARLogContains(text string) ARLogContains {
	return ARLogContains{Text: casefold(text)}
}

// Matches reports whether the raw content contains the text.
func (c ARLogContains) Matches(event *ARLogEvent) bool {
	return strings.Contains(casefold(event.RawText()), c.Text)
}

// Prescreen reports whether the entry contains the text.
func (c ARLogContains) Prescreen(entry *log.ARLogEntry) bool {
	return strings.Contains(casefold(entry.ContentValue()), c.Text)
}

// ARLogEverything mirrors `ARLogEverything`: keep every event,
// translating the log without filtering it.
type ARLogEverything struct{}

// Matches always matches.
func (ARLogEverything) Matches(*ARLogEvent) bool { return true }

// Prescreen always keeps the entry in play.
func (ARLogEverything) Prescreen(*log.ARLogEntry) bool { return true }

// ARLogAll mirrors `ARLogAll`: match events satisfying every
// sub-predicate (AND).
type ARLogAll struct {
	Predicates []ARLogPredicate
}

// NewARLogAll stores the sub-predicates to AND together.
func NewARLogAll(predicates ...ARLogPredicate) ARLogAll {
	return ARLogAll{Predicates: predicates}
}

// Matches reports whether every sub-predicate matches.
func (a ARLogAll) Matches(event *ARLogEvent) bool {
	for _, p := range a.Predicates {
		if !p.Matches(event) {
			return false
		}
	}
	return true
}

// Prescreen reports whether every sub-predicate keeps the entry in play.
func (a ARLogAll) Prescreen(entry *log.ARLogEntry) bool {
	for _, p := range a.Predicates {
		if !MayMatch(p, entry) {
			return false
		}
	}
	return true
}

// ARLogAny mirrors `ARLogAny`: match events satisfying any sub-predicate
// (OR).
type ARLogAny struct {
	Predicates []ARLogPredicate
}

// NewARLogAny stores the sub-predicates to OR together.
func NewARLogAny(predicates ...ARLogPredicate) ARLogAny {
	return ARLogAny{Predicates: predicates}
}

// Matches reports whether at least one sub-predicate matches.
func (a ARLogAny) Matches(event *ARLogEvent) bool {
	for _, p := range a.Predicates {
		if p.Matches(event) {
			return true
		}
	}
	return false
}

// Prescreen reports whether any sub-predicate keeps the entry in play.
func (a ARLogAny) Prescreen(entry *log.ARLogEntry) bool {
	for _, p := range a.Predicates {
		if MayMatch(p, entry) {
			return true
		}
	}
	return false
}

// ARLogNot mirrors `ARLogNot`: match events failing the wrapped
// predicate (negation). It has no prescreen, like the Python class.
type ARLogNot struct {
	Predicate ARLogPredicate
}

// NewARLogNot stores the predicate to negate.
func NewARLogNot(predicate ARLogPredicate) ARLogNot {
	return ARLogNot{Predicate: predicate}
}

// Matches reports whether the wrapped predicate does not match.
func (n ARLogNot) Matches(event *ARLogEvent) bool {
	return !n.Predicate.Matches(event)
}

// Go-only helpers

// isNaive reports whether a time states no offset (Python `tzinfo is
// None`).
func isNaive(t time.Time) bool {
	return t.Location() == consts.NaiveLocation
}

// casefold approximates Python `str.casefold`: Go has only the simple
// case folding, so lower(upper(c)) covers the simple mappings (`ſ`, `µ`,
// `ς`, ...) and the one full folding a log text plausibly holds (`ß`)
// is added by hand.
func casefold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'ß', 'ẞ':
			b.WriteString("ss")
			continue
		}
		b.WriteRune(unicode.ToLower(unicode.ToUpper(r)))
	}
	return b.String()
}
