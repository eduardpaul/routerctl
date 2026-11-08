// Go-only datetime parsing that reproduces the two Python calls used by
// `raw_to_datetime`: `datetime.fromisoformat` (CPython 3.13 rules) and
// `datetime.strptime(value, "%a, %d %b %Y %H:%M:%S %z")`.

package converters

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NaiveLocation marks a `time.Time` that stands for a Python naive
// datetime (no tzinfo). `consts.NaiveLocation` is the same value.
var NaiveLocation = time.FixedZone("naive", 0)

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// atoiRunes parses a run of ASCII digits.
func atoiRunes(r []rune) (int, bool) {
	if len(r) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range r {
		if !isASCIIDigit(c) {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// isoSeparator mirrors `_find_isoformat_datetime_separator`.
func isoSeparator(s []rune) (int, bool) {
	n := len(s)
	if n == 7 {
		return 7, true
	}
	if s[4] == '-' {
		if s[5] == 'W' {
			if n > 8 && s[8] == '-' {
				if n == 9 {
					return 0, false
				}
				if n > 10 && isASCIIDigit(s[10]) {
					return 8, true
				}
				return 10, true
			}
			return 8, true
		}
		return 10, true
	}
	if s[4] == 'W' {
		idx := 7
		for idx < n && isASCIIDigit(s[idx]) {
			idx++
		}
		if idx < 9 {
			return idx, true
		}
		if idx%2 == 0 {
			return 7, true
		}
		return 8, true
	}
	return 8, true
}

// isoDate mirrors `_parse_isoformat_date`.
func isoDate(s []rune) (year, month, day int, ok bool) {
	if len(s) != 7 && len(s) != 8 && len(s) != 10 {
		return 0, 0, 0, false
	}
	if year, ok = atoiRunes(s[0:4]); !ok {
		return 0, 0, 0, false
	}
	hasSep := s[4] == '-'
	pos := 4
	if hasSep {
		pos++
	}
	sub := func(from, to int) []rune {
		if from > len(s) {
			from = len(s)
		}
		if to > len(s) {
			to = len(s)
		}
		return s[from:to]
	}
	if string(sub(pos, pos+1)) == "W" {
		pos++
		week, ok := atoiRunes(sub(pos, pos+2))
		if !ok {
			return 0, 0, 0, false
		}
		pos += 2
		weekday := 1
		if len(s) > pos {
			if (string(sub(pos, pos+1)) == "-") != hasSep {
				return 0, 0, 0, false
			}
			if hasSep {
				pos++
			}
			if weekday, ok = atoiRunes(sub(pos, pos+1)); !ok {
				return 0, 0, 0, false
			}
		}
		return isoWeekToGregorian(year, week, weekday)
	}
	if month, ok = atoiRunes(sub(pos, pos+2)); !ok {
		return 0, 0, 0, false
	}
	pos += 2
	if (string(sub(pos, pos+1)) == "-") != hasSep {
		return 0, 0, 0, false
	}
	if hasSep {
		pos++
	}
	if day, ok = atoiRunes(sub(pos, pos+2)); !ok || pos+2 != len(s) {
		return 0, 0, 0, false
	}
	return year, month, day, true
}

// isoWeekToGregorian mirrors `_isoweek_to_gregorian`.
func isoWeekToGregorian(year, week, day int) (int, int, int, bool) {
	if year < 1 || year > 9999 {
		return 0, 0, 0, false
	}
	if week < 1 || week > 52 {
		outOfRange := true
		if week == 53 {
			first := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Weekday()
			leap := year%4 == 0 && (year%100 != 0 || year%400 == 0)
			if first == time.Thursday || (first == time.Wednesday && leap) {
				outOfRange = false
			}
		}
		if outOfRange {
			return 0, 0, 0, false
		}
	}
	if day < 1 || day > 7 {
		return 0, 0, 0, false
	}
	// Monday of ISO week 1: the week holding January 4th
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	offset := (int(jan4.Weekday()) + 6) % 7
	monday := jan4.AddDate(0, 0, -offset)
	t := monday.AddDate(0, 0, (week-1)*7+(day-1))
	return t.Year(), int(t.Month()), t.Day(), true
}

var fractionCorrection = []int{100000, 10000, 1000, 100, 10}

// isoHHMMSSFF mirrors CPython's C `parse_hh_mm_ss_ff` on s[p:end]
// (HH[:?MM[:?SS[{.,}fff[fff]]]]). It returns rv < 0 on error, 1 when
// characters remain and 0 otherwise. Reads past `end` see the rest of s
// and then a NUL, as the C code does.
func isoHHMMSSFF(s []rune, p, end int) ([4]int, int) {
	var comps [4]int
	at := func(i int) rune {
		if i < len(s) {
			return s[i]
		}
		return 0
	}
	digits := func(p, n int) (int, bool) {
		v := 0
		for k := 0; k < n; k++ {
			c := at(p + k)
			if !isASCIIDigit(c) {
				return 0, false
			}
			v = v*10 + int(c-'0')
		}
		return v, true
	}

	hasSep := true
	for i := 0; i < 3; i++ {
		v, ok := digits(p, 2)
		if !ok {
			return comps, -3
		}
		comps[i] = v
		p += 2
		c := at(p)
		p++
		if i == 0 {
			hasSep = c == ':'
		}
		if c == '.' || c == ',' {
			break
		}
		if p >= end {
			if c != 0 {
				return comps, 1
			}
			return comps, 0
		}
		if hasSep && c == ':' {
			continue
		}
		if !hasSep {
			p--
			continue
		}
		// Malformed time separator
		return comps, -4
	}

	// Parse fractional components
	toParse := min(end-p, 6)
	v, ok := digits(p, toParse)
	if !ok || toParse <= 0 {
		return comps, -3
	}
	comps[3] = v
	if toParse < 6 {
		comps[3] *= fractionCorrection[toParse-1]
	}
	p += toParse
	for isASCIIDigit(at(p)) {
		p++
	}
	if at(p) != 0 {
		return comps, 1
	}
	return comps, 0
}

// isoTime mirrors CPython's C `parse_isoformat_time`.
func isoTime(s []rune) (comps [4]int, loc *time.Location, ok bool) {
	tzPos := 0
	for tzPos < len(s) && s[tzPos] != 'Z' && s[tzPos] != '+' && s[tzPos] != '-' {
		tzPos++
	}
	comps, rv := isoHHMMSSFF(s, 0, tzPos)
	if rv < 0 {
		return comps, nil, false
	}
	if tzPos == len(s) {
		// No time zone, so trailing characters are an error
		return comps, NaiveLocation, rv == 0
	}
	// Special case UTC / Zulu time
	if s[tzPos] == 'Z' {
		return comps, time.UTC, tzPos+1 == len(s)
	}

	tz, rv := isoHHMMSSFF(s, tzPos+1, len(s))
	if rv != 0 {
		return comps, nil, false
	}
	offset := tz[0]*3600 + tz[1]*60 + tz[2]
	if offset >= 86400 {
		return comps, nil, false
	}
	if s[tzPos] == '-' {
		offset = -offset
	}
	// Sub-second offsets cannot be represented by a Go zone: truncated
	return comps, fixedZone(offset), true
}

// fixedZone mirrors Python `timezone(timedelta(seconds=offset))`.
func fixedZone(offset int) *time.Location {
	if offset == 0 {
		return time.UTC
	}
	sign := '+'
	abs := offset
	if offset < 0 {
		sign, abs = '-', -offset
	}
	name := fmt.Sprintf("UTC%c%02d:%02d", sign, abs/3600, abs%3600/60)
	if abs%60 != 0 {
		name += fmt.Sprintf(":%02d", abs%60)
	}
	return time.FixedZone(name, offset)
}

// newDatetime mirrors the Python `datetime(...)` constructor checks.
func newDatetime(year, month, day, hour, minute, second, micro int, loc *time.Location) (time.Time, bool) {
	if year < 1 || year > 9999 || month < 1 || month > 12 || day < 1 {
		return time.Time{}, false
	}
	if day > time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return time.Time{}, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, micro*1000, loc), true
}

// fromISOFormat mirrors `datetime.fromisoformat`.
func fromISOFormat(text string) (time.Time, bool) {
	s := []rune(text)
	if len(s) < 7 {
		return time.Time{}, false
	}
	sep, ok := isoSeparator(s)
	if !ok {
		return time.Time{}, false
	}
	dstr := s[:min(sep, len(s))]
	hasTime := sep < len(s)
	var tstr []rune
	if hasTime {
		tstr = s[sep+1:]
	}
	for _, c := range dstr {
		if c > 0x7f {
			return time.Time{}, false
		}
	}
	year, month, day, ok := isoDate(dstr)
	if !ok {
		return time.Time{}, false
	}
	comps, loc := [4]int{}, NaiveLocation
	if hasTime {
		if comps, loc, ok = isoTime(tstr); !ok {
			return time.Time{}, false
		}
	}
	return newDatetime(year, month, day, comps[0], comps[1], comps[2], comps[3], loc)
}

var (
	pyMonths = map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}
	// `_strptime` regex for "%a, %d %b %Y %H:%M:%S %z" (C locale)
	rfc1123Re = regexp.MustCompile(`^(?i)` +
		`(?P<a>mon|tue|wed|thu|fri|sat|sun),` + PyWhitespaceClass + `+` +
		`(?P<d>3[0-1]|[1-2]\d|0[1-9]|[1-9]| [1-9])` + PyWhitespaceClass + `+` +
		`(?P<b>jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)` + PyWhitespaceClass + `+` +
		`(?P<Y>\d\d\d\d)` + PyWhitespaceClass + `+` +
		`(?P<H>2[0-3]|[0-1]\d|\d| \d):(?P<M>[0-5]\d|\d):(?P<S>6[0-1]|[0-5]\d|\d)` + PyWhitespaceClass + `+` +
		`(?P<z>[+-]\d\d:?[0-5]\d(:?[0-5]\d(\.\d{1,6})?)?|(?-i:Z))`)
)

// strptimeRFC1123 mirrors
// `datetime.strptime(value, "%a, %d %b %Y %H:%M:%S %z")`.
func strptimeRFC1123(text string) (time.Time, bool) {
	m := rfc1123Re.FindStringSubmatchIndex(text)
	if m == nil || m[1] != len(text) {
		return time.Time{}, false
	}
	group := func(name string) string {
		i := rfc1123Re.SubexpIndex(name)
		return text[m[2*i]:m[2*i+1]]
	}
	atoi := func(name string) int {
		v, _ := strconv.Atoi(strings.TrimSpace(group(name)))
		return v
	}

	offset := 0
	z := group("z")
	if z != "Z" {
		if z[3] == ':' {
			z = z[:3] + z[4:]
			if len(z) > 5 {
				if z[5] != ':' {
					return time.Time{}, false
				}
				z = z[:5] + z[6:]
			}
		}
		hours, _ := strconv.Atoi(z[1:3])
		minutes, _ := strconv.Atoi(z[3:5])
		seconds := 0
		if len(z) > 5 {
			var err error
			if seconds, err = strconv.Atoi(z[5:min(len(z), 7)]); err != nil {
				return time.Time{}, false
			}
		}
		offset = hours*3600 + minutes*60 + seconds
		if strings.HasPrefix(z, "-") {
			offset = -offset
		}
	}

	return newDatetime(
		atoi("Y"), pyMonths[strings.ToLower(group("b"))], atoi("d"),
		atoi("H"), atoi("M"), atoi("S"), 0, fixedZone(offset),
	)
}
