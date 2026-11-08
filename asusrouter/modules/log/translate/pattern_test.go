package translate

// Tests for the declarative message pattern engine.

import (
	"errors"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
)

// testEvent is a minimal event enum for the engine tests.
type testEvent string

func (e testEvent) Name() string { return strings.ToUpper(string(e)) }
func (e testEvent) Valid() bool  { return true }

const (
	evUnknown testEvent = "unknown"
	evHit     testEvent = "hit"
	evOther   testEvent = "other"
)

func mustPattern(t *testing.T, event any, pattern string, opts PatternOptions) *ARLogPattern {
	t.Helper()
	p, err := NewARLogPattern(event, pattern, opts)
	if err != nil {
		t.Fatalf("NewARLogPattern(%q): %v", pattern, err)
	}
	return p
}

var strip = Plain(func(s string) any { return converters.PyStrip(s) })

func TestRead(t *testing.T) {
	t.Run("fields_named_after_keys", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+) on (?P<chain>\w+)`, PatternOptions{})
		assertEvent(t, p.Read("port 80 on FORWARD", evUnknown), ev{
			log.AREventKeyEventType: evHit,
			log.AREventKeyRaw:       "port 80 on FORWARD",
			log.AREventKeyPort:      "80",
			log.AREventKeyChain:     "FORWARD",
		})
	})
	t.Run("converter_applied", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
		})
		eq(t, p.Read("port 80", evUnknown), log.AREventKeyPort, 80)
	})
	t.Run("no_match", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+)`, PatternOptions{})
		if got := p.Read("nothing here", evUnknown); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
	t.Run("absent_group_omitted", func(t *testing.T) {
		p := mustPattern(t, evHit, `link(?: at (?P<speed>\d+))?`, PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeySpeed: Int},
		})
		absent(t, p.Read("link", evUnknown), log.AREventKeySpeed)
	})
	t.Run("converter_none_omitted", func(t *testing.T) {
		p := mustPattern(t, evHit, `speed (?P<speed>\w+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeySpeed: Plain(func(string) any { return nil }),
			},
		})
		absent(t, p.Read("speed fast", evUnknown), log.AREventKeySpeed)
	})
}

func TestEventType(t *testing.T) {
	t.Run("mapped_by_group", func(t *testing.T) {
		p := mustPattern(t, map[string]StrEnum{"up": evHit, "down": evOther},
			`link (?P<event_type>up|down)`, PatternOptions{})
		assertEvent(t, p.Read("link down", evUnknown), ev{
			log.AREventKeyEventType: evOther,
			log.AREventKeyRaw:       "link down",
		})
	})
	t.Run("unmapped_value_falls_back", func(t *testing.T) {
		p := mustPattern(t, map[string]StrEnum{"up": evHit}, `link (?P<event_type>\w+)`, PatternOptions{})
		eq(t, p.Read("link sideways", evUnknown), log.AREventKeyEventType, evUnknown)
	})
}

func TestDerive(t *testing.T) {
	t.Run("derived_from_read_key", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
			Derive: []Derived{{
				Key: log.AREventKeyValue, Source: log.AREventKeyPort,
				Func: func(v any) any { return v.(int) * 2 },
			}},
		})
		eq(t, p.Read("port 21", evUnknown), log.AREventKeyValue, 42)
	})
	t.Run("absent_source_skips", func(t *testing.T) {
		p := mustPattern(t, evHit, `link(?: at (?P<speed>\d+))?`, PatternOptions{
			Derive: []Derived{{
				Key: log.AREventKeyValue, Source: log.AREventKeySpeed,
				Func: func(v any) any { return converters.PyStrip(v.(string)) },
			}},
		})
		absent(t, p.Read("link", evUnknown), log.AREventKeyValue)
	})
}

func TestMarker(t *testing.T) {
	t.Run("missing_marker_skips", func(t *testing.T) {
		p := mustPattern(t, evHit, `(?P<port>\d+)`, PatternOptions{Marker: "port"})
		if got := p.Read("80", evUnknown); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
		if got := p.Read("port 80", evUnknown); got == nil {
			t.Fatal("got nil, want a match")
		}
	})
}

func TestPlanValidation(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		opts    PatternOptions
		match   string
	}{
		{"group_without_key", `(?P<not_a_key>\d+)`, PatternOptions{}, "without an event key"},
		{"unused_converter", `(?P<port>\d+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyChain: strip},
		}, "never reads"},
		{"derive_from_unread_key", `(?P<port>\d+)`, PatternOptions{
			Derive: []Derived{{
				Key: log.AREventKeyValue, Source: log.AREventKeyChain,
				Func: func(v any) any { return v },
			}},
		}, "never reads"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewARLogPattern(evHit, tt.pattern, tt.opts)
			if !errors.Is(err, ErrPattern) || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("err = %v, want ErrPattern matching %q", err, tt.match)
			}
		})
	}
}

