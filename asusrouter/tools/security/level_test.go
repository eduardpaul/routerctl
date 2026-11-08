package security

import "testing"

func TestARSecurityLevelMembers(t *testing.T) {
	cases := map[string]int{"UNKNOWN": -999, "STRICT": 0, "DEFAULT": 1, "SANITIZED": 5, "REASONABLE": 7, "UNSAFE": 9}
	for name, value := range cases {
		if int(aRSecurityLevelByName[name]) != value || aRSecurityLevelByName[name].Name() != name {
			t.Errorf("%s != %d", name, value)
		}
	}
}

func TestARSecurityLevelOrdering(t *testing.T) {
	if !(ARSecurityLevelStrict < ARSecurityLevelDefault &&
		ARSecurityLevelDefault < ARSecurityLevelSanitized &&
		ARSecurityLevelSanitized < ARSecurityLevelReasonable &&
		ARSecurityLevelReasonable < ARSecurityLevelUnsafe) {
		t.Fatal("bad ordering")
	}
}

func TestARSecurityLevelUnique(t *testing.T) {
	seen := map[ARSecurityLevel]bool{}
	for _, m := range ARSecurityLevelMembers {
		if seen[m] {
			t.Fatalf("duplicate %d", m)
		}
		seen[m] = true
	}
}

func TestARSecurityLevelFromValue(t *testing.T) {
	cases := []struct {
		value    any
		expected ARSecurityLevel
	}{
		{ARSecurityLevelDefault, ARSecurityLevelDefault},
		{1, ARSecurityLevelDefault},
		{"1", ARSecurityLevelDefault},
		{"5.0", ARSecurityLevelSanitized},
		{7, ARSecurityLevelReasonable},
		{861, ARSecurityLevelUnknown},
		{"sanitized", ARSecurityLevelSanitized},
		{"REASONABLE", ARSecurityLevelReasonable},
		{nil, ARSecurityLevelUnknown},
		{"unknown-name", ARSecurityLevelUnknown},
		{struct{}{}, ARSecurityLevelUnknown},
	}
	for _, c := range cases {
		if got := ARSecurityLevelFromValue(c.value); got != c.expected {
			t.Errorf("FromValue(%#v) = %d, want %d", c.value, got, c.expected)
		}
	}
}
