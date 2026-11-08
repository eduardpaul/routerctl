// Timemap model and codec for parental control schedules.
//
// A timemap is a `<`-joined string of 12-char entries, each encoding one
// weekly window as `[mode][enable][weekday_hex][sH][sM][eH][eM]`:
//
//   - `mode`  - `W` offline / `M` online, per-rule (see the schedule mode enum)
//   - `enable`- per-entry `0`/`1`
//   - `weekday_hex` - 2 hex digits, one bit per day (bit 0 Sun .. bit 6 Sat)
//   - `sH sM eH eM` - zero-padded start/end hour and minute; a window that
//     spans midnight is stored as a single entry with the end earlier than
//     the start, and `eH` may be `24`

package schedule

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	pc "github.com/eduardpaul/routerctl/asusrouter/modules/parental_control"
	"github.com/eduardpaul/routerctl/asusrouter/tools/readers"
)

// DefaultTimemap is the default weekly schedule the web UI seeds a new
// rule with.
const DefaultTimemap = "W03E21000700<W04122000800"

const tokenLen = 12

// Every real weekday, in bit order (UNKNOWN is not a bitmap position)
var weekdays = func() []pc.ARWeekday {
	var out []pc.ARWeekday
	for _, day := range pc.ARWeekdayMembers {
		if day != pc.ARWeekdayUnknown {
			out = append(out, day)
		}
	}
	return out
}()

// WeekdaySet is a set of weekdays (Python `frozenset[ARWeekday]`).
type WeekdaySet map[pc.ARWeekday]struct{}

// NewWeekdaySet builds a set from days.
func NewWeekdaySet(days ...pc.ARWeekday) WeekdaySet {
	out := WeekdaySet{}
	for _, day := range days {
		out[day] = struct{}{}
	}
	return out
}

// Has reports whether the set holds day.
func (s WeekdaySet) Has(day pc.ARWeekday) bool {
	_, ok := s[day]
	return ok
}

// Sorted returns the days in ascending order.
func (s WeekdaySet) Sorted() []pc.ARWeekday {
	out := make([]pc.ARWeekday, 0, len(s))
	for day := range s {
		out = append(out, day)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Equal reports whether both sets hold the same days.
func (s WeekdaySet) Equal(other WeekdaySet) bool {
	if len(s) != len(other) {
		return false
	}
	for day := range s {
		if !other.Has(day) {
			return false
		}
	}
	return true
}

// CanonicalValue renders the set as a sorted list (canonical form of a
// Python set).
func (s WeekdaySet) CanonicalValue() any {
	out := make([]any, 0, len(s))
	for _, day := range s.Sorted() {
		out = append(out, int(day))
	}
	return out
}

// Common day groups, matching the web UI's Daily/Weekdays/Weekend presets
var (
	Daily    = NewWeekdaySet(weekdays...)
	Weekdays = NewWeekdaySet(
		pc.ARWeekdayMonday,
		pc.ARWeekdayTuesday,
		pc.ARWeekdayWednesday,
		pc.ARWeekdayThursday,
		pc.ARWeekdayFriday,
	)
	Weekend = NewWeekdaySet(pc.ARWeekdaySunday, pc.ARWeekdaySaturday)
)

// ARScheduleEntry mirrors `ARScheduleEntry`: a single weekly schedule
// window.
type ARScheduleEntry struct {
	Days        WeekdaySet `json:"days"`
	StartHour   int        `json:"start_hour"`
	StartMinute int        `json:"start_minute"`
	EndHour     int        `json:"end_hour"`
	EndMinute   int        `json:"end_minute"`
	Enable      bool       `json:"enable"`
}

// NewARScheduleEntry returns an entry with the Python defaults (no days,
// 00:00-24:00, enabled).
func NewARScheduleEntry() ARScheduleEntry {
	return ARScheduleEntry{Days: WeekdaySet{}, EndHour: 24, Enable: true}
}

// Equal mirrors the dataclass `__eq__`.
func (e ARScheduleEntry) Equal(other ARScheduleEntry) bool {
	return e.Days.Equal(other.Days) &&
		e.StartHour == other.StartHour &&
		e.StartMinute == other.StartMinute &&
		e.EndHour == other.EndHour &&
		e.EndMinute == other.EndMinute &&
		e.Enable == other.Enable
}

// TimemapMode mirrors `timemap_mode`: the schedule mode from a timemap's
// first entry prefix char.
func TimemapMode(raw any) pc.ARParentalControlScheduleMode {
	text := readers.Decode(raw)
	first := ""
	if text != "" {
		first = strings.ToUpper(string([]rune(text)[:1]))
	}
	return pc.ARParentalControlScheduleModeFromValue(first)
}

func daysFromBits(bits int) WeekdaySet {
	// Expand a weekday bitmap into its set of days
	out := WeekdaySet{}
	for _, day := range weekdays {
		if bits>>int(day)&1 == 1 {
			out[day] = struct{}{}
		}
	}
	return out
}

func bitsFromDays(days WeekdaySet) int {
	// Collapse a set of days into a weekday bitmap
	bits := 0
	for day := range days {
		bits += 1 << int(day)
	}
	return bits
}

// pyInt mirrors Python `int(text, base)` for the timemap fields (surrounding
// whitespace, a sign and `_` separators are accepted like Python).
func pyInt(text string, base int) (int, bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return 0, false
	}
	sign := ""
	if t[0] == '+' || t[0] == '-' {
		sign, t = t[:1], t[1:]
	}
	if t == "" || strings.HasPrefix(t, "_") || strings.HasSuffix(t, "_") || strings.Contains(t, "__") {
		return 0, false
	}
	v, err := strconv.ParseInt(sign+strings.ReplaceAll(t, "_", ""), base, 64)
	if err != nil {
		return 0, false
	}
	return int(v), true
}

func parseEntry(token string) *ARScheduleEntry {
	// Parse one 12-char timemap entry, or nil when it is malformed
	runes := []rune(token)
	if len(runes) != tokenLen {
		return nil
	}
	field := func(from, to int) string { return string(runes[from:to]) }

	bits, ok1 := pyInt(field(2, 4), 16)
	startHour, ok2 := pyInt(field(4, 6), 10)
	startMinute, ok3 := pyInt(field(6, 8), 10)
	endHour, ok4 := pyInt(field(8, 10), 10)
	endMinute, ok5 := pyInt(field(10, 12), 10)
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		slog.Debug("Skipping malformed timemap entry", "token", token)
		return nil
	}
	return &ARScheduleEntry{
		Enable:      runes[1] != '0',
		Days:        daysFromBits(bits),
		StartHour:   startHour,
		StartMinute: startMinute,
		EndHour:     endHour,
		EndMinute:   endMinute,
	}
}

