package query

import (
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

var (
	predMac  = mustMac("aa:bb:cc:00:00:01")
	predWhen = naive(2026, 7, 22, 8, 43, 21)
)

// predEvent mirrors `_event(**overrides)`: mutate the defaults.
func predEvent(override func(*eventOpts)) *ARLogEvent {
	o := eventOpts{
		timestamp:   timePtr(predWhen),
		program:     log.ARProgramNtp,
		programName: strPtr("ntp"),
		eventType:   translate.AREventNtpStartUpdate,
		raw:         "start NTP update",
		data:        map[log.AREventKey]any{log.AREventKeyClientMac: &predMac},
	}
	if override != nil {
		override(&o)
	}
	return newEvent(o)
}

func predEntry() *log.ARLogEntry {
	return &log.ARLogEntry{
		TimestampStr: "Jul 22 08:43:21",
		Timestamp:    timePtr(predWhen),
		Program:      &log.ARLogProgram{Name: "ntp"},
		Content:      security.NewSensitiveText("start NTP update"),
	}
}

func check(t *testing.T, got, want bool) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatch(t *testing.T) {
	t.Run("test_facet_hit_cross_format", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyClientMac, "aa-bb-cc-00-00-01").Matches(predEvent(nil)), true)
	})
	t.Run("test_facet_miss", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyClientMac, "ff:ff:ff:ff:ff:ff").Matches(predEvent(nil)), false)
	})
	t.Run("test_program_dimension", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyProgram, "ntp").Matches(predEvent(nil)), true)
	})
	t.Run("test_event_type_via_string", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyEventType, "start_update").Matches(predEvent(nil)), true)
	})
}

func TestBetween(t *testing.T) {
	cest := time.FixedZone("", 2*3600)
	at := func(tz *time.Location) *ARLogEvent {
		return predEvent(func(o *eventOpts) {
			stamp := time.Date(2026, 7, 22, 8, 43, 21, 0, tz)
			o.timestamp = &stamp
		})
	}
	window := func(tz *time.Location) ARLogBetween {
		return ARLogBetween{
			After:  timePtr(time.Date(2026, 7, 22, 8, 0, 0, 0, tz)),
			Before: timePtr(time.Date(2026, 7, 22, 9, 0, 0, 0, tz)),
		}
	}
	naiveTZ := predWhen.Location()

	t.Run("test_inside", func(t *testing.T) {
		check(t, window(naiveTZ).Matches(predEvent(nil)), true)
	})
	t.Run("test_before_only", func(t *testing.T) {
		check(t, ARLogBetween{Before: timePtr(predWhen)}.Matches(predEvent(nil)), true)
	})
	t.Run("test_after_excludes", func(t *testing.T) {
		check(t, ARLogBetween{After: timePtr(naive(2026, 7, 22, 9, 0, 0))}.Matches(predEvent(nil)), false)
	})
	t.Run("test_no_timestamp", func(t *testing.T) {
		check(t, ARLogBetween{Before: timePtr(predWhen)}.Matches(predEvent(func(o *eventOpts) { o.timestamp = nil })), false)
	})
	t.Run("test_aware_bound_against_aware_stamp", func(t *testing.T) {
		check(t, window(cest).Matches(at(cest)), true)
	})
	tests := []struct {
		name            string
		stampTZ, boundT *time.Location
	}{
		{"aware_stamp_naive_bound", cest, naiveTZ},
		{"naive_stamp_aware_bound", naiveTZ, time.UTC},
	}
	for _, tt := range tests {
		t.Run("test_mismatched_awareness_reads_as_wall_clock/"+tt.name, func(t *testing.T) {
			check(t, window(tt.boundT).Matches(at(tt.stampTZ)), true)
		})
	}
}

