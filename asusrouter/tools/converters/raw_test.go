package converters

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestRawConvert(t *testing.T) {
	toInt := AsAny(RawToInt)
	// None and empty strings are treated as no value
	if RawConvert(nil, AsAny(RawToStr)) != nil || RawConvert("", AsAny(RawToStr)) != nil {
		t.Fatal("none/empty not treated as no value")
	}
	// A value is passed through the converter
	if got := RawConvert("5", toInt); got != 5 {
		t.Fatalf("value: %v", got)
	}
	// An empty-list result is treated as no value
	if got := RawConvert("x", func(any) any { return []any{} }); got != nil {
		t.Fatalf("empty list: %v", got)
	}
	// A zero result is kept (not treated as absent)
	if got := RawConvert("0", toInt); got != 0 {
		t.Fatalf("zero: %v", got)
	}
	// A typed nil pointer from the converter is no value
	if got := RawConvert("x", func(v any) any { return RawToInt(v) }); got != nil {
		t.Fatalf("nil pointer: %v", got)
	}
	// A nil pointer input is no value
	if got := RawConvert((*string)(nil), toInt); got != nil {
		t.Fatalf("nil input: %v", got)
	}
}

func TestStrToBoolType(t *testing.T) {
	if len(strToBool) != 14 {
		t.Fatalf("strToBool has %d entries", len(strToBool))
	}
}

