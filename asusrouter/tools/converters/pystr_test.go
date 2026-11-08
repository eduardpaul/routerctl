package converters

import (
	"math"
	"testing"
)

func TestFloatRepr(t *testing.T) {
	cases := map[float64]string{
		1: "1.0", 0.1: "0.1", 1e16: "1e+16", 1e15: "1000000000000000.0",
		1.5e-5: "1.5e-05", 0.0001: "0.0001", -2.5: "-2.5", 1e100: "1e+100",
		math.Inf(1): "inf",
	}
	for f, want := range cases {
		if got := FloatRepr(f); got != want {
			t.Errorf("FloatRepr(%v) = %q, want %q", f, got, want)
		}
	}
}

func TestPyStr(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, "None"}, {true, "True"}, {3, "3"}, {2.0, "2.0"}, {"x", "x"},
		{[]any{"a", int64(1), nil}, "['a', 1, None]"},
		{map[string]any{"b": "it's", "a": 1.5}, `{'a': 1.5, 'b': "it's"}`},
	}
	for _, c := range cases {
		if got := PyStr(c.value); got != c.want {
			t.Errorf("PyStr(%#v) = %q, want %q", c.value, got, c.want)
		}
	}
}
