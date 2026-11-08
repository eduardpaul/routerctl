package enum

import (
	"testing"
)

type fakeStrEnum string

const (
	fakeStrEnumUnknown fakeStrEnum = "unknown_value"
	fakeStrEnumFoo     fakeStrEnum = "foo_value"
	fakeStrEnumBar     fakeStrEnum = "bar_value"
)

var (
	fakeStrMembers = []fakeStrEnum{fakeStrEnumUnknown, fakeStrEnumFoo, fakeStrEnumBar}
	fakeStrNames   = map[string]fakeStrEnum{"UNKNOWN": fakeStrEnumUnknown, "FOO": fakeStrEnumFoo, "BAR": fakeStrEnumBar}
)

type fakeIntEnum int

const (
	fakeIntEnumUnknown fakeIntEnum = -999
	fakeIntEnumOne     fakeIntEnum = 1
	fakeIntEnumTwo     fakeIntEnum = 2
)

var (
	fakeIntMembers = []fakeIntEnum{fakeIntEnumUnknown, fakeIntEnumOne, fakeIntEnumTwo}
	fakeIntNames   = map[string]fakeIntEnum{"UNKNOWN": fakeIntEnumUnknown, "ONE": fakeIntEnumOne, "TWO": fakeIntEnumTwo}
)

func TestFromStr(t *testing.T) {
	cases := []struct {
		id     string
		value  any
		member fakeStrEnum
	}{
		{"actual_enum", fakeStrEnumFoo, fakeStrEnumFoo},
		{"from_correct_value", "unknown_value", fakeStrEnumUnknown},
		{"from_parsed_value", "  foo_value  ", fakeStrEnumFoo},
		{"from_correct_key", "foo", fakeStrEnumFoo},
		{"from_parsed_key", "  bar  ", fakeStrEnumBar},
		{"not_value_not_key", "none", fakeStrEnumUnknown},
		{"not_a_string", nil, fakeStrEnumUnknown},
		{"wrong_type", 123, fakeStrEnumUnknown},
		{"object", struct{}{}, fakeStrEnumUnknown},
	}
	for _, c := range cases {
		if got := FromStr(c.value, fakeStrMembers, fakeStrNames, fakeStrEnumUnknown); got != c.member {
			t.Errorf("%s: got %q, want %q", c.id, got, c.member)
		}
	}
}

func TestFromStrNoUnknown(t *testing.T) {
	members := []fakeStrEnum{fakeStrEnumFoo, fakeStrEnumBar}
	names := map[string]fakeStrEnum{"FOO": fakeStrEnumFoo, "BAR": fakeStrEnumBar}
	if _, ok := FromStrOK("unknown_value", members, names); ok {
		t.Fatal("expected no resolution")
	}
	err := &ErrNoUnknown{Enum: "FakeStrEnumNoUnknown", Value: "unknown_value"}
	want := "FakeStrEnumNoUnknown.from_value: cannot resolve 'unknown_value' and no `UNKNOWN` member is defined"
	if err.Error() != want {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFromInt(t *testing.T) {
	cases := []struct {
		id     string
		value  any
		member fakeIntEnum
	}{
		{"actual_enum", fakeIntEnumOne, fakeIntEnumOne},
		{"from_correct_value", 1, fakeIntEnumOne},
		{"from_parsed_value", "2", fakeIntEnumTwo},
		{"from_correct_key", "one", fakeIntEnumOne},
		{"from_parsed_key", "  two  ", fakeIntEnumTwo},
		{"value_does_not_exist", "three", fakeIntEnumUnknown},
		{"not_value_not_key", 4, fakeIntEnumUnknown},
		{"not_compatible", nil, fakeIntEnumUnknown},
		{"wrong_type", []any{1}, fakeIntEnumUnknown},
		{"object", struct{}{}, fakeIntEnumUnknown},
	}
	for _, c := range cases {
		if got := FromInt(c.value, fakeIntMembers, fakeIntNames, fakeIntEnumUnknown); got != c.member {
			t.Errorf("%s: got %d, want %d", c.id, got, c.member)
		}
	}
}

func TestFromIntNoUnknown(t *testing.T) {
	members := []fakeIntEnum{fakeIntEnumOne, fakeIntEnumTwo}
	names := map[string]fakeIntEnum{"ONE": fakeIntEnumOne, "TWO": fakeIntEnumTwo}
	if _, ok := FromIntOK("unknown_value", members, names); ok {
		t.Fatal("expected no resolution")
	}
}