func TestRawClassification(t *testing.T) {
	macLevel := identifiers.MacAddress{}.RevealLevel()

	t.Run("no_sensitive_field_stays_plain", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+)`, PatternOptions{})
		eq(t, p.Read("port 80", evUnknown), log.AREventKeyRaw, "port 80")
	})
	t.Run("untyped_converter_stays_plain", func(t *testing.T) {
		p := mustPattern(t, evHit, `port (?P<port>\d+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{log.AREventKeyPort: Int},
		})
		eq(t, p.Read("port 80", evUnknown), log.AREventKeyRaw, "port 80")
	})
	t.Run("sensitive_field_classifies_raw", func(t *testing.T) {
		p := mustPattern(t, evHit, `sta (?P<client_mac>`+MacPattern+`)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
			},
		})
		content := "sta AA:BB:CC:00:00:01"
		eq(t, p.Read(content, evUnknown), log.AREventKeyRaw, classified(content, macLevel))
	})
	t.Run("strictest_field_wins", func(t *testing.T) {
		p := mustPattern(t, evHit, `(?P<hostname>\w+) as (?P<subject>\w+)`, PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyHostname: FromValueSafe(identifiers.HostnameFromValueSafe),
				log.AREventKeySubject:  FromValueSafe(identifiers.UsernameFromValueSafe),
			},
		})
		content := "myhost as someone"
		// A hostname reveals at REASONABLE, a login name only at UNSAFE
		eq(t, p.Read(content, evUnknown), log.AREventKeyRaw,
			classified(content, identifiers.Username{}.RevealLevel()))
	})
	t.Run("planned_field_counts_when_absent", func(t *testing.T) {
		p := mustPattern(t, evHit, `link(?: (?P<client_mac>`+MacPattern+`))?`, PatternOptions{
			Convert: map[log.AREventKey]Converter{
				log.AREventKeyClientMac: FromValueSafe(identifiers.MacAddressFromValueSafe),
			},
		})
		event := p.Read("link", evUnknown)
		absent(t, event, log.AREventKeyClientMac)
		eq(t, event, log.AREventKeyRaw, classified("link", macLevel))
	})
	t.Run("derived_field_counts", func(t *testing.T) {
		// Go cannot read the level off the function, so the derived field
		// takes it from the identifier's converter
		c := FromValueSafe(identifiers.MacAddressFromValueSafe)
		p := mustPattern(t, evHit, `sta (?P<value>\S+)`, PatternOptions{
			Derive: []Derived{{
				Key: log.AREventKeyClientMac, Source: log.AREventKeyValue,
				Func:  func(v any) any { return c.Func(v.(string)) },
				Level: c.Level,
			}},
		})
		content := "sta AA:BB:CC:00:00:01"
		eq(t, p.Read(content, evUnknown), log.AREventKeyRaw, classified(content, macLevel))
	})
}

func TestPatternSet(t *testing.T) {
	t.Run("first_match_wins", func(t *testing.T) {
		set := NewARLogPatternSet(evUnknown,
			mustPattern(t, evOther, `never`, PatternOptions{}),
			mustPattern(t, evHit, `line`, PatternOptions{}),
		)
		assertEvent(t, set.Translate("line"), ev{
			log.AREventKeyEventType: evHit,
			log.AREventKeyRaw:       "line",
		})
	})
	t.Run("unknown_keeps_raw", func(t *testing.T) {
		set := NewARLogPatternSet(evUnknown, mustPattern(t, evHit, `never`, PatternOptions{}))
		assertEvent(t, set.Translate("line"), ev{
			log.AREventKeyEventType: evUnknown,
			log.AREventKeyRaw:       unread("line"),
		})
	})
	t.Run("no_patterns_is_the_fallback", func(t *testing.T) {
		assertEvent(t, NewARLogPatternSet(evUnknown).Translate("line"), ev{
			log.AREventKeyEventType: evUnknown,
			log.AREventKeyRaw:       unread("line"),
		})
	})
}
