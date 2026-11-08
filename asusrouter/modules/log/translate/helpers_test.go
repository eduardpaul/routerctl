package translate

// Helpers mirroring tests/modules/log/translate/__init__.py.

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/log"
	"github.com/eduardpaul/routerctl/asusrouter/tools/identifiers"
	"github.com/eduardpaul/routerctl/asusrouter/tools/security"
)

// The levels the raw message is classified at, named for why it got one
const (
	IDENTIFIER = security.ARSecurityLevelReasonable
	UNREAD     = security.ARSecurityLevelUnsafe
)

// classified builds the raw message a pattern carrying `level` fields
// yields.
func classified(content string, level security.ARSecurityLevel) security.SensitiveText {
	return security.SensitiveText{Value: content, Level: level}
}

// unread builds the raw message of a line no pattern matched.
func unread(content string) security.SensitiveText {
	return classified(content, UNREAD)
}

// ev is a translated event.
type ev = map[log.AREventKey]any

// norm dereferences pointer values so identifiers compare by value.
func norm(v any) any {
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return rv.Elem().Interface()
	}
	return v
}

func normMap(m ev) ev {
	if m == nil {
		return nil
	}
	out := ev{}
	for k, v := range m {
		out[k] = norm(v)
	}
	return out
}

func show(m ev) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	s := "{"
	for _, k := range keys {
		v := m[log.AREventKey(k)]
		s += fmt.Sprintf("%s: %#v (%T), ", k, v, v)
	}
	return s + "}"
}

// assertEvent mirrors `assert translated == {...}`.
func assertEvent(t *testing.T, got, want ev) {
	t.Helper()
	g, w := normMap(got), normMap(want)
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("event mismatch\n got: %s\nwant: %s", show(g), show(w))
	}
}

// eq mirrors `assert event[key] == want`.
func eq(t *testing.T, event ev, key log.AREventKey, want any) {
	t.Helper()
	got, ok := event[key]
	if !ok {
		t.Fatalf("%s missing from %s", key, show(event))
	}
	if !reflect.DeepEqual(norm(got), norm(want)) {
		t.Fatalf("%s: got %#v (%T), want %#v (%T)", key, got, got, want, want)
	}
}

// absent mirrors `assert key not in event`.
func absent(t *testing.T, event ev, key log.AREventKey) {
	t.Helper()
	if v, ok := event[key]; ok {
		t.Fatalf("%s present (%#v) in %s", key, v, show(event))
	}
}

// present mirrors `assert key in event`.
func present(t *testing.T, event ev, key log.AREventKey) {
	t.Helper()
	if _, ok := event[key]; !ok {
		t.Fatalf("%s missing from %s", key, show(event))
	}
}

// tcase is one translation case: an exact event and/or extra checks.
type tcase struct {
	name    string
	content string
	want    ev
	check   func(t *testing.T, event ev)
}

func runCases(t *testing.T, set *ARLogPatternSet, cases []tcase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := set.Translate(tc.content)
			if tc.want != nil {
				assertEvent(t, event, tc.want)
			}
			if tc.check != nil {
				tc.check(t, event)
			}
		})
	}
}

func mac(t testing.TB, s string) identifiers.MacAddress {
	t.Helper()
	m, err := identifiers.NewMacAddress(s)
	if err != nil {
		t.Fatalf("mac %q: %v", s, err)
	}
	return m
}

func ip(t testing.TB, s string) identifiers.IpAddress {
	t.Helper()
	a, err := identifiers.NewIpAddress(s)
	if err != nil {
		t.Fatalf("ip %q: %v", s, err)
	}
	return a
}

func wl(unit, subunit int) identifiers.WiFiInterface {
	return identifiers.WiFiInterface{Unit: unit, Subunit: subunit}
}
