package log

// Log timestamp resolution for AsusRouter.

import (
	"strings"
	"time"

	"github.com/eduardpaul/routerctl/asusrouter/consts"
	"github.com/eduardpaul/routerctl/asusrouter/modules/log/internal/pyre"
	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// `<hh>:<mm>:<ss>`
const clockParts = 3

var months = map[string]int{
	"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
	"Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
}

// `<Mon> <day> <hh>:<mm>:<ss>`
const stampParts = 3

const yearWrapMonths = 2

const futureGrace = 24 * time.Hour

// now is the host clock (a seam for tests)
var now = time.Now

type stamp struct {
	month, day, hour, minute, second int
}

// parseStamp parses a stamp into (month, day, hour, minute, second).
func parseStamp(timestampStr string) (stamp, bool) {
	// Splitting beats a regex on a stamp per entry
	parts := converters.PySplit(timestampStr)
	if len(parts) != stampParts {
		return stamp{}, false
	}

	month, ok := months[parts[0]]
	if !ok {
		return stamp{}, false
	}

	clock := strings.Split(parts[2], ":")
	if len(clock) != clockParts {
		return stamp{}, false
	}

	var values [4]int
	for i, text := range []string{parts[1], clock[0], clock[1], clock[2]} {
		value, ok := pyre.Int(text, 10)
		if !ok {
			return stamp{}, false
		}
		values[i] = value
	}
	return stamp{month, values[0], values[1], values[2], values[3]}, true
}

// pyDatetime mirrors `datetime(year, month, day, hour, minute, second,
// tzinfo=zone)`, which raises ValueError on an out-of-range field.
func pyDatetime(year int, s stamp, zone *time.Location) (time.Time, bool) {
	if year < 1 || year > 9999 || s.month < 1 || s.month > 12 ||
		s.hour < 0 || s.hour > 23 || s.minute < 0 || s.minute > 59 ||
		s.second < 0 || s.second > 59 || s.day < 1 {
		return time.Time{}, false
	}
	// The day must exist in that month
	if last := time.Date(year, time.Month(s.month)+1, 0, 0, 0, 0, 0, time.UTC).Day(); s.day > last {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(s.month), s.day, s.hour, s.minute, s.second, 0, zone), true
}

// hostFallback uses host clock data if no device clock is available.
func hostFallback(entries []*ARLogEntry) (int, *time.Location) {
	// `datetime.now().astimezone()`: the host time in its current offset
	current := now()
	_, offset := current.Zone()
	zone := time.FixedZone("", offset)
	current = current.In(zone)
	year := current.Year()

	for i := len(entries) - 1; i >= 0; i-- {
		parts, ok := parseStamp(entries[i].TimestampStr)
		if !ok {
			continue
		}
		newest, ok := pyDatetime(year, parts, zone)
		if !ok {
			continue
		}
		if newest.After(current.Add(futureGrace)) {
			year--
		}
		break
	}

	return year, zone
}

// isAware reports whether a time carries an offset (Python `tzinfo`).
func isAware(t time.Time) bool {
	return t.Location() != consts.NaiveLocation
}

// ResolveDatetimes mirrors `resolve_datetimes`: fill each entry's
// `timestamp` against the device's own clock. anchor may be nil.
func ResolveDatetimes(entries []*ARLogEntry, anchor *time.Time) []*ARLogEntry {
	if len(entries) == 0 {
		return entries
	}

	var year int
	var zone *time.Location
	// Use device anchor, or fall back to the host
	if anchor != nil && isAware(*anchor) {
		year, zone = anchor.Year(), anchor.Location()
	} else {
		year, zone = hostFallback(entries)
	}
	newerMonth := 0

	// Newest entry is last; walk back to oldest
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		parts, ok := parseStamp(entry.TimestampStr)
		if !ok {
			continue
		}

		if newerMonth != 0 && parts.month-newerMonth >= yearWrapMonths {
			year--
		}
		newerMonth = parts.month

		stamped, ok := pyDatetime(year, parts, zone)
		if !ok {
			continue
		}

		entry.Timestamp = &stamped
	}

	return entries
}
