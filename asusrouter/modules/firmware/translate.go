package firmware

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

const majorBetaPrefix = 9

var (
	warnedFwStrings   = map[string]struct{}{}
	warnedFwStringsMu sync.Mutex
)

// The Python patterns carry an optional lookahead `(?=_rog|$)?`, which is
// a no-op (it may match zero times); RE2 has no lookaheads, so it is
// dropped here.
var (
	FwMajorPattern = regexp.MustCompile(`^([39]).?([0-9]).?([0-9]).?([0-9])$`)
	FwBuildPattern = regexp.MustCompile(
		`^(?P<build>[0-9]+)[_.-]?` +
			`(?P<revision>[a-zA-Z0-9_-]*?)` +
			`(?P<rog>_rog)?$`,
	)
	FwPattern = regexp.MustCompile(
		`^(?P<major>[39].?0.?0.?[46])?[_.]?` +
			`(?P<minor>[0-9]{3})[_.]?` +
			`(?P<build>[0-9]+)[_.-]?` +
			`(?P<revision>[a-zA-Z0-9_-]+?)` +
			`(?P<rog>_rog)?$`,
	)
)

// warnOnce reports whether raw was not warned about yet (and marks it).
func warnOnce(raw string) bool {
	warnedFwStringsMu.Lock()
	defer warnedFwStringsMu.Unlock()
	if _, ok := warnedFwStrings[raw]; ok {
		return false
	}
	warnedFwStrings[raw] = struct{}{}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// translateRevision translates a revision string to int, string, or nil.
func translateRevision(raw string) any {
	if raw == "" {
		return nil
	}
	if isDigits(raw) {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return raw
}

// TranslateMajor translates a major version string to an int tuple if
// possible.
func TranslateMajor(raw any) *[4]int {
	s := converters.RawToStr(raw)
	if s == nil || *s == "" {
		return nil
	}
	m := FwMajorPattern.FindStringSubmatch(*s)
	if m == nil {
		return nil
	}
	var out [4]int
	for i := 0; i < 4; i++ {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return &out
}

// TranslateMinor translates the minor version into `(minor, build)`.
//
// New firmware reports just the branch (`388`); older firmware packs the
// build into a dotted `buildno` (`380.70` -> minor 380, build 70).
func TranslateMinor(raw any) (*int, *int) {
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok && s == "" {
		return nil, nil
	}
	text := converters.PyStr(raw)
	branch, build, _ := strings.Cut(text, ".")
	var buildInt *int
	if build != "" {
		buildInt = converters.RawToInt(build)
	}
	return converters.RawToInt(branch), buildInt
}

// TranslateBuild translates a build string to (build, revision, rog).
func TranslateBuild(raw any) (*int, any, bool) {
	s := converters.RawToStr(raw)
	if s == nil || *s == "" {
		return nil, nil, false
	}

	m := FwBuildPattern.FindStringSubmatch(*s)
	if m == nil {
		return nil, nil, false
	}

	build, err := strconv.Atoi(m[FwBuildPattern.SubexpIndex("build")])
	if err != nil {
		return nil, nil, false
	}
	revision := translateRevision(m[FwBuildPattern.SubexpIndex("revision")])
	rog := m[FwBuildPattern.SubexpIndex("rog")] != ""

	return &build, revision, rog
}

// TranslateType determines the firmware type from firmware components.
func TranslateType(major *[4]int, minor *int, build *int, revision any, rog bool) ARFirmwareType {
	if major == nil || minor == nil || build == nil {
		return ARFirmwareTypeUnknown
	}
	if major[0] == majorBetaPrefix {
		return ARFirmwareTypeStock
	}
	if rev, ok := revision.(string); ok {
		if strings.Contains(rev, "gnuton") {
			return ARFirmwareTypeGnuton
		}
		if strings.Contains(rev, "alpha") || strings.Contains(rev, "beta") {
			return ARFirmwareTypeMerlin
		}
	}
	if rog {
		return ARFirmwareTypeMerlin
	}
	return ARFirmwareTypeStock
}

// TranslateString translates a firmware version string to components:
// (major, minor, build, revision, rog).
func TranslateString(raw any) (*[4]int, *int, *int, any, bool) {
	s := converters.RawToStr(raw)
	if s == nil || *s == "" || *s == "__" || *s == "___" {
		return nil, nil, nil, nil, false
	}
	text := *s

	m := FwPattern.FindStringSubmatch(text)
	if m == nil {
		if warnOnce(text) {
			slog.Warn("Firmware string cannot be parsed. " +
				"Please report this. Original string: `" + text + "`")
		}
		return nil, nil, nil, nil, false
	}

	majorRaw := m[FwPattern.SubexpIndex("major")]
	var major *[4]int
	if majorRaw != "" {
		var digits []int
		for _, ch := range majorRaw {
			if ch >= '0' && ch <= '9' {
				digits = append(digits, int(ch-'0'))
			}
		}
		// Python unpacks exactly four digits (and raises otherwise)
		if len(digits) == 4 {
			major = &[4]int{digits[0], digits[1], digits[2], digits[3]}
		}
	} else if warnOnce(text) {
		slog.Warn("Partial firmware string, major version missing. " +
			"Please report this. Original string: `" + text + "`")
	}

	minor, _ := strconv.Atoi(m[FwPattern.SubexpIndex("minor")])
	build, err := strconv.Atoi(m[FwPattern.SubexpIndex("build")])
	if err != nil {
		return nil, nil, nil, nil, false
	}
	revision := translateRevision(m[FwPattern.SubexpIndex("revision")])
	rog := m[FwPattern.SubexpIndex("rog")] != ""

	return major, &minor, &build, revision, rog
}
