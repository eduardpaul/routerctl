package firmware

import (
	"fmt"
	"testing"
)

// Canonical instances reused across comparison / equality test cases
var (
	unknownFw     = NewARFirmware(nil, nil, nil, nil, false)
	stockBase     = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), "gA", false)
	stockCopy     = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), "gA", false)
	stockBigMajor = NewARFirmware(major(3, 0, 0, 6), ip(388), ip(8), "gA", false)
	stockBigMinor = NewARFirmware(major(3, 0, 0, 4), ip(390), ip(8), "gA", false)
	stockBigBuild = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(10), "gA", false)
	stockAltRev   = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), "gB", false)
	stockNoRev    = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), nil, false)
	merlinBase    = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 2, true)
	merlinBigRev  = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 7, true)
	merlinNoRog   = NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 2, false)
)

func TestCompareRevision(t *testing.T) {
	cases := []struct {
		a, b     any
		expected bool
	}{
		{nil, "abc", true},
		{nil, 5, true},
		{"abc", nil, false},
		{5, nil, false},
		{2, 5, true},
		{5, 2, false},
		{2, 2, false},
		{"abc", "abd", true},
		{"abd", "abc", false},
		{"abc", "abc", false},
		{2, "abc", true},
		{"abc", 2, false},
	}
	for _, c := range cases {
		if got := compareRevision(c.a, c.b); got != c.expected {
			t.Errorf("(%#v, %#v): got %v", c.a, c.b, got)
		}
	}
}

type fwAttrs struct {
	major        *[4]int
	minor, build *int
	revision     any
	rog          bool
	firmwareType *ARFirmwareType
}

func ft(v ARFirmwareType) *ARFirmwareType { return &v }

func checkAttrs(t *testing.T, name string, fw *ARFirmware, e fwAttrs) {
	t.Helper()
	if !eqMajorP(fw.Major(), e.major) || !eqIntP(fw.Minor(), e.minor) || !eqIntP(fw.Build(), e.build) ||
		fw.Revision() != e.revision || fw.Rog() != e.rog {
		t.Errorf("%s: got %v %v %v %#v %v", name, fw.Major(), fw.Minor(), fw.Build(), fw.Revision(), fw.Rog())
	}
	if e.firmwareType != nil && fw.FirmwareType() != *e.firmwareType {
		t.Errorf("%s: type %v, want %v", name, fw.FirmwareType(), *e.firmwareType)
	}
}

func TestARFirmwareInit(t *testing.T) {
	cases := []struct {
		fw       *ARFirmware
		expected fwAttrs
	}{
		{NewARFirmware(nil, nil, nil, nil, false), fwAttrs{firmwareType: ft(ARFirmwareTypeUnknown)}},
		{
			NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), "g8178ee0", false),
			fwAttrs{major(3, 0, 0, 4), ip(388), ip(8), "g8178ee0", false, ft(ARFirmwareTypeStock)},
		},
		{
			NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 2, true),
			fwAttrs{major(3, 0, 0, 4), ip(388), ip(8), 2, true, ft(ARFirmwareTypeMerlin)},
		},
		{
			NewARFirmware(major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false),
			fwAttrs{major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false, ft(ARFirmwareTypeStock)},
		},
		{
			NewARFirmware(major(3, 0, 0, 6), ip(388), ip(8), "gnuton1", false),
			fwAttrs{major(3, 0, 0, 6), ip(388), ip(8), "gnuton1", false, ft(ARFirmwareTypeGnuton)},
		},
	}
	for i, c := range cases {
		checkAttrs(t, fmt.Sprint(i), c.fw, c.expected)
	}

	// The zero value is the Python default `ARFirmware()`
	var zero ARFirmware
	checkAttrs(t, "zero", &zero, fwAttrs{firmwareType: ft(ARFirmwareTypeUnknown)})
}