func TestRawToBool(t *testing.T) {
	cases := []struct {
		value    any
		expected *bool
	}{
		{nil, nil}, {[]any{}, nil}, {map[string]any{}, nil},
		{"unknown_string", nil}, {"", nil},
		{true, ptr(true)}, {false, ptr(false)},
		{1, ptr(true)}, {-1, ptr(true)}, {0, ptr(false)}, {42, ptr(true)},
		{int64(0), ptr(false)},
		{1.0, ptr(true)}, {-0.5, ptr(true)}, {0.0, ptr(false)},
		{"true", ptr(true)}, {"allow", ptr(true)}, {"enable", ptr(true)},
		{"enabled", ptr(true)}, {"yes", ptr(true)}, {"1", ptr(true)}, {"on", ptr(true)},
		{"false", ptr(false)}, {"block", ptr(false)}, {"disable", ptr(false)},
		{"disabled", ptr(false)}, {"no", ptr(false)}, {"0", ptr(false)}, {"off", ptr(false)},
		{"  TRUE  ", ptr(true)}, {"  FALSE  ", ptr(false)}, {"On", ptr(true)}, {"OFF", ptr(false)},
		// Go-only inputs
		{ptr("on"), ptr(true)}, {(*string)(nil), nil}, {ptr(0), ptr(false)},
	}
	for _, c := range cases {
		if got := RawToBool(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("RawToBool(%#v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestRawToInt(t *testing.T) {
	cases := []struct {
		value    any
		expected *int
	}{
		{nil, nil}, {[]any{}, nil}, {map[string]any{}, nil},
		{"", nil}, {"  ", nil}, {"abc", nil}, {"inf", nil}, {"1e400", nil},
		{true, ptr(1)}, {false, ptr(0)}, {42, ptr(42)}, {-5, ptr(-5)},
		{3.7, ptr(3)}, {-2.9, ptr(-2)},
		{math.Inf(1), nil}, {math.Inf(-1), nil}, {math.NaN(), nil},
		{"42", ptr(42)}, {"-5", ptr(-5)},
		{"3.14", ptr(3)},
		{"  42  ", ptr(42)}, {bom + "42", ptr(42)},
		// Python int()/float() syntax details
		{"1_000", ptr(1000)}, {"1__0", nil}, {"0123", ptr(123)}, {"+7", ptr(7)},
		{"1e5", ptr(100000)}, {".5", ptr(0)}, {"0x1f", nil}, {"\x1c7\x1f", ptr(7)},
		// Go-only inputs
		{int64(7), ptr(7)}, {ptr("8"), ptr(8)},
	}
	for _, c := range cases {
		if got := RawToInt(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("RawToInt(%#v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestRawToIntBase(t *testing.T) {
	cases := []struct {
		base     int
		value    string
		expected int
	}{
		{16, "ff", 255}, {16, "FF", 255}, {16, "1f", 31}, {10, "42", 42},
		{16, "0x1f", 31}, {2, "0b11", 3}, {8, "0o17", 15},
	}
	for _, c := range cases {
		if got := RawToIntBase(c.value, c.base); got == nil || *got != c.expected {
			t.Errorf("RawToIntBase(%q, %d) = %v, want %d", c.value, c.base, got, c.expected)
		}
	}
}

func TestRawToFloat(t *testing.T) {
	cases := []struct {
		value    any
		expected *float64
	}{
		{nil, nil}, {[]any{}, nil}, {map[string]any{}, nil},
		{"", nil}, {"  ", nil}, {"abc", nil},
		{true, ptr(1.0)}, {false, ptr(0.0)}, {42, ptr(42.0)}, {3.14, ptr(3.14)}, {-5, ptr(-5.0)},
		{"3.14", ptr(3.14)}, {"-5.5", ptr(-5.5)}, {"42", ptr(42.0)},
		{"  3.14  ", ptr(3.14)}, {bom + "3.14", ptr(3.14)},
		{"inf", ptr(math.Inf(1))}, {"-Infinity", ptr(math.Inf(-1))}, {"1e400", ptr(math.Inf(1))},
		{"1_0.5", ptr(10.5)}, {"0x1p-2", nil},
	}
	for _, c := range cases {
		if got := RawToFloat(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("RawToFloat(%#v) = %v, want %v", c.value, got, c.expected)
		}
	}
	if got := RawToFloat("nan"); got == nil || !math.IsNaN(*got) {
		t.Errorf("RawToFloat(nan) = %v", got)
	}
}

func TestBOMConstant(t *testing.T) {
	if []rune(bom)[0] != 0xFEFF || len([]rune(bom)) != 1 {
		t.Fatal("bad BOM")
	}
}

func TestRawToStr(t *testing.T) {
	cases := []struct {
		value    any
		expected *string
	}{
		{nil, nil}, {0, nil}, {true, nil}, {[]any{}, nil},
		{"", nil}, {"   ", nil},
		{bom, nil}, {bom + "   ", nil},
		{"hello", ptr("hello")}, {"123", ptr("123")},
		{"  hello  ", ptr("hello")}, {"\t hello \n", ptr("hello")},
		{bom + "hello", ptr("hello")}, {bom + "  hello  ", ptr("hello")},
		// Python whitespace (str.isspace) includes \x1c-\x1f
		{"\x1chello\x1f\v", ptr("hello")}, {" x　", ptr("x")},
	}
	for _, c := range cases {
		if got := RawToStr(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("RawToStr(%#v) = %v, want %v", c.value, got, c.expected)
		}
	}
}

func TestRawToDatetime(t *testing.T) {
	naive := func(y, mo, d, h, mi, s, us int) *time.Time {
		v := time.Date(y, time.Month(mo), d, h, mi, s, us*1000, NaiveLocation)
		return &v
	}
	aware := func(y, mo, d, h, mi, s, us, off int) *time.Time {
		v := time.Date(y, time.Month(mo), d, h, mi, s, us*1000, time.FixedZone("", off))
		return &v
	}
	cases := []struct {
		value    any
		expected *time.Time
	}{
		{"2021-01-01   ", naive(2021, 1, 1, 0, 0, 0, 0)},
		{"2021-01-01 00:00:00", naive(2021, 1, 1, 0, 0, 0, 0)},
		{"Mon, 01 Jan 2021 00:00:00 +0000", aware(2021, 1, 1, 0, 0, 0, 0, 0)},
		{nil, nil}, {"", nil}, {"  ", nil},
		{"unknown", nil}, {"test", nil}, {"  test  ", nil},
		// fromisoformat details (checked against CPython 3.13)
		{"20210101", naive(2021, 1, 1, 0, 0, 0, 0)},
		{"2021-W01-1", naive(2021, 1, 4, 0, 0, 0, 0)},
		{"2021-01-01x10:30:15,123456789", naive(2021, 1, 1, 10, 30, 15, 123456)},
		{"2021-01-01T10:30Z", aware(2021, 1, 1, 10, 30, 0, 0, 0)},
		{"2021-01-01T10:30+0130", aware(2021, 1, 1, 10, 30, 0, 0, 5400)},
		{"2021-01-01T10:30 +01:00", aware(2021, 1, 1, 10, 30, 0, 0, 3600)},
		{"2021-01-01T10.5", naive(2021, 1, 1, 10, 0, 0, 500000)},
		{"2021-01-01T", nil}, {"2021-01-01T10:30x", nil}, {"2021-02-30", nil},
		{"2021-01-01T24:00", nil}, {"2021-01-01T10:30:15.+01", nil},
		// strptime details
		{"mon, 1 jan 2021 0:0:0 Z", aware(2021, 1, 1, 0, 0, 0, 0, 0)},
		{"Sun, 05 Dec 2021 23:59:59 -05:30", aware(2021, 12, 5, 23, 59, 59, 0, -19800)},
		{"Fri, 31 Feb 2021 10:00:00 +0100", nil},
		{"Sun, 05 Dec 2021 23:59:59 z", nil},
		{"Sun, 05 Dec 2021 23:59:59 +0130:15", nil},
	}
	for _, c := range cases {
		got := RawToDatetime(c.value)
		switch {
		case got == nil && c.expected == nil:
		case got == nil || c.expected == nil:
			t.Errorf("RawToDatetime(%#v) = %v, want %v", c.value, got, c.expected)
		case !got.Equal(*c.expected):
			t.Errorf("RawToDatetime(%#v) = %v, want %v", c.value, got, c.expected)
		case (got.Location() == NaiveLocation) != (c.expected.Location() == NaiveLocation):
			t.Errorf("RawToDatetime(%#v) naive mismatch: %v", c.value, got.Location())
		}
	}
}

func TestRawToStrList(t *testing.T) {
	cases := []struct {
		value     any
		delimiter string
		expected  []string
	}{
		{nil, " ", []string{}}, {1, " ", []string{}}, {map[int]int{1: 2}, " ", []string{}},
		{"", " ", []string{}}, {"   ", " ", []string{}},
		{"test", " ", []string{"test"}},
		{"test1 test2", " ", []string{"test1", "test2"}},
		{"a&#60b&#60c", "&#60", []string{"a", "b", "c"}},
		{"test1 test2", ";", []string{"test1 test2"}},
	}
	for _, c := range cases {
		if got := RawToStrList(c.value, c.delimiter); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("RawToStrList(%#v, %q) = %v, want %v", c.value, c.delimiter, got, c.expected)
		}
	}
}