// ParseTimemap mirrors `parse_timemap`: decode a raw timemap string into
// its schedule entries.
func ParseTimemap(raw any) []ARScheduleEntry {
	text := readers.Decode(raw)
	if text == "" {
		return []ARScheduleEntry{}
	}
	out := []ARScheduleEntry{}
	for _, token := range strings.Split(text, "<") {
		if entry := parseEntry(token); entry != nil {
			out = append(out, *entry)
		}
	}
	return out
}

func serializeEntry(entry ARScheduleEntry, prefix string) string {
	// Serialize one schedule entry into its 12-char token
	enable := "0"
	if entry.Enable {
		enable = "1"
	}
	return prefix +
		enable +
		fmt.Sprintf("%02X", bitsFromDays(entry.Days)) +
		fmt.Sprintf("%02d%02d", entry.StartHour, entry.StartMinute) +
		fmt.Sprintf("%02d%02d", entry.EndHour, entry.EndMinute)
}

// SerializeTimemap mirrors `serialize_timemap`: serialize schedule
// entries into a raw timemap string for the mode (an empty mode is
// OFFLINE, the Python default).
func SerializeTimemap(entries []ARScheduleEntry, mode pc.ARParentalControlScheduleMode) string {
	if mode == "" || mode == pc.ARParentalControlScheduleModeUnknown {
		mode = pc.ARParentalControlScheduleModeOffline
	}
	parts := make([]string, len(entries))
	for i, entry := range entries {
		parts[i] = serializeEntry(entry, string(mode))
	}
	return strings.Join(parts, "<")
}

// ApplyMode mirrors `apply_mode`: rewrite each raw timemap entry's prefix
// char to the given mode.
func ApplyMode(timemap string, mode pc.ARParentalControlScheduleMode) string {
	if mode == pc.ARParentalControlScheduleModeUnknown || timemap == "" {
		return timemap
	}
	entries := strings.Split(timemap, "<")
	for i, entry := range entries {
		if entry != "" {
			entries[i] = string(mode) + string([]rune(entry)[1:])
		}
	}
	return strings.Join(entries, "<")
}
