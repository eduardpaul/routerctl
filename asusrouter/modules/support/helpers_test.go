package support_test

import (
	"reflect"
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/connection"
	"github.com/eduardpaul/routerctl/asusrouter/modules/support"
)

// Python passes non-dict values ("not_a_dict", None); a Go translator
// takes a map, so both are represented by a nil map.

func TestMakeBoolTranslator(t *testing.T) {
	cases := []struct {
		key      string
		negate   bool
		data     map[string]any
		expected bool
	}{
		// negate=False: absent or false -> False
		{"k", false, map[string]any{}, false},
		{"k", false, map[string]any{"k": 0}, false},
		{"k", false, map[string]any{"k": "0"}, false},
		{"k", false, map[string]any{"other": 1}, false},
		// negate=False: key true -> True
		{"k", false, map[string]any{"k": 1}, true},
		{"k", false, map[string]any{"k": "1"}, true},
		// negate=False: non-dict -> False
		{"k", false, nil, false},
		// negate=True: key absent or false -> True
		{"k", true, map[string]any{}, true},
		{"k", true, map[string]any{"k": 0}, true},
		// negate=True: key true -> False
		{"k", true, map[string]any{"k": 1}, false},
		{"k", true, map[string]any{"k": "1"}, false},
		// negate=True: non-dict -> False
		{"k", true, nil, false},
	}
	for i, c := range cases {
		if got := support.MakeBoolTranslator(c.key, c.negate)(c.data); got != c.expected {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

func entries(kv ...string) []support.TableEntry[string] {
	out := []support.TableEntry[string]{}
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, support.TableEntry[string]{Key: kv[i], Value: kv[i+1]})
	}
	return out
}

func TestMakeEnumTranslator(t *testing.T) {
	cases := []struct {
		table    []support.TableEntry[string]
		def      string
		data     map[string]any
		expected string
	}{
		// no match -> default
		{entries("k1", "v1"), "d", map[string]any{}, "d"},
		{entries("k1", "v1"), "d", map[string]any{"k1": 0}, "d"},
		{entries("k1", "v1"), "d", map[string]any{"k1": "0"}, "d"},
		{entries("k1", "v1"), "d", map[string]any{"k2": 1}, "d"},
		// single key match
		{entries("k1", "v1"), "d", map[string]any{"k1": 1}, "v1"},
		{entries("k1", "v1"), "d", map[string]any{"k1": "1"}, "v1"},
		// first key wins
		{entries("k1", "v1", "k2", "v2"), "d", map[string]any{"k1": 1, "k2": 1}, "v1"},
		// second key when first absent
		{entries("k1", "v1", "k2", "v2"), "d", map[string]any{"k2": 1}, "v2"},
		// non-dict -> default
		{entries("k1", "v1"), "d", nil, "d"},
	}
	for i, c := range cases {
		if got := support.MakeEnumTranslator(c.table, c.def)(c.data); got != c.expected {
			t.Errorf("case %d: got %q, want %q", i, got, c.expected)
		}
	}
}

func TestMakeIntTranslator(t *testing.T) {
	cases := []struct {
		key      string
		data     map[string]any
		expected int
	}{
		// absent -> 0
		{"k", map[string]any{}, 0},
		// zero values -> 0
		{"k", map[string]any{"k": 0}, 0},
		{"k", map[string]any{"k": "0"}, 0},
		// positive values
		{"k", map[string]any{"k": 1}, 1},
		{"k", map[string]any{"k": 2}, 2},
		{"k", map[string]any{"k": "1"}, 1},
		// invalid string -> 0
		{"k", map[string]any{"k": "invalid"}, 0},
		// wrong key -> 0
		{"k", map[string]any{"other": 1}, 0},
		// non-dict -> 0
		{"k", nil, 0},
	}
	for i, c := range cases {
		if got := support.MakeIntTranslator(c.key)(c.data); got != c.expected {
			t.Errorf("case %d: got %d, want %d", i, got, c.expected)
		}
	}
}

func TestMakeListTranslator(t *testing.T) {
	cases := []struct {
		table    []support.TableEntry[string]
		data     map[string]any
		expected []string
	}{
		// empty dict -> empty list
		{entries("k1", "v1"), map[string]any{}, []string{}},
		// key present -> item in list
		{entries("k1", "v1"), map[string]any{"k1": 1}, []string{"v1"}},
		{entries("k1", "v1"), map[string]any{"k1": "1"}, []string{"v1"}},
		// key false -> empty
		{entries("k1", "v1"), map[string]any{"k1": 0}, []string{}},
		{entries("k1", "v1"), map[string]any{"k1": "0"}, []string{}},
		// wrong key -> empty
		{entries("k1", "v1"), map[string]any{"k2": 1}, []string{}},
		// multiple keys - all true
		{entries("k1", "v1", "k2", "v2"), map[string]any{"k1": 1, "k2": 1}, []string{"v1", "v2"}},
		// multiple keys - partial
		{entries("k1", "v1", "k2", "v2"), map[string]any{"k1": 1}, []string{"v1"}},
		{entries("k1", "v1", "k2", "v2"), map[string]any{"k2": 1}, []string{"v2"}},
		// non-dict -> empty
		{entries("k1", "v1"), nil, []string{}},
	}
	for i, c := range cases {
		if got := support.MakeListTranslator(c.table)(c.data); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

type supportMap = map[support.ARSupportType]any

func TestSupportAvailable(t *testing.T) {
	cases := []struct {
		support  supportMap
		key      support.ARSupportType
		expected bool
	}{
		// True flag -> True
		{supportMap{support.ARSupportTypeAi: true}, support.ARSupportTypeAi, true},
		// False flag -> False
		{supportMap{support.ARSupportTypeAi: false}, support.ARSupportTypeAi, false},
		// int 1 is not True -> False
		{supportMap{support.ARSupportTypeAi: 1}, support.ARSupportTypeAi, false},
		// absent key -> False
		{supportMap{}, support.ARSupportTypeAi, false},
		// other key true, queried absent -> False
		{supportMap{support.ARSupportTypeWan: true}, support.ARSupportTypeAi, false},
	}
	for i, c := range cases {
		if got := support.SupportAvailable(c.support, c.key); got != c.expected {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

func TestSupportAvailableIn(t *testing.T) {
	conn := support.ARSupportTypeConnections
	https, ssh := connection.ARConnectionHttps, connection.ARConnectionSsh
	cases := []struct {
		support  supportMap
		item     any
		expected bool
	}{
		// item in list -> True
		{supportMap{conn: []connection.ARConnection{https}}, https, true},
		// item not in list -> False
		{supportMap{conn: []connection.ARConnection{https}}, ssh, false},
		// empty list -> False
		{supportMap{conn: []connection.ARConnection{}}, https, false},
		// item found in multi-item list -> True
		{supportMap{conn: []connection.ARConnection{https, ssh}}, ssh, true},
		// key absent -> False
		{supportMap{}, https, false},
		// non-list value -> False
		{supportMap{conn: true}, https, false},
		// dict capabilities -> membership by key
		{supportMap{conn: map[connection.ARConnection]any{https: true}}, https, true},
		{supportMap{conn: map[connection.ARConnection]any{https: true}}, ssh, false},
	}
	for i, c := range cases {
		if got := support.SupportAvailableIn(c.support, conn, c.item); got != c.expected {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

func TestSupportValue(t *testing.T) {
	cases := []struct {
		support  supportMap
		key      support.ARSupportType
		expected any
	}{
		// bool value
		{supportMap{support.ARSupportTypeAi: true}, support.ARSupportTypeAi, true},
		// int value (support_value returns whatever is stored)
		{supportMap{support.ARSupportTypeSpeedtest: 2}, support.ARSupportTypeSpeedtest, 2},
		// list value
		{
			supportMap{support.ARSupportTypeConnections: []connection.ARConnection{connection.ARConnectionHttps}},
			support.ARSupportTypeConnections,
			[]connection.ARConnection{connection.ARConnectionHttps},
		},
		// absent key -> None
		{supportMap{}, support.ARSupportTypeAi, nil},
	}
	for i, c := range cases {
		if got := support.SupportValue(c.support, c.key); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}

func TestSupportCapability(t *testing.T) {
	conn := support.ARSupportTypeConnections
	https := connection.ARConnectionHttps
	cases := []struct {
		support    supportMap
		key        support.ARSupportType
		capability any
		expected   any
	}{
		// capability present in the dict
		{supportMap{conn: map[connection.ARConnection]any{https: 5}}, conn, https, 5},
		// capability missing from the dict -> None
		{supportMap{conn: map[connection.ARConnection]any{}}, conn, https, nil},
		// non-dict value -> None
		{supportMap{support.ARSupportTypeAi: true}, support.ARSupportTypeAi, https, nil},
		// key absent -> None
		{supportMap{}, support.ARSupportTypeAi, https, nil},
	}
	for i, c := range cases {
		if got := support.SupportCapability(c.support, c.key, c.capability); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("case %d: got %v, want %v", i, got, c.expected)
		}
	}
}
