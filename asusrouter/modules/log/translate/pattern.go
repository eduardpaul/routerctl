package translate

// Declarative message patterns for log event translation.

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/internal/pyre"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// EventGroup mirrors `EVENT_GROUP`.
// The group picking the event type when a pattern covers several of them
var EventGroup = string(log.AREventKeyEventType)

// Error texts of a rejected pattern plan
const (
	ErrorDeriveSource    = "derives from a key the pattern never reads"
	ErrorGroupKey        = "names a group without an event key"
	ErrorUnusedConverter = "converts a key the pattern never reads"
)

// ErrPattern wraps every error a pattern plan is rejected with (Python
// raises ValueError).
var ErrPattern = errors.New("invalid log pattern")

// StrEnum is the Go form of the Python `StrEnum` event types: every
// generated event enum implements it.
type StrEnum interface {
	Name() string
	Valid() bool
}

// Converter is one field converter (Python `Callable[[str], Any]`).
//
// Python finds the level of the sensitive type a converter builds by the
// class a `X.from_value_safe` is bound to; Go cannot look that up, so the
// converter carries it.
type Converter struct {
	// Func returns nil for Python None
	Func func(string) any
	// Level is the reveal level of the sensitive type Func builds, nil
	// when it builds nothing sensitive
	Level *security.ARSecurityLevel
}

// Plain wraps a converter building nothing sensitive (a plain function).
func Plain(f func(string) any) Converter { return Converter{Func: f} }

// revealer is a sensitive type (Python `ARSensitive.reveal_level`).
type revealer interface {
	RevealLevel() security.ARSecurityLevel
}

// FromValueSafe wraps an identifier's `from_value_safe` classmethod: a
// nil result is None, and a sensitive identifier lends its reveal level.
func FromValueSafe[T any](fromValueSafe func(any) *T) Converter {
	var zero T
	var level *security.ARSecurityLevel
	if r, ok := any(&zero).(revealer); ok {
		l := r.RevealLevel()
		level = &l
	}
	return Converter{
		Func: func(s string) any {
			if v := fromValueSafe(s); v != nil {
				return v
			}
			return nil
		},
		Level: level,
	}
}

// Int mirrors the builtin `int` as a converter (a match the pattern
// guarantees to be digits never fails; one that does gives None).
var Int = Plain(func(s string) any {
	if v, ok := pyre.Int(s, 10); ok {
		return v
	}
	return nil
})

// IntBase16 mirrors `partial(int, base=16)`.
var IntBase16 = Plain(func(s string) any {
	if v, ok := pyre.Int(s, 16); ok {
		return v
	}
	return nil
})

// Derived is one derived field (Python `derive={key: (source, func)}`):
// the key it fills, the key it reads and its function.
type Derived struct {
	Key    log.AREventKey
	Source log.AREventKey
	Func   func(any) any
	// Level as in Converter (nil for a plain function)
	Level *security.ARSecurityLevel
}

// PatternOptions are the keyword arguments of `ARLogPattern`.
type PatternOptions struct {
	Convert map[log.AREventKey]Converter
	// Derive keeps the Python dict order
	Derive []Derived
	// Marker "" is Python None (an empty marker is in every message)
	Marker string
}

// One planned field: the key it fills, its group number and its converter
type field struct {
	key     log.AREventKey
	number  int
	convert *Converter
}

// ARLogPattern mirrors `ARLogPattern`: one recognized message shape and
// the event it yields.
type ARLogPattern struct {
	derived  []Derived
	event    StrEnum
	events   map[string]StrEnum
	fields   []field
	marker   string
	rawLevel *security.ARSecurityLevel
	regex    *regexp.Regexp
	pattern  string
}

// NewARLogPattern compiles the pattern and plans the fields it fills.
// event is a StrEnum, or a map[string]StrEnum picking the event type
// from the `event_type` group. The error wraps ErrPattern.
func NewARLogPattern(event any, pattern string, opts PatternOptions) (*ARLogPattern, error) {
	translated, err := pyre.Translate(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPattern, err)
	}
	regex, err := regexp.Compile(translated)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPattern, err)
	}
	p := &ARLogPattern{regex: regex, pattern: pattern, marker: opts.Marker}

	// A mapping picks the event type from the `event_type` group
	switch e := event.(type) {
	case map[string]StrEnum:
		p.events = e
	case StrEnum:
		p.event = e
	default:
		return nil, fmt.Errorf("%w: event %T is neither a StrEnum nor a mapping", ErrPattern, event)
	}

	convert := opts.Convert
	// Groups are read by number: the name only plans the field
	for number, name := range regex.SubexpNames() {
		if name == "" || name == EventGroup {
			continue
		}
		key, err := p.groupKey(name)
		if err != nil {
			return nil, err
		}
		f := field{key: key, number: number}
		if c, ok := convert[key]; ok {
			c := c
			f.convert = &c
		}
		p.fields = append(p.fields, f)
	}

	p.derived = append(p.derived, opts.Derive...)

	read := map[log.AREventKey]bool{}
	for _, f := range p.fields {
		read[f.key] = true
	}
	var unused []log.AREventKey
	for key := range convert {
		if !read[key] {
			unused = append(unused, key)
		}
	}
	if err := p.reject(unused, ErrorUnusedConverter); err != nil {
		return nil, err
	}
	var sources []log.AREventKey
	seen := map[log.AREventKey]bool{}
	for _, d := range p.derived {
		if !read[d.Source] && !seen[d.Source] {
			seen[d.Source] = true
			sources = append(sources, d.Source)
		}
	}
	if err := p.reject(sources, ErrorDeriveSource); err != nil {
		return nil, err
	}

	p.rawLevel = p.classify()

	return p, nil
}

