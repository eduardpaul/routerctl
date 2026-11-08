package readers

import (
	"reflect"
	"testing"
)

func TestReadJSSection(t *testing.T) {
	cases := []struct {
		id       string
		data     any
		key      string
		expected any
	}{
		{"dict", map[string]any{"a": []any{map[string]any{"x": int64(1)}}}, "a", map[string]any{"x": int64(1)}},
		{"list", map[string]any{"a": []any{[]any{int64(1), int64(2)}}}, "a", []any{int64(1), int64(2)}},
		{"empty", map[string]any{"a": []any{}}, "a", nil},
		{"not_list", map[string]any{"a": "x"}, "a", nil},
		{"absent", map[string]any{}, "a", nil},
		{"not_dict", "not-a-dict", "a", nil},
		// A tuple is not a list in Python
		{"tuple", map[string]any{"a": Tuple{int64(1)}}, "a", nil},
	}
	for _, c := range cases {
		if got := ReadJSSection(c.data, c.key); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("%s: got %#v, want %#v", c.id, got, c.expected)
		}
	}
}

func TestReadJSVariables(t *testing.T) {
	cases := []struct {
		content  string
		expected map[string]any
	}{
		// JS from the active temperature sensors
		{`curr_coreTmp_wl0_raw = "44&deg;C";`, map[string]any{"curr_coreTmp_wl0_raw": "44&deg;C"}},
		// JS from the disabled temperature sensors
		{`curr_coreTmp_wl2_raw = "<i>disabled</i>";`, map[string]any{"curr_coreTmp_wl2_raw": "<i>disabled</i>"}},
		// JS from the VPN
		{`vpn_client1_status = "None";`, map[string]any{"vpn_client1_status": "None"}},
		// JSON variables
		{"get_onboardinglist = [{}][0];", map[string]any{"get_onboardinglist": []any{map[string]any{}}}},
		// Value with array index, fallback to string
		{"arr = value[0];", map[string]any{"arr": "value"}},
		// Quoted string, not valid JSON or Python literal
		{"foo = 'bar';", map[string]any{"foo": "bar"}},
		{`baz = "qux";`, map[string]any{"baz": "qux"}},
		// Unquoted string, not valid JSON or Python literal
		{"plain = not_json;", map[string]any{"plain": "not_json"}},
		// Extra quotes
		{`extra = ""quoted"";`, map[string]any{"extra": `"quoted"`}},
		// Multi-line variable
		{
			"multi_line = {\n" + `    "key1": "value1",` + "\n    \n" + `    "key2": "value2"` + "\n};",
			map[string]any{"multi_line": map[string]any{"key1": "value1", "key2": "value2"}},
		},
		{"if (true) {\n    conditional = \"value\";\n};", map[string]any{"conditional": "value"}},
		// Weird formatting
		{"something var=42;", map[string]any{"var": int64(42)}},
		{"something var='4;2';", map[string]any{"var": "4;2"}},
		{`var="val42";`, map[string]any{"var": "val42"}},
		{`var="val;42";`, map[string]any{"var": "val;42"}},
		{`if(True){var="val42";}`, map[string]any{"var": "val42"}},
		{`if(True){ var="value;amp"; }`, map[string]any{"var": "value;amp"}},
		{`if(True){var="val;42";}`, map[string]any{"var": "val;42"}},
		// Other cases
		{`var="val\"42";`, map[string]any{"var": `val"42`}},
		{"var=42; // comment", map[string]any{"var": int64(42)}},
		{`var="foo"; /* block comment */`, map[string]any{"var": "foo"}},
		{"a=1; b=2;", map[string]any{"a": int64(1), "b": int64(2)}},
		{`   var =   "foo"   ;   `, map[string]any{"var": "foo"}},
		{"var=;", map[string]any{"var": ""}},
		// Non-string values
		{"num=123;", map[string]any{"num": int64(123)}},
		{"flag=true;", map[string]any{"flag": true}},
	}
	for _, c := range cases {
		if got := ReadJSVariables(c.content); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("ReadJSVariables(%q) = %#v, want %#v", c.content, got, c.expected)
		}
	}
}

// TestReadJSVariablesWordBoundary covers the Python `\b` the Go regex
// drops: names start at word boundaries and back-to-back assignments are
// all found (CPython gives the same results).
func TestReadJSVariablesWordBoundary(t *testing.T) {
	cases := []struct {
		content  string
		expected map[string]any
	}{
		{"xvar=1;", map[string]any{"xvar": int64(1)}},
		{"a=1;b=2;", map[string]any{"a": int64(1), "b": int64(2)}},
		{"x.y = 3;", map[string]any{"y": int64(3)}},
		{"é = 'u';", map[string]any{"é": "u"}},
		{"var a = 5;", map[string]any{"a": int64(5)}},
	}
	for _, c := range cases {
		if got := ReadJSVariables(c.content); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("ReadJSVariables(%q) = %#v, want %#v", c.content, got, c.expected)
		}
	}
}

// TestReadJSVariablesLiteralTypes checks the Go types literal_eval
// results map to.
func TestReadJSVariablesLiteralTypes(t *testing.T) {
	got := ReadJSVariables("t = (1, 'a'); s = {1}; d = {1: 'x'}; c = 1j; b = b'x'; n = None;")
	expected := map[string]any{
		"t": Tuple{int64(1), "a"},
		"s": Set{int64(1)},
		"d": map[any]any{int64(1): "x"},
		"c": complex(0, 1),
		"b": []byte("x"),
		"n": nil,
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("got %#v", got)
	}
}
