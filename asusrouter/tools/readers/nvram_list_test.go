package readers

import (
	"reflect"
	"testing"
)

func TestDecode(t *testing.T) {
	// Char-encoded separators decode to `<`/`>`
	if got := Decode("&#60a&#62b"); got != "<a>b" {
		t.Errorf("decodes separators: %q", got)
	}
	// A plain string passes through unchanged
	if got := Decode("<a>b"); got != "<a>b" {
		t.Errorf("plain passthrough: %q", got)
	}
	// A non-string yields an empty string
	for _, raw := range []any{nil, 5, []any{"<a>"}} {
		if got := Decode(raw); got != "" {
			t.Errorf("non string %#v: %q", raw, got)
		}
	}
}

func TestSplitRows(t *testing.T) {
	// Rows split on `<`, decoded first
	if got := SplitRows("&#60a&#62b&#60c"); !reflect.DeepEqual(got, []string{"", "a>b", "c"}) {
		t.Errorf("splits rows: %q", got)
	}
	// Empty rows are kept, so positional lists stay aligned
	if got := SplitRows("<a<<b"); !reflect.DeepEqual(got, []string{"", "a", "", "b"}) {
		t.Errorf("keeps empty rows: %q", got)
	}
	// No usable data yields no rows
	for _, raw := range []any{nil, "", 5} {
		if got := SplitRows(raw); len(got) != 0 {
			t.Errorf("no data %#v: %q", raw, got)
		}
	}
}

func TestGetField(t *testing.T) {
	// A present field is returned as-is
	if got := GetField([]string{"a", "b"}, 1); got == nil || *got != "b" {
		t.Errorf("present: %v", got)
	}
	// An absent or negative index yields None
	if GetField([]string{"a"}, 5) != nil || GetField([]string{"a"}, -1) != nil {
		t.Error("out of range should be nil")
	}
}
