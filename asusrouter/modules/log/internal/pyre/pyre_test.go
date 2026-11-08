package pyre

import (
	"reflect"
	"testing"
)

func TestTranslateMatchesLikePython(t *testing.T) {
	cases := []struct {
		pattern, text string
		want          bool
	}{
		{`^\d+$`, "١٢٣", true},             // Unicode digits
		{`^\d+$`, "12\n", true},            // `$` before a final newline
		{`^\d+$`, "12\n\n", false},         // but only one
		{`^\w+$`, "héllo_1", true},         // Unicode word
		{`^\W$`, "-", true},                // non-word
		{`^a\sb$`, "a b", true},            // Unicode space
		{`^a\sb$`, "a\vb", true},           // `\v` is a space in Python
		{`^a\Sb$`, "a b", false},           // negated space
		{`^[\d.]+$`, "1.٢", true},          // shorthand inside a class
		{`^[^\]]+$`, "abc", true},          // escaped bracket in a class
		{`^[]a]+$`, "]a", true},            // leading `]` is literal
		{`^\$\d`, "$1", true},              // escaped dollar stays literal
		{`^[$]$`, "$", true},               // dollar in a class stays literal
		{`x\Z`, "x", true},                 // \Z is the very end
		{`x\Z`, "x\n", false},              // even before a newline
		{`^\D$`, "a", true},                // non-digit
		{`(?P<name>\d+)`, "a1", true},      // named group
		{`\"(?P<a>[^\"]+)\"`, `"x"`, true}, // escaped quote
	}
	for _, tc := range cases {
		re := Compile(tc.pattern)
		if got := re.MatchString(tc.text); got != tc.want {
			t.Errorf("%q on %q: got %v, want %v", tc.pattern, tc.text, got, tc.want)
		}
	}
}

func TestTranslateRejects(t *testing.T) {
	for _, pattern := range []string{`[\W]`, `[\S]`, `\b`, `\B`, `x\`} {
		if _, err := Translate(pattern); err == nil {
			t.Errorf("%q: no error", pattern)
		}
	}
}

func TestSplit(t *testing.T) {
	re := Compile(`(\d)(x)?`)
	got := Split(re, "a1b2xc")
	want := []string{"a", "1", "", "b", "2", "x", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		text string
		base int
		want int
		ok   bool
	}{
		{"12", 10, 12, true},
		{" +12 ", 10, 12, true},
		{"-1_000", 10, -1000, true},
		{"١٢", 10, 12, true},
		{"0x1F", 16, 31, true},
		{"1f", 16, 31, true},
		{"0x_1f", 16, 31, true},
		{"0x1f", 10, 0, false},
		{"1__0", 10, 0, false},
		{"_1", 10, 0, false},
		{"", 10, 0, false},
		{"12a", 10, 0, false},
		{"010", 0, 0, false},
		{"0o17", 0, 15, true},
		{"99999999999999999999", 10, 0, false},
	}
	for _, tc := range cases {
		got, ok := Int(tc.text, tc.base)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("Int(%q, %d) = %d, %v", tc.text, tc.base, got, ok)
		}
	}
}
