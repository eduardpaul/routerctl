package query

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/translate"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

func entriesFixture() []*log.ARLogEntry {
	out := make([]*log.ARLogEntry, 0, 5)
	for index := 0; index < 5; index++ {
		out = append(out, &log.ARLogEntry{
			TimestampStr: fmt.Sprintf("Jul 21 15:08:0%d", index),
			Content:      security.NewSensitiveText(fmt.Sprintf("line %d", index)),
		})
	}
	return out
}

func mixedFixture() []*log.ARLogEntry {
	return []*log.ARLogEntry{
		{
			TimestampStr: "Jul 21 15:08:01",
			Program:      &log.ARLogProgram{Name: "vpnclient5", Pid: intPtr(10)},
			Content:      security.NewSensitiveText("up"),
			Timestamp:    timePtr(naive(2026, 7, 21, 15, 8, 1)),
		},
		{
			TimestampStr: "Jul 21 15:08:05",
			Program:      &log.ARLogProgram{Name: "wlceventd"},
			Content:      security.NewSensitiveText("Deauth WL0"),
			Timestamp:    timePtr(naive(2026, 7, 21, 15, 8, 5)),
		},
		{
			TimestampStr: "Jul 21 15:08:09",
			Program:      &log.ARLogProgram{Name: "vpnclient6", Pid: intPtr(22)},
			Content:      security.NewSensitiveText("up again"),
			Timestamp:    timePtr(naive(2026, 7, 21, 15, 8, 9)),
		},
		{TimestampStr: "Jul 21 15:08:11", Content: security.NewSensitiveText("no program here")},
	}
}

func eventsFixture(mixed []*log.ARLogEntry) []*ARLogEvent {
	out := []*ARLogEvent{}
	for _, entry := range mixed[:2] {
		out = append(out, newEvent(eventOpts{
			timestamp:   entry.Timestamp,
			program:     log.ARProgramNtp,
			programName: strPtr("ntp"),
			eventType:   translate.AREventNtpStartUpdate,
			raw:         entry.Content.Value,
		}))
	}
	return out
}

