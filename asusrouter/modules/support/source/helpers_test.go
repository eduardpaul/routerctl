package source

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

// d is a raw support payload.
type d = map[string]any

// k renders a support value as its raw payload key.
func k(v support.ARSupportValue) string { return string(v) }

// check runs a translator over a case table, comparing with DeepEqual.
func check[T any](t *testing.T, name string, f func(map[string]any) T, cases []struct {
	data     d
	expected T
}) {
	t.Helper()
	for i, c := range cases {
		if got := f(c.data); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("%s case %d (%v): got %#v, want %#v", name, i, c.data, got, c.expected)
		}
	}
}

type boolCase = struct {
	data     d
	expected bool
}
