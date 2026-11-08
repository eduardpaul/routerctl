package schedule

import (
	"testing"

	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
)

func TestDayGroups(t *testing.T) {
	if len(Daily) != 7 {
		t.Error("daily")
	}
	if !Weekdays.Equal(NewWeekdaySet(pc.ARWeekdayMonday, pc.ARWeekdayTuesday, pc.ARWeekdayWednesday, pc.ARWeekdayThursday, pc.ARWeekdayFriday)) {
		t.Error("weekdays")
	}
	if !Weekend.Equal(NewWeekdaySet(pc.ARWeekdaySunday, pc.ARWeekdaySaturday)) {
		t.Error("weekend")
	}
}

func TestParse(t *testing.T) {
	if len(ParseTimemap("")) != 0 || len(ParseTimemap(nil)) != 0 {
		t.Error("empty")
	}

	entries := ParseTimemap("W03E21000700")
	if len(entries) != 1 {
		t.Fatalf("single_entry: %v", entries)
	}
	e := entries[0]
	if !e.Days.Equal(Weekdays) || e.StartHour != 21 || e.StartMinute != 0 || e.EndHour != 7 || e.EndMinute != 0 || e.Enable {
		t.Errorf("single_entry: %+v", e)
	}

	entries = ParseTimemap("W13E17002100&#60M14116002200")
	if len(entries) != 2 || !entries[0].Days.Equal(Weekdays) || !entries[0].Enable || !entries[1].Days.Equal(Weekend) {
		t.Errorf("multiple_entries_decoded: %+v", entries)
	}

	entries = ParseTimemap("W04122000600")
	if len(entries) != 1 || !entries[0].Days.Equal(Weekend) || entries[0].StartHour != 22 || entries[0].EndHour != 6 {
		t.Errorf("cross_midnight_single_entry: %+v", entries)
	}

	entries = ParseTimemap("W17F09002400")
	if len(entries) != 1 || !entries[0].Days.Equal(Daily) || entries[0].EndHour != 24 {
		t.Errorf("end_hour_24: %+v", entries)
	}

	if len(ParseTimemap("W03E210007")) != 0 {
		t.Error("malformed_length_skipped")
	}
	if len(ParseTimemap("W03EXX000700")) != 0 {
		t.Error("malformed_value_skipped")
	}
}

func TestSerialize(t *testing.T) {
	if SerializeTimemap(nil, pc.ARParentalControlScheduleModeOffline) != "" {
		t.Error("empty")
	}
	offline := ARScheduleEntry{Days: Weekdays, StartHour: 21, EndHour: 7, Enable: false}
	if got := SerializeTimemap([]ARScheduleEntry{offline}, pc.ARParentalControlScheduleModeOffline); got != "W03E21000700" {
		t.Errorf("offline_prefix: %s", got)
	}
	online := ARScheduleEntry{Days: Weekdays, StartHour: 17, EndHour: 21, Enable: true}
	if got := SerializeTimemap([]ARScheduleEntry{online}, pc.ARParentalControlScheduleModeOnline); got != "M13E17002100" {
		t.Errorf("online_prefix: %s", got)
	}
	daily := NewARScheduleEntry()
	daily.Days = Daily
	if got := SerializeTimemap([]ARScheduleEntry{daily}, pc.ARParentalControlScheduleModeUnknown); got[:1] != "W" {
		t.Errorf("unknown_mode_defaults_offline: %s", got)
	}
	weekend := NewARScheduleEntry()
	weekend.Days = Weekend
	if got := SerializeTimemap([]ARScheduleEntry{weekend}, ""); got[:1] != "W" {
		t.Errorf("default_mode_offline: %s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	for raw, mode := range map[string]pc.ARParentalControlScheduleMode{
		"W03E21000700<W04122000800": pc.ARParentalControlScheduleModeOffline,
		"M13E17002100<M14116002200": pc.ARParentalControlScheduleModeOnline,
		"W17F00002400":              pc.ARParentalControlScheduleModeOffline,
	} {
		if got := SerializeTimemap(ParseTimemap(raw), mode); got != raw {
			t.Errorf("%s -> %s", raw, got)
		}
	}
}

func TestApplyMode(t *testing.T) {
	if got := ApplyMode("W03E21000700<W04122000800", pc.ARParentalControlScheduleModeOnline); got != "M03E21000700<M04122000800" {
		t.Errorf("rewrites_prefixes: %s", got)
	}
	if got := ApplyMode("W03E21000700", pc.ARParentalControlScheduleModeUnknown); got != "W03E21000700" {
		t.Errorf("unknown_mode_noop: %s", got)
	}
	if got := ApplyMode("", pc.ARParentalControlScheduleModeOnline); got != "" {
		t.Errorf("empty_noop: %s", got)
	}
}

func TestTimemapMode(t *testing.T) {
	for raw, want := range map[string]pc.ARParentalControlScheduleMode{
		"W03E21000700":     pc.ARParentalControlScheduleModeOffline,
		"M13E17002100":     pc.ARParentalControlScheduleModeOnline,
		"":                 pc.ARParentalControlScheduleModeUnknown,
		"&#60W03E21000700": pc.ARParentalControlScheduleModeUnknown,
	} {
		if got := TimemapMode(raw); got != want {
			t.Errorf("%q: %q", raw, got)
		}
	}
}

func TestWeekdaySetCanonical(t *testing.T) {
	got := NewWeekdaySet(pc.ARWeekdaySaturday, pc.ARWeekdaySunday).CanonicalValue().([]any)
	if len(got) != 2 || got[0] != 0 || got[1] != 6 {
		t.Errorf("got %v", got)
	}
}
