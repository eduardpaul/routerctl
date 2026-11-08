package log

import (
	"testing"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// Ported from tests/modules/log/test_clock.py.

// The device states the offset it wrote the stamps in
var (
	cest   = time.FixedZone("", 2*3600)
	anchor = time.Date(2026, 1, 1, 0, 0, 0, 0, cest)
)

func clockEntry(timestampStr string) *ARLogEntry {
	return &ARLogEntry{TimestampStr: timestampStr, Content: security.NewSensitiveText("x")}
}

func sameInstant(t *testing.T, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("got nil, want %v", want)
	}
	_, gotOffset := got.Zone()
	_, wantOffset := want.Zone()
	if !got.Equal(want) || gotOffset != wantOffset {
		t.Errorf("got %v, want %v", *got, want)
	}
}

func TestResolveDatetimes(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := ResolveDatetimes(nil, nil); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("mutates_in_place", func(t *testing.T) {
		entries := []*ARLogEntry{clockEntry("Jul 21 15:08:01")}
		result := ResolveDatetimes(entries, &anchor)
		if &result[0] != &entries[0] {
			t.Error("not the same slice")
		}
		sameInstant(t, entries[0].Timestamp, time.Date(2026, 7, 21, 15, 8, 1, 0, cest))
	})

	t.Run("anchor_year_applied", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01")}, &anchor)
		sameInstant(t, result[0].Timestamp, time.Date(2026, 7, 21, 15, 8, 1, 0, cest))
	})

	t.Run("default_anchor_is_current_year", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01")}, nil)
		if result[0].Timestamp == nil || result[0].Timestamp.Year() != time.Now().Year() {
			t.Errorf("got %v", result[0].Timestamp)
		}
	})

	t.Run("year_wrap_backwards", func(t *testing.T) {
		entries := []*ARLogEntry{clockEntry("Dec 31 23:59:59"), clockEntry("Jan 01 00:00:05")}
		result := ResolveDatetimes(entries, &anchor)
		sameInstant(t, result[1].Timestamp, time.Date(2026, 1, 1, 0, 0, 5, 0, cest))
		sameInstant(t, result[0].Timestamp, time.Date(2025, 12, 31, 23, 59, 59, 0, cest))
	})

	t.Run("month_jitter_no_wrap", func(t *testing.T) {
		entries := []*ARLogEntry{clockEntry("Jul 31 23:59:59"), clockEntry("Aug 01 00:00:05")}
		result := ResolveDatetimes(entries, &anchor)
		sameInstant(t, result[0].Timestamp, time.Date(2026, 7, 31, 23, 59, 59, 0, cest))
		sameInstant(t, result[1].Timestamp, time.Date(2026, 8, 1, 0, 0, 5, 0, cest))
	})

	stays := []struct {
		name   string
		stamp  string
		anchor *time.Time
	}{
		{"unparseable_stamp_stays_none", "not a stamp", nil},
		{"invalid_date_stays_none", "Feb 30 00:00:00", &anchor},
		{"short_stamp_stays_none", "Jul 21", nil},
		{"partial_clock_stays_none", "Jul 21 15:08", nil},
		{"non_numeric_stamp_stays_none", "Jul xx 15:08:01", nil},
		{"unknown_month_stays_none", "Zzz 01 00:00:00", nil},
	}
	for _, tc := range stays {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveDatetimes([]*ARLogEntry{clockEntry(tc.stamp)}, tc.anchor)
			if result[0].Timestamp != nil {
				t.Errorf("got %v", *result[0].Timestamp)
			}
		})
	}

	// Go-only: Python int() takes a sign, underscores and Unicode digits
	t.Run("python_int_forms", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul +2_1 15:08:٠١")}, &anchor)
		sameInstant(t, result[0].Timestamp, time.Date(2026, 7, 21, 15, 8, 1, 0, cest))
	})
}

func hostOffset() int {
	_, offset := time.Now().Zone()
	return offset
}

func TestHostFallback(t *testing.T) {
	t.Run("host_offset_is_used", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01")}, nil)
		ts := result[0].Timestamp
		if ts == nil {
			t.Fatal("nil timestamp")
		}
		if _, offset := ts.Zone(); offset != hostOffset() || ts.Location() == consts.NaiveLocation {
			t.Errorf("offset %d, want %d", offset, hostOffset())
		}
	})

	t.Run("future_stamp_takes_the_year_before", func(t *testing.T) {
		current := time.Now()
		ahead := current.Add(60 * 24 * time.Hour)
		result := ResolveDatetimes([]*ARLogEntry{clockEntry(ahead.Format("Jan 02 15:04:05"))}, nil)
		ts := result[0].Timestamp
		if ts == nil || ts.After(current.Add(24*time.Hour)) {
			t.Errorf("got %v", ts)
		}
	})

	t.Run("unparseable_newest_falls_back_to_host_year", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01"), clockEntry("not a stamp")}, nil)
		if result[0].Timestamp == nil || result[0].Timestamp.Year() != time.Now().Year() {
			t.Errorf("got %v", result[0].Timestamp)
		}
	})

	t.Run("invalid_newest_stamp_is_skipped", func(t *testing.T) {
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01"), clockEntry("Feb 30 00:00:00")}, nil)
		if result[1].Timestamp != nil {
			t.Errorf("newest: got %v", *result[1].Timestamp)
		}
		if result[0].Timestamp == nil || result[0].Timestamp.Location() == consts.NaiveLocation {
			t.Errorf("oldest: got %v", result[0].Timestamp)
		}
	})

	t.Run("naive_anchor_falls_back_to_the_host", func(t *testing.T) {
		naive := time.Date(2019, 1, 1, 0, 0, 0, 0, consts.NaiveLocation)
		result := ResolveDatetimes([]*ARLogEntry{clockEntry("Jul 21 15:08:01")}, &naive)
		ts := result[0].Timestamp
		if ts == nil || ts.Location() == consts.NaiveLocation || ts.Year() != time.Now().Year() {
			t.Errorf("got %v", ts)
		}
	})
}
