package firmware

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func ip(v int) *int { return &v }

func major(a, b, c, d int) *[4]int { return &[4]int{a, b, c, d} }

func eqIntP(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func eqMajorP(a, b *[4]int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestTranslateRevision(t *testing.T) {
	cases := []struct {
		raw      string
		expected any
	}{
		// Python also passes None; Go takes the regex group string
		{"", nil},
		{"2", 2},
		{"120", 120},
		{"g8178ee0", "g8178ee0"},
		{"g0d793c1_386-g45e48", "g0d793c1_386-g45e48"},
	}
	for _, c := range cases {
		if got := translateRevision(c.raw); got != c.expected {
			t.Errorf("%q: got %#v, want %#v", c.raw, got, c.expected)
		}
	}
}

func TestTranslateMajor(t *testing.T) {
	cases := []struct {
		raw      any
		expected *[4]int
	}{
		{nil, nil},
		{"", nil},
		{"   ", nil},
		{"invalid", nil},
		{"1.2.3", nil},
		{"3.0.0.4", major(3, 0, 0, 4)},
		{"3.0.0.6", major(3, 0, 0, 6)},
		{"9.0.0.4", major(9, 0, 0, 4)},
		{"9.0.0.6", major(9, 0, 0, 6)},
		{"3004", major(3, 0, 0, 4)},
		{"3006", major(3, 0, 0, 6)},
		{"9006", major(9, 0, 0, 6)},
		{"  3.0.0.4  ", major(3, 0, 0, 4)},
	}
	for _, c := range cases {
		if got := TranslateMajor(c.raw); !eqMajorP(got, c.expected) {
			t.Errorf("%#v: got %v, want %v", c.raw, got, c.expected)
		}
	}
}

func TestTranslateBuild(t *testing.T) {
	cases := []struct {
		raw      any
		build    *int
		revision any
		rog      bool
	}{
		{nil, nil, nil, false},
		{"", nil, nil, false},
		{"   ", nil, nil, false},
		{"not_a_build", nil, nil, false},
		// Bare build (old firmware extendno), no revision
		{"70", ip(70), nil, false},
		{"0", ip(0), nil, false},
		// Merlin: numeric revision
		{"8_2", ip(8), 2, false},
		{"7_120", ip(7), 120, false},
		// Stock: string revision
		{"40456_g8178ee0", ip(40456), "g8178ee0", false},
		{"34713_g0d793c1_386-g45e48", ip(34713), "g0d793c1_386-g45e48", false},
		// ROG
		{"8_2_rog", ip(8), 2, true},
	}
	for _, c := range cases {
		build, revision, rog := TranslateBuild(c.raw)
		if !eqIntP(build, c.build) || revision != c.revision || rog != c.rog {
			t.Errorf("%#v: got (%v, %#v, %v)", c.raw, build, revision, rog)
		}
	}
}

func TestTranslateMinor(t *testing.T) {
	cases := []struct {
		raw          any
		minor, build *int
	}{
		{nil, nil, nil},
		{"", nil, nil},
		{388, ip(388), nil},
		{"388", ip(388), nil},
		// Old firmware packs the build into a dotted buildno
		{"380.70", ip(380), ip(70)},
		{"384.20", ip(384), ip(20)},
	}
	for _, c := range cases {
		minor, build := TranslateMinor(c.raw)
		if !eqIntP(minor, c.minor) || !eqIntP(build, c.build) {
			t.Errorf("%#v: got (%v, %v)", c.raw, minor, build)
		}
	}
}

func TestTranslateType(t *testing.T) {
	cases := []struct {
		major    *[4]int
		minor    *int
		build    *int
		revision any
		rog      bool
		expected ARFirmwareType
	}{
		// Unknown: missing major, minor, or build
		{nil, ip(388), ip(8), nil, false, ARFirmwareTypeUnknown},
		{major(3, 0, 0, 6), nil, ip(8), nil, false, ARFirmwareTypeUnknown},
		{major(3, 0, 0, 6), ip(388), nil, nil, false, ARFirmwareTypeUnknown},
		// Beta major → STOCK regardless of other fields
		{major(9, 0, 0, 6), ip(102), ip(8), nil, false, ARFirmwareTypeStock},
		{major(9, 0, 0, 6), ip(102), ip(8), "gnuton1", false, ARFirmwareTypeStock},
		{major(9, 0, 0, 6), ip(102), ip(8), "alpha1", false, ARFirmwareTypeStock},
		{major(9, 0, 0, 6), ip(102), ip(8), nil, true, ARFirmwareTypeStock},
		// Gnuton (must be checked before Merlin)
		{major(3, 0, 0, 6), ip(388), ip(8), "gnuton1", false, ARFirmwareTypeGnuton},
		{major(3, 0, 0, 6), ip(388), ip(8), "1-gnuton0_beta2", false, ARFirmwareTypeGnuton},
		// Merlin: alpha or beta string in revision
		{major(3, 0, 0, 6), ip(388), ip(8), "alpha1", false, ARFirmwareTypeMerlin},
		{major(3, 0, 0, 6), ip(388), ip(8), "beta2", false, ARFirmwareTypeMerlin},
		{major(3, 0, 0, 6), ip(388), ip(8), "2beta1", false, ARFirmwareTypeMerlin},
		// Merlin: rog flag
		{major(3, 0, 0, 6), ip(388), ip(8), nil, true, ARFirmwareTypeMerlin},
		{major(3, 0, 0, 6), ip(388), ip(8), "g8178ee0", true, ARFirmwareTypeMerlin},
		// Stock: string revision with no known markers
		{major(3, 0, 0, 6), ip(388), ip(8), "g8178ee0", false, ARFirmwareTypeStock},
		// Stock: int revision (not a Merlin marker in this function)
		{major(3, 0, 0, 6), ip(388), ip(8), 2, false, ARFirmwareTypeStock},
	}
	for i, c := range cases {
		if got := TranslateType(c.major, c.minor, c.build, c.revision, c.rog); got != c.expected {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

// clearWarnedStrings mirrors the autouse fixture clearing
// `_WARNED_FW_STRINGS`.
func clearWarnedStrings() {
	warnedFwStringsMu.Lock()
	defer warnedFwStringsMu.Unlock()
	warnedFwStrings = map[string]struct{}{}
}

// captureLogs mirrors pytest's caplog at WARNING level.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestTranslateString(t *testing.T) {
	type out struct {
		major        *[4]int
		minor, build *int
		revision     any
		rog          bool
	}
	none := out{}
	cases := []struct {
		raw      any
		expected out
	}{
		// None / empty / sentinels
		{nil, none},
		{"", none},
		{"__", none},
		{"___", none},
		{"  __  ", none},
		// Standard Merlin
		{"3.0.0.4.388.8_2", out{major(3, 0, 0, 4), ip(388), ip(8), 2, false}},
		// ROG
		{"3.0.0.4.388.7_0_rog", out{major(3, 0, 0, 4), ip(388), ip(7), 0, true}},
		// Undotted major
		{"3004.388.7_120", out{major(3, 0, 0, 4), ip(388), ip(7), 120, false}},
		// Beta major
		{"9006_102_4856-g8178ee0", out{major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false}},
		// Long stock revision
		{"3.0.0.6.102.34713_g0d793c1_386-g45e48", out{major(3, 0, 0, 6), ip(102), ip(34713), "g0d793c1_386-g45e48", false}},
		// No major (partial)
		{".386.7_120", out{nil, ip(386), ip(7), 120, false}},
		// Whitespace trimmed
		{"  9006_102_4856-g8178ee0  ", out{major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false}},
	}
	for _, c := range cases {
		clearWarnedStrings()
		mj, mn, b, r, rog := TranslateString(c.raw)
		e := c.expected
		if !eqMajorP(mj, e.major) || !eqIntP(mn, e.minor) || !eqIntP(b, e.build) || !reflect.DeepEqual(r, e.revision) || rog != e.rog {
			t.Errorf("%#v: got (%v, %v, %v, %#v, %v)", c.raw, mj, mn, b, r, rog)
		}
	}
}

func TestTranslateStringWarnsOnce(t *testing.T) {
	clearWarnedStrings()
	buf := captureLogs(t)
	invalid := "this_is_not_a_firmware_string"
	TranslateString(invalid)
	TranslateString(invalid)
	if n := strings.Count(buf.String(), invalid); n != 1 {
		t.Fatalf("warned %d times: %s", n, buf.String())
	}
}

func TestTranslateStringWarnsPerUnique(t *testing.T) {
	clearWarnedStrings()
	buf := captureLogs(t)
	TranslateString("fw_unparseable_aaa")
	TranslateString("fw_unparseable_bbb")
	if !strings.Contains(buf.String(), "fw_unparseable_aaa") || !strings.Contains(buf.String(), "fw_unparseable_bbb") {
		t.Fatalf("missing warnings: %s", buf.String())
	}
}

func TestTranslateStringPartialWarnsOnce(t *testing.T) {
	clearWarnedStrings()
	buf := captureLogs(t)
	partial := ".388.8_2"
	TranslateString(partial)
	TranslateString(partial)
	if n := strings.Count(buf.String(), partial); n != 1 {
		t.Fatalf("warned %d times: %s", n, buf.String())
	}
}