func TestContains(t *testing.T) {
	t.Run("test_hit", func(t *testing.T) {
		check(t, NewARLogContains("NTP UPDATE").Matches(predEvent(func(o *eventOpts) { o.raw = "start NTP update" })), true)
	})
	t.Run("test_miss", func(t *testing.T) {
		check(t, NewARLogContains("reboot").Matches(predEvent(nil)), false)
	})
	t.Run("test_text_folded_once", func(t *testing.T) {
		if got := NewARLogContains("NTP Update").Text; got != "ntp update" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestCombinators(t *testing.T) {
	program := func(v string) ARLogPredicate { return NewARLogMatch(log.AREventKeyProgram, v) }
	tests := []struct {
		name      string
		predicate ARLogPredicate
		want      bool
	}{
		{"test_all_true", NewARLogAll(program("ntp"), NewARLogMatch(log.AREventKeyClientMac, predMac)), true},
		{"test_all_one_fails", NewARLogAll(program("ntp"), program("kernel")), false},
		{"test_any_one_matches", NewARLogAny(program("kernel"), program("ntp")), true},
		{"test_any_none", NewARLogAny(program("kernel")), false},
		{"test_not_inverts", NewARLogNot(program("kernel")), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(t, tt.predicate.Matches(predEvent(nil)), tt.want)
		})
	}
}

func TestPrescreen(t *testing.T) {
	program := func(v string) ARLogMatch { return NewARLogMatch(log.AREventKeyProgram, v) }
	t.Run("test_may_match_without_prescreen", func(t *testing.T) {
		check(t, MayMatch(NewARLogNot(NewARLogContains("x")), predEntry()), true)
	})
	t.Run("test_match_on_program", func(t *testing.T) {
		check(t, program("ntp").Prescreen(predEntry()), true)
		check(t, program("kernel").Prescreen(predEntry()), false)
	})
	t.Run("test_match_on_program_name", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyProgramName, "ntp").Prescreen(predEntry()), true)
	})
	t.Run("test_match_on_translated_dimension_keeps_entry", func(t *testing.T) {
		check(t, NewARLogMatch(log.AREventKeyClientMac, predMac).Prescreen(predEntry()), true)
	})
	t.Run("test_between_uses_entry_timestamp", func(t *testing.T) {
		check(t, ARLogBetween{Before: timePtr(predWhen)}.Prescreen(predEntry()), true)
		check(t, ARLogBetween{After: timePtr(naive(2026, 7, 22, 9, 0, 0))}.Prescreen(predEntry()), false)
	})
	t.Run("test_contains_uses_entry_content", func(t *testing.T) {
		check(t, NewARLogContains("NTP").Prescreen(predEntry()), true)
		check(t, NewARLogContains("reboot").Prescreen(predEntry()), false)
	})
	t.Run("test_all_requires_every_sub", func(t *testing.T) {
		check(t, NewARLogAll(program("ntp"), NewARLogContains("ntp")).Prescreen(predEntry()), true)
		check(t, NewARLogAll(program("ntp"), NewARLogContains("reboot")).Prescreen(predEntry()), false)
	})
	t.Run("test_any_requires_one_sub", func(t *testing.T) {
		check(t, NewARLogAny(program("kernel"), NewARLogContains("ntp")).Prescreen(predEntry()), true)
		check(t, NewARLogAny(program("kernel"), NewARLogContains("reboot")).Prescreen(predEntry()), false)
	})
}

func TestEverything(t *testing.T) {
	t.Run("test_matches_any_event", func(t *testing.T) {
		check(t, ARLogEverything{}.Matches(predEvent(nil)), true)
		check(t, ARLogEverything{}.Matches(predEvent(func(o *eventOpts) {
			o.timestamp = nil
			o.data = map[log.AREventKey]any{}
		})), true)
	})
	t.Run("test_prescreens_any_entry", func(t *testing.T) {
		check(t, ARLogEverything{}.Prescreen(predEntry()), true)
		check(t, MayMatch(ARLogEverything{}, predEntry()), true)
	})
}