func TestARFirmwareFromNvram(t *testing.T) {
	cases := []struct {
		fwMajor, fwMinor, fwBuild any
		expected                  fwAttrs
	}{
		{"3.0.0.4", 388, "40456_g8178ee0", fwAttrs{major(3, 0, 0, 4), ip(388), ip(40456), "g8178ee0", false, ft(ARFirmwareTypeStock)}},
		{"3.0.0.4", 388, "8_2_rog", fwAttrs{major(3, 0, 0, 4), ip(388), ip(8), 2, true, ft(ARFirmwareTypeMerlin)}},
		{"9.0.0.6", "102", "4856_g8178ee0", fwAttrs{major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false, ft(ARFirmwareTypeStock)}},
		// Old firmware: build in dotted buildno, extendno is the revision
		{"3.0.0.4", "380.70", "0", fwAttrs{major(3, 0, 0, 4), ip(380), ip(70), 0, false, nil}},
		// Old firmware with an empty extendno keeps no revision
		{"3.0.0.4", "380.70", "", fwAttrs{major(3, 0, 0, 4), ip(380), ip(70), nil, false, nil}},
		{nil, nil, nil, fwAttrs{firmwareType: ft(ARFirmwareTypeUnknown)}},
	}
	for i, c := range cases {
		checkAttrs(t, fmt.Sprint(i), ARFirmwareFromNvram(c.fwMajor, c.fwMinor, c.fwBuild), c.expected)
	}
}

func TestARFirmwareFromString(t *testing.T) {
	cases := []struct {
		fwString any
		expected fwAttrs
	}{
		{"3.0.0.4.388.40456_g8178ee0", fwAttrs{major(3, 0, 0, 4), ip(388), ip(40456), "g8178ee0", false, ft(ARFirmwareTypeStock)}},
		{"3.0.0.4.388.8_2_rog", fwAttrs{major(3, 0, 0, 4), ip(388), ip(8), 2, true, ft(ARFirmwareTypeMerlin)}},
		{"9006_102_4856-g8178ee0", fwAttrs{major(9, 0, 0, 6), ip(102), ip(4856), "g8178ee0", false, ft(ARFirmwareTypeStock)}},
		{nil, fwAttrs{firmwareType: ft(ARFirmwareTypeUnknown)}},
		{"", fwAttrs{firmwareType: ft(ARFirmwareTypeUnknown)}},
	}
	for _, c := range cases {
		checkAttrs(t, fmt.Sprint(c.fwString), ARFirmwareFromString(c.fwString), c.expected)
	}
}

func TestARFirmwareStr(t *testing.T) {
	cases := []struct {
		fw       *ARFirmware
		expected string
	}{
		{NewARFirmware(major(3, 0, 0, 4), ip(388), ip(40456), "g8178ee0", false), "3.0.0.4.388.40456_g8178ee0"},
		{NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 2, true), "3.0.0.4.388.8_2_rog"},
		{NewARFirmware(nil, ip(388), ip(8), "g8178ee0", false), "{}.388.8_g8178ee0"},
		{NewARFirmware(major(3, 0, 0, 4), nil, ip(8), "g8178ee0", false), "3.0.0.4.{}.8_g8178ee0"},
		{NewARFirmware(major(3, 0, 0, 4), ip(388), nil, "g8178ee0", false), "3.0.0.4.388.{}_g8178ee0"},
		{NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), nil, false), "3.0.0.4.388.8_{}"},
		{NewARFirmware(nil, nil, nil, nil, false), "{}.{}.{}_{}"},
		{NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), 2, false), "3.0.0.4.388.8_2"},
	}
	for _, c := range cases {
		if got := c.fw.String(); got != c.expected {
			t.Errorf("got %q, want %q", got, c.expected)
		}
	}
}

func TestARFirmwareReprEqualsStr(t *testing.T) {
	// Python `__repr__` delegates to `__str__`; Go `%v` uses String()
	fw := NewARFirmware(major(3, 0, 0, 4), ip(388), ip(8), "g8178ee0", false)
	if fmt.Sprintf("%v", fw) != fw.String() {
		t.Fatal("repr differs from str")
	}
}

