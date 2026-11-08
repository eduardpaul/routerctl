package firmware

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eduardpaul/routerctl/asusrouter/tools/converters"
)

// compareRevision returns true if revision a is less than b.
func compareRevision(a, b any) bool {
	if a == nil {
		return true
	}
	if b == nil {
		return false
	}
	ai, aok := a.(int)
	bi, bok := b.(int)
	if aok && bok {
		return ai < bi
	}
	return converters.PyStr(a) < converters.PyStr(b)
}

// ARFirmware is the firmware information of the device.
//
// The revision is `int | str | None` in Python: nil, an int or a string
// here. ARFirmware is immutable once built.
type ARFirmware struct {
	major        *[4]int
	minor        *int
	build        *int
	revision     any
	rog          bool
	firmwareType ARFirmwareType
}

// NewARFirmware mirrors `ARFirmware(major, minor, build, revision, rog)`.
func NewARFirmware(major *[4]int, minor *int, build *int, revision any, rog bool) *ARFirmware {
	return &ARFirmware{
		major:        major,
		minor:        minor,
		build:        build,
		revision:     revision,
		rog:          rog,
		firmwareType: TranslateType(major, minor, build, revision, rog),
	}
}

// Build gets the build number.
func (f *ARFirmware) Build() *int { return f.build }

// FirmwareType gets the firmware type.
func (f *ARFirmware) FirmwareType() ARFirmwareType {
	// The zero value is the Python default `ARFirmware()`
	if f.firmwareType == "" {
		return ARFirmwareTypeUnknown
	}
	return f.firmwareType
}

// Major gets the major version as an int tuple.
func (f *ARFirmware) Major() *[4]int { return f.major }

// Minor gets the minor version.
func (f *ARFirmware) Minor() *int { return f.minor }

// Revision gets the revision (nil, int or string).
func (f *ARFirmware) Revision() any { return f.revision }

// Rog gets the ROG flag.
func (f *ARFirmware) Rog() bool { return f.rog }

// ARFirmwareFromNvram mirrors `ARFirmware.from_nvram`: build from nvram
// values (FW_MAJOR, FW_MINOR, FW_BUILD).
func ARFirmwareFromNvram(fwMajor, fwMinor, fwBuild any) *ARFirmware {
	major := TranslateMajor(fwMajor)
	minor, minorBuild := TranslateMinor(fwMinor)
	var buildRaw any
	if fwBuild != nil {
		buildRaw = converters.PyStr(fwBuild)
	}
	build, revision, rog := TranslateBuild(buildRaw)
	// Older firmware packs the build into a dotted `buildno` (380.70),
	// leaving `extendno` as the revision (e.g. `0`) - keep it
	if minorBuild != nil {
		if revision == nil && build != nil {
			revision = *build
		}
		build = minorBuild
	}
	return NewARFirmware(major, minor, build, revision, rog)
}

// ARFirmwareFromString mirrors `ARFirmware.from_string`: build from a full
// firmware version string.
func ARFirmwareFromString(fwString any) *ARFirmware {
	major, minor, build, revision, rog := TranslateString(fwString)
	return NewARFirmware(major, minor, build, revision, rog)
}

// String returns the firmware as a version string.
func (f *ARFirmware) String() string {
	majorStr := "{}"
	if f.major != nil {
		parts := make([]string, 4)
		for i, x := range f.major {
			parts[i] = strconv.Itoa(x)
		}
		majorStr = strings.Join(parts, ".")
	}
	minorStr := "{}"
	if f.minor != nil {
		minorStr = strconv.Itoa(*f.minor)
	}
	buildStr := "{}"
	if f.build != nil {
		buildStr = strconv.Itoa(*f.build)
	}
	revisionStr := "{}"
	if f.revision != nil {
		revisionStr = converters.PyStr(f.revision)
	}
	rogStr := ""
	if f.rog {
		rogStr = "_rog"
	}
	return fmt.Sprintf("%s.%s.%s_%s%s", majorStr, minorStr, buildStr, revisionStr, rogStr)
}

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func eqMajor(a, b *[4]int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Equal mirrors `__eq__`.
func (f *ARFirmware) Equal(other *ARFirmware) bool {
	if f == nil || other == nil {
		return f == other
	}
	return eqMajor(f.major, other.major) &&
		eqIntPtr(f.minor, other.minor) &&
		eqIntPtr(f.build, other.build) &&
		f.revision == other.revision &&
		f.rog == other.rog
}

// majorTail returns `(major or ())[1:]`.
func majorTail(m *[4]int) []int {
	if m == nil {
		return nil
	}
	return m[1:]
}

// lessInts compares int tuples lexicographically, like Python.
func lessInts(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func orZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// Less mirrors `__lt__`: compare firmware versions.
//
// Comparison is only valid within the same firmware type. Major is
// compared by platform identity (skip first segment - the beta/normal
// prefix '3' or '9').
func (f *ARFirmware) Less(other *ARFirmware) bool {
	if f == nil || other == nil {
		return false
	}
	if f.FirmwareType() != other.FirmwareType() {
		return false
	}
	if !eqMajor(f.major, other.major) {
		return lessInts(majorTail(f.major), majorTail(other.major))
	}
	if !eqIntPtr(f.minor, other.minor) || !eqIntPtr(f.build, other.build) {
		return lessInts(
			[]int{orZero(f.minor), orZero(f.build)},
			[]int{orZero(other.minor), orZero(other.build)},
		)
	}
	if f.revision != other.revision {
		return compareRevision(f.revision, other.revision)
	}
	return false
}

// Greater mirrors `__gt__`.
func (f *ARFirmware) Greater(other *ARFirmware) bool {
	if f == nil || other == nil {
		return false
	}
	return other.Less(f)
}

// CanonicalValue renders the public properties, like the Python
// canonical form of a plain object.
func (f *ARFirmware) CanonicalValue() any {
	var major any
	if f.major != nil {
		major = []int{f.major[0], f.major[1], f.major[2], f.major[3]}
	}
	return map[string]any{
		"build":         f.build,
		"firmware_type": f.FirmwareType(),
		"major":         major,
		"minor":         f.minor,
		"revision":      f.revision,
		"rog":           f.rog,
	}
}

func intPtr(v int) *int { return &v }

// ARFw388 mirrors `AR_FW_388`.
var ARFw388 = NewARFirmware(&[4]int{3, 0, 0, 4}, intPtr(388), intPtr(0), nil, false)
