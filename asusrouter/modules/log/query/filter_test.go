package query

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
)

const filterRaw = "Jul 22 08:43:21 ntp: start NTP update\n" +
	"Jul 22 08:43:22 rc_service: cfg 1:notify_rc restart_wan\n" +
	"Jul 22 08:43:23 ntp: start NTP update\n" +
	"Jul 22 08:43:24 kernel: boot\n"

func filterEntries() []*log.ARLogEntry {
	return log.ResolveDatetimes(log.ParseLog(filterRaw, nil), nil)
}

var filterNtp = NewARLogMatch(log.AREventKeyProgram, log.ARProgramNtp)

func entryList(t *testing.T, r map[log.ARLogField]any) []*log.ARLogEntry {
	t.Helper()
	v, ok := r[log.ARLogFieldEntryList].([]*log.ARLogEntry)
	if !ok {
		t.Fatalf("ENTRY_LIST %#v", r[log.ARLogFieldEntryList])
	}
	return v
}

func eventList(t *testing.T, r map[log.ARLogField]any) []*ARLogEvent {
	t.Helper()
	v, ok := r[log.ARLogFieldEventList].([]*ARLogEvent)
	if !ok {
		t.Fatalf("EVENT_LIST %#v", r[log.ARLogFieldEventList])
	}
	return v
}

func TestNoPredicate(t *testing.T) {
	t.Run("test_entry_list_and_total", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{})
		if len(entryList(t, result)) != 4 || result[log.ARLogFieldEntryCount] != 4 || result[log.ARLogFieldTotal] != 4 {
			t.Fatalf("got %v", result)
		}
		if _, ok := result[log.ARLogFieldEventList]; ok {
			t.Error("EVENT_LIST present")
		}
	})
	t.Run("test_last_slices_before_anything", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{Last: intPtr(1)})
		entries := entryList(t, result)
		if len(entries) != 1 || entries[0].ContentValue() != "boot" || result[log.ARLogFieldTotal] != 4 {
			t.Fatalf("got %v", result)
		}
	})
	t.Run("test_unique_dedups_content", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{Unique: true})
		if result[log.ARLogFieldEntryCount] != 3 || result[log.ARLogFieldTotal] != 4 {
			t.Fatalf("got %v", result)
		}
	})
	t.Run("test_programs_opt_in", func(t *testing.T) {
		if _, ok := FilterLog(filterEntries(), FilterOptions{Programs: true})[log.ARLogFieldProgramList]; !ok {
			t.Fatal("PROGRAM_LIST missing")
		}
	})
	t.Run("test_programs_grouped_over_matches_only", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{Predicate: filterNtp, Programs: true})
		groups := result[log.ARLogFieldProgramList].([]log.ARLogProgramGroup)
		if len(groups) != 1 || groups[0].Name != "ntp" || groups[0].Count != 2 {
			t.Fatalf("got %+v", groups)
		}
	})
	t.Run("test_empty", func(t *testing.T) {
		result := FilterLog(nil, FilterOptions{})
		if len(entryList(t, result)) != 0 || result[log.ARLogFieldEntryCount] != 0 ||
			result[log.ARLogFieldTotal] != 0 || lastEntry(t, result) != nil {
			t.Fatalf("got %v", result)
		}
	})
}

// onlyMatches is a predicate offering no prescreen.
type onlyMatches struct{}

func (onlyMatches) Matches(event *ARLogEvent) bool { return event.Program == log.ARProgramNtp }

// counting wraps a predicate and records the content of every event it
// is asked about (every entry that got translated).
type counting struct {
	inner      ARLogMatch
	translated []string
}

func (c *counting) Matches(event *ARLogEvent) bool {
	c.translated = append(c.translated, event.RawText())
	return c.inner.Matches(event)
}

func (c *counting) Prescreen(entry *log.ARLogEntry) bool { return c.inner.Prescreen(entry) }

func TestWithPredicate(t *testing.T) {
	t.Run("test_event_list_filtered", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{Predicate: filterNtp})
		events := eventList(t, result)
		if len(events) != 2 {
			t.Fatalf("got %d events", len(events))
		}
		for _, e := range events {
			if e.Program != log.ARProgramNtp {
				t.Errorf("program %v", e.Program)
			}
		}
		if result[log.ARLogFieldEntryCount] != 2 || result[log.ARLogFieldTotal] != 4 {
			t.Errorf("got %v", result)
		}
		if _, ok := result[log.ARLogFieldEntryList]; ok {
			t.Error("ENTRY_LIST present")
		}
	})
	t.Run("test_content_predicate", func(t *testing.T) {
		events := eventList(t, FilterLog(filterEntries(), FilterOptions{Predicate: NewARLogContains("restart_wan")}))
		if len(events) != 1 || events[0].Program != log.ARProgramRcService {
			t.Fatalf("got %v", events)
		}
	})
	t.Run("test_last_slices_after_filter", func(t *testing.T) {
		result := FilterLog(filterEntries(), FilterOptions{Predicate: filterNtp, Last: intPtr(1)})
		if len(eventList(t, result)) != 1 || result[log.ARLogFieldEntryCount] != 1 || result[log.ARLogFieldTotal] != 4 {
			t.Fatalf("got %v", result)
		}
	})
	t.Run("test_unique_before_filter", func(t *testing.T) {
		if got := eventList(t, FilterLog(filterEntries(), FilterOptions{Predicate: filterNtp, Unique: true})); len(got) != 1 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("test_last_entry_resolved", func(t *testing.T) {
		if lastEntry(t, FilterLog(filterEntries(), FilterOptions{Predicate: filterNtp})) == nil {
			t.Fatal("LAST_ENTRY is None")
		}
	})
	t.Run("test_predicate_without_prescreen", func(t *testing.T) {
		if got := eventList(t, FilterLog(filterEntries(), FilterOptions{Predicate: onlyMatches{}})); len(got) != 2 {
			t.Fatalf("got %v", got)
		}
	})
	// Python spies on `build_event` via monkeypatch; Go records every
	// translated event through the predicate's Matches instead
	t.Run("test_prescreen_skips_translation", func(t *testing.T) {
		spy := &counting{inner: filterNtp}
		FilterLog(filterEntries(), FilterOptions{Predicate: spy})
		if !sameSlice(spy.translated, []string{"start NTP update", "start NTP update"}) {
			t.Fatalf("translated %v", spy.translated)
		}
	})
}