// Go has no `__hash__`; ARFirmware is used by pointer, so the hash tests
// map onto Equal (equal hashes for equal values, distinct otherwise).
func TestARFirmwareHash(t *testing.T) {
	if !stockBase.Equal(stockCopy) {
		t.Error("equal objects must compare equal")
	}
	if merlinBase.Equal(merlinNoRog) {
		t.Error("rog flag must differ")
	}
	if stockBase.Equal(stockAltRev) {
		t.Error("revision must differ")
	}
	if !stockBase.Equal(stockBase) {
		t.Error("stability")
	}
}

func TestARFirmwareEq(t *testing.T) {
	cases := []struct {
		a, b     *ARFirmware
		expected bool
	}{
		{stockBase, stockCopy, true},
		{stockBase, stockBigMajor, false},
		{stockBase, stockBigMinor, false},
		{stockBase, stockBigBuild, false},
		{stockBase, stockAltRev, false},
		{merlinBase, merlinNoRog, false},
		{unknownFw, unknownFw, true},
	}
	for i, c := range cases {
		if got := c.a.Equal(c.b); got != c.expected {
			t.Errorf("case %d: got %v", i, got)
		}
	}
	// Python returns NotImplemented for non-firmware; Go is typed, nil is
	// the only non-firmware value
	if stockBase.Equal(nil) {
		t.Error("equal to nil")
	}
}

func TestARFirmwareLt(t *testing.T) {
	cases := []struct {
		a, b     *ARFirmware
		expected bool
	}{
		// Different firmware types → False both ways
		{stockBase, merlinBase, false},
		{merlinBase, stockBase, false},
		// Same UNKNOWN type, all equal → False
		{unknownFw, unknownFw, false},
		// Different major (compared by [1:])
		{stockBase, stockBigMajor, true},
		{stockBigMajor, stockBase, false},
		// Different minor
		{stockBigMinor, stockBase, false},
		{stockBase, stockBigMinor, true},
		// Different build
		{stockBase, stockBigBuild, true},
		{stockBigBuild, stockBase, false},
		// Different revision (int, Merlin)
		{merlinBase, merlinBigRev, true},
		{merlinBigRev, merlinBase, false},
		// Different revision (str, Stock)
		{stockBase, stockAltRev, true},
		// Revision None vs non-None (None is less)
		{stockNoRev, stockBase, true},
		{stockBase, stockNoRev, false},
		// Equal → False
		{stockBase, stockCopy, false},
	}
	for i, c := range cases {
		if got := c.a.Less(c.b); got != c.expected {
			t.Errorf("case %d: got %v", i, got)
		}
	}
	if stockBase.Less(nil) {
		t.Error("less than nil")
	}
}

func TestARFirmwareGt(t *testing.T) {
	cases := []struct {
		a, b     *ARFirmware
		expected bool
	}{
		{stockBigMajor, stockBase, true},
		{stockBase, stockBigMajor, false},
		{stockBase, stockCopy, false},
		{stockBase, merlinBase, false},
	}
	for i, c := range cases {
		if got := c.a.Greater(c.b); got != c.expected {
			t.Errorf("case %d: got %v", i, got)
		}
	}
	if stockBase.Greater(nil) {
		t.Error("greater than nil")
	}
}

func TestARFirmwareCanonicalValue(t *testing.T) {
	got := ARFw388.CanonicalValue().(map[string]any)
	if got["firmware_type"] != ARFirmwareTypeStock || got["rog"] != false || got["revision"] != nil {
		t.Fatalf("got %v", got)
	}
	m, ok := got["major"].([]int)
	if !ok || len(m) != 4 || m[3] != 4 {
		t.Fatalf("major %v", got["major"])
	}
}