func sameSlice[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLastEntries(t *testing.T) {
	entries := entriesFixture()
	t.Run("test_all_when_none", func(t *testing.T) {
		// Python checks identity (`is`): the very same backing array
		got := LastEntries(entries, nil)
		if len(got) != len(entries) || &got[0] != &entries[0] {
			t.Fatal("not the same list")
		}
	})
	t.Run("test_last_n", func(t *testing.T) {
		if !sameSlice(LastEntries(entries, intPtr(2)), entries[3:]) {
			t.Fatal("wrong slice")
		}
	})
	t.Run("test_count_exceeds_length", func(t *testing.T) {
		if !sameSlice(LastEntries(entries, intPtr(99)), entries) {
			t.Fatal("wrong slice")
		}
	})
	for _, count := range []int{0, -3} {
		t.Run(fmt.Sprintf("test_non_positive/%d", count), func(t *testing.T) {
			if got := LastEntries(entries, intPtr(count)); len(got) != 0 {
				t.Fatalf("got %v", got)
			}
		})
	}
	t.Run("test_generic_over_tuples", func(t *testing.T) {
		type pair struct {
			n int
			s string
		}
		got := LastEntries([]pair{{1, "a"}, {2, "b"}, {3, "c"}}, intPtr(1))
		if !sameSlice(got, []pair{{3, "c"}}) {
			t.Fatalf("got %v", got)
		}
	})
}

func lastEntry(t *testing.T, result map[log.ARLogField]any) *time.Time {
	t.Helper()
	v, ok := result[log.ARLogFieldLastEntry].(*time.Time)
	if !ok {
		t.Fatalf("LAST_ENTRY %#v is not a *time.Time", result[log.ARLogFieldLastEntry])
	}
	return v
}

func TestSummarize(t *testing.T) {
	mixed := mixedFixture()
	events := eventsFixture(mixed)
	t.Run("test_entry_list_default", func(t *testing.T) {
		result := Summarize(mixed, 4, nil, false)
		list := result[log.ARLogFieldEntryList].([]*log.ARLogEntry)
		if &list[0] != &mixed[0] {
			t.Error("ENTRY_LIST is not the same list")
		}
		if _, ok := result[log.ARLogFieldEventList]; ok {
			t.Error("EVENT_LIST present")
		}
		if result[log.ARLogFieldEntryCount] != len(mixed) {
			t.Error("count")
		}
		if lastEntry(t, result) != nil { // last entry has no time
			t.Error("LAST_ENTRY not None")
		}
	})
	t.Run("test_programs_opt_in", func(t *testing.T) {
		result := Summarize(mixed, 4, nil, true)
		if !reflect.DeepEqual(result[log.ARLogFieldProgramList], GroupPrograms(mixed)) {
			t.Fatal("PROGRAM_LIST differs")
		}
	})
	t.Run("test_programs_default_skips_grouping", func(t *testing.T) {
		if _, ok := Summarize(mixed, 4, nil, false)[log.ARLogFieldProgramList]; ok {
			t.Fatal("PROGRAM_LIST present")
		}
	})
	t.Run("test_events_swap_list_key", func(t *testing.T) {
		result := Summarize(mixed[:2], 4, events, false)
		list := result[log.ARLogFieldEventList].([]*ARLogEvent)
		if &list[0] != &events[0] {
			t.Error("EVENT_LIST is not the same list")
		}
		if _, ok := result[log.ARLogFieldEntryList]; ok {
			t.Error("ENTRY_LIST present")
		}
	})
	t.Run("test_events_drive_count_and_last", func(t *testing.T) {
		result := Summarize([]*log.ARLogEntry{}, 9, events, false)
		if result[log.ARLogFieldEntryCount] != len(events) {
			t.Error("count")
		}
		if got := lastEntry(t, result); got == nil || !got.Equal(*events[len(events)-1].Timestamp) {
			t.Errorf("LAST_ENTRY %v", got)
		}
	})
	t.Run("test_total_when_given", func(t *testing.T) {
		if got := Summarize(mixed[:1], 42, nil, false)[log.ARLogFieldTotal]; got != 42 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("test_total_always_reported", func(t *testing.T) {
		if got := Summarize(mixed, len(mixed), nil, false)[log.ARLogFieldTotal]; got != len(mixed) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("test_last_entry_timestamp", func(t *testing.T) {
		if got := lastEntry(t, Summarize(mixed[:2], 4, nil, false)); got == nil || !got.Equal(*mixed[1].Timestamp) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("test_empty", func(t *testing.T) {
		result := Summarize([]*log.ARLogEntry{}, 0, nil, true)
		if result[log.ARLogFieldEntryCount] != 0 || lastEntry(t, result) != nil {
			t.Error("not zeroed")
		}
		if got := result[log.ARLogFieldProgramList].([]log.ARLogProgramGroup); len(got) != 0 {
			t.Errorf("PROGRAM_LIST %v", got)
		}
	})
}

func TestGroupPrograms(t *testing.T) {
	t.Run("test_grouped_with_kind_and_index", func(t *testing.T) {
		want := []log.ARLogProgramGroup{
			{Name: "vpnclient5", Program: log.ARProgramVpnClient, Index: intPtr(5), Pids: []int{10}, Pidless: false, Count: 1},
			{Name: "wlceventd", Program: log.ARProgramWirelessClientDaemon, Index: nil, Pids: []int{}, Pidless: true, Count: 1},
			{Name: "vpnclient6", Program: log.ARProgramVpnClient, Index: intPtr(6), Pids: []int{22}, Pidless: false, Count: 1},
		}
		if got := GroupPrograms(mixedFixture()); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("test_mixed_pid_and_pidless", func(t *testing.T) {
		entries := []*log.ARLogEntry{
			{TimestampStr: "Jul 21 15:08:01", Program: &log.ARLogProgram{Name: "ntp", Pid: intPtr(7)}},
			{TimestampStr: "Jul 21 15:08:02", Program: &log.ARLogProgram{Name: "ntp"}},
		}
		want := []log.ARLogProgramGroup{
			{Name: "ntp", Program: log.ARProgramNtp, Pids: []int{7}, Pidless: true, Count: 2},
		}
		if got := GroupPrograms(entries); !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("test_parsed_unknown_for_unrecognized", func(t *testing.T) {
		group := GroupPrograms([]*log.ARLogEntry{
			{TimestampStr: "Jul 21 15:08:01", Program: &log.ARLogProgram{Name: "ARK"}},
		})[0]
		if group.Program != log.ARProgramUnknown || group.Index != nil {
			t.Fatalf("got %+v", group)
		}
	})
	t.Run("test_skips_programless", func(t *testing.T) {
		if got := GroupPrograms([]*log.ARLogEntry{{TimestampStr: "Jul 21 15:08:01"}}); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestUniqueEntries(t *testing.T) {
	t.Run("test_dedup_keeps_first", func(t *testing.T) {
		entries := []*log.ARLogEntry{
			{TimestampStr: "Jul 21 15:08:01", Content: security.NewSensitiveText("a")},
			{TimestampStr: "Jul 21 15:09:02", Content: security.NewSensitiveText("a")},
			{TimestampStr: "Jul 21 15:10:03", Content: security.NewSensitiveText("b")},
		}
		result := UniqueEntries(entries)
		var contents []string
		for _, e := range result {
			contents = append(contents, e.ContentValue())
		}
		if !sameSlice(contents, []string{"a", "b"}) || result[0].TimestampStr != "Jul 21 15:08:01" {
			t.Fatalf("got %v", contents)
		}
	})
	t.Run("test_empty", func(t *testing.T) {
		if got := UniqueEntries(nil); len(got) != 0 {
			t.Fatalf("got %v", got)
		}
	})
}
