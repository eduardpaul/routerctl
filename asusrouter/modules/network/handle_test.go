package network

import (
	"testing"

	"github.com/eduardpaul/routerctl/asusrouter/modules/source"
)

func TestARNetworkHandleDefaults(t *testing.T) {
	// The backend is the only required field; the rest default empty
	handle := ARNetworkHandle{Backend: ARNetworkBackendSdn}
	if handle.Backend != ARNetworkBackendSdn || handle.SdnIdx != nil || handle.ApPrefix != nil || handle.ApIdx != nil || len(handle.Units) != 0 {
		t.Errorf("unexpected defaults: %+v", handle)
	}
	assertCanon(t, handle, map[string]any{
		"backend": "sdn", "sdn_idx": nil, "ap_prefix": nil, "ap_idx": nil, "units": []any{},
	})
}

func TestARNetworkHandleCarriesFields(t *testing.T) {
	// Provided SDN and legacy fields are stored
	handle := ARNetworkHandle{
		Backend:  ARNetworkBackendLegacy,
		SdnIdx:   intp(2),
		ApPrefix: strp("wl"),
		ApIdx:    intp(1),
		Units:    []ARNetworkHandleUnit{{Unit: 0}, {Unit: 1, Slot: intp(2)}},
	}
	assertEqual(t, *handle.SdnIdx, 2)
	assertCanon(t, handle.Units, []any{[]any{0, nil}, []any{1, 2}})
}

func TestARNetworkHandleEqualByValue(t *testing.T) {
	// Handles with the same fields compare equal and hash alike
	a := ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(1)}
	b := ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(1)}
	if !a.Equal(b) {
		t.Error("handles should be equal")
	}
	if a.String() != b.String() {
		t.Errorf("repr differs: %s vs %s", a, b)
	}
	ka := source.IDOf(keyed{a.String()})
	kb := source.IDOf(keyed{b.String()})
	if ka != kb {
		t.Errorf("keys differ: %s vs %s", ka, kb)
	}
	c := ARNetworkHandle{Backend: ARNetworkBackendSdn, SdnIdx: intp(2)}
	if a.Equal(c) {
		t.Error("handles should differ")
	}
}

// keyed wraps a key value as a data source for ID comparisons.
type keyed struct{ key any }

func (keyed) SourceName() string { return "keyed" }
func (k keyed) SourceKey() []any { return []any{k.key} }