// MustARLogPattern is NewARLogPattern panicking on error, for the
// package-level pattern tables (Python raises at import).
func MustARLogPattern(event any, pattern string, opts PatternOptions) *ARLogPattern {
	p, err := NewARLogPattern(event, pattern, opts)
	if err != nil {
		panic(err)
	}
	return p
}

// classify finds the level the raw message is exposed from, if at all.
func (p *ARLogPattern) classify() *security.ARSecurityLevel {
	var levels []security.ARSecurityLevel
	for _, f := range p.fields {
		if f.convert != nil && f.convert.Level != nil {
			levels = append(levels, *f.convert.Level)
		}
	}
	for _, d := range p.derived {
		if d.Level != nil {
			levels = append(levels, *d.Level)
		}
	}

	if len(levels) == 0 {
		return nil
	}
	highest := levels[0]
	for _, l := range levels[1:] {
		if l > highest {
			highest = l
		}
	}
	return &highest
}

// groupKey resolves a group name into the event key it fills.
func (p *ARLogPattern) groupKey(name string) (log.AREventKey, error) {
	key := log.AREventKeyFromValue(name)
	if key == log.AREventKeyUnknown {
		return key, fmt.Errorf("%w: %s %s: %s", ErrPattern, converters.PyRepr(p.pattern), ErrorGroupKey, name)
	}

	return key, nil
}

// reject rejects a plan naming a key the pattern does not fill.
func (p *ARLogPattern) reject(unknown []log.AREventKey, message string) error {
	if len(unknown) == 0 {
		return nil
	}
	keys := make([]string, len(unknown))
	for i, k := range unknown {
		keys[i] = string(k)
	}
	sort.Strings(keys)
	return fmt.Errorf("%w: %s %s: %s", ErrPattern, converters.PyRepr(p.pattern), message, strings.Join(keys, ", "))
}

// Read mirrors `read`: read the event out of a message, or nil when it
// does not match.
func (p *ARLogPattern) Read(content string, unknown StrEnum) map[log.AREventKey]any {
	if p.marker != "" && !strings.Contains(content, p.marker) {
		return nil
	}

	match := p.regex.FindStringSubmatchIndex(content)
	if match == nil {
		return nil
	}

	// The event type is either fixed or picked by the `event_type` group
	var eventType StrEnum = p.event
	if p.events != nil {
		eventType = unknown
		if picked, ok := pyre.Group(p.regex, content, match, EventGroup); ok {
			if e, found := p.events[picked]; found {
				eventType = e
			}
		}
	}
	event := map[log.AREventKey]any{
		log.AREventKeyEventType: eventType,
	}
	// A shape carrying nothing sensitive is understood in full, so its
	// message needs no protection of its own
	if p.rawLevel == nil {
		event[log.AREventKeyRaw] = content
	} else {
		event[log.AREventKeyRaw] = security.SensitiveText{Value: content, Level: *p.rawLevel}
	}
	for _, f := range p.fields {
		if match[2*f.number] < 0 {
			continue
		}
		raw := content[match[2*f.number]:match[2*f.number+1]]
		var value any = raw
		if f.convert != nil {
			value = f.convert.Func(raw)
		}
		if value != nil {
			event[f.key] = value
		}
	}

	for _, d := range p.derived {
		if value, ok := event[d.Source]; ok && value != nil {
			event[d.Key] = d.Func(value)
		}
	}

	return event
}

// ARLogPatternSet mirrors `ARLogPatternSet`: every message shape one
// program emits, with its fallback type.
type ARLogPatternSet struct {
	patterns []*ARLogPattern
	unknown  StrEnum
}

// NewARLogPatternSet holds the patterns to try and the type an unread
// message gets.
func NewARLogPatternSet(unknown StrEnum, patterns ...*ARLogPattern) *ARLogPatternSet {
	return &ARLogPatternSet{unknown: unknown, patterns: patterns}
}

// Translate mirrors `translate`: read a message with the first pattern
// that matches it.
func (s *ARLogPatternSet) Translate(content string) map[log.AREventKey]any {
	unknown := s.unknown
	for _, pattern := range s.patterns {
		if event := pattern.Read(content, unknown); event != nil {
			return event
		}
	}

	return map[log.AREventKey]any{
		log.AREventKeyEventType: unknown,
		log.AREventKeyRaw:       security.NewSensitiveText(content),
	}
}
