package network

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/parity/canonical"
)

// enc char-encodes `<`/`>` as the router does.
func enc(text string) string {
	return strings.NewReplacer("<", "&#60", ">", "&#62").Replace(text)
}

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

// canon renders a value in canonical JSON form for comparisons.
func canon(v any) string {
	b, _ := json.Marshal(canonical.Of(v))
	return string(b)
}

// assertCanon compares two values by their canonical form.
func assertCanon(t *testing.T, got, want any) {
	t.Helper()
	if g, w := canon(got), canon(want); g != w {
		t.Errorf("got %s, want %s", g, w)
	}
}

func assertEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func assertMissing[K comparable, V any](t *testing.T, m map[K]V, key K) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("unexpected key %v in %v", key, m)
	}
}
