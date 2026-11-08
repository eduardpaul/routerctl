package network

import (
	"fmt"
	"strconv"
	"strings"
)

// ARNetworkHandleUnit is one `(unit, slot)` pair of a legacy handle; Slot
// is nil for the main network of the band.
//
// Python stores it as a plain `tuple[int, int | None]`; Go keeps a struct
// that canonicalizes as the same two-item list.
type ARNetworkHandleUnit struct {
	Unit int
	Slot *int
}

// CanonicalValue renders the pair as `[unit, slot]` like the Python tuple.
func (u ARNetworkHandleUnit) CanonicalValue() any {
	var slot any
	if u.Slot != nil {
		slot = *u.Slot
	}
	return []any{u.Unit, slot}
}

// equal compares two pairs by value.
func (u ARNetworkHandleUnit) equal(o ARNetworkHandleUnit) bool {
	if u.Unit != o.Unit {
		return false
	}
	if u.Slot == nil || o.Slot == nil {
		return u.Slot == nil && o.Slot == nil
	}
	return *u.Slot == *o.Slot
}

// ARNetworkHandle mirrors `ARNetworkHandle`: opaque round-trip target for
// enabling or disabling a network.
type ARNetworkHandle struct {
	Backend ARNetworkBackend `json:"backend"`

	// SDN
	SdnIdx   *int    `json:"sdn_idx"`
	ApPrefix *string `json:"ap_prefix"`
	ApIdx    *int    `json:"ap_idx"`

	// Legacy
	Units []ARNetworkHandleUnit `json:"units"`
}

// Equal mirrors the frozen dataclass `__eq__`: equal by value.
func (h ARNetworkHandle) Equal(o ARNetworkHandle) bool {
	if h.Backend != o.Backend || !eqInt(h.SdnIdx, o.SdnIdx) || !eqInt(h.ApIdx, o.ApIdx) {
		return false
	}
	if (h.ApPrefix == nil) != (o.ApPrefix == nil) || (h.ApPrefix != nil && *h.ApPrefix != *o.ApPrefix) {
		return false
	}
	if len(h.Units) != len(o.Units) {
		return false
	}
	for i := range h.Units {
		if !h.Units[i].equal(o.Units[i]) {
			return false
		}
	}
	return true
}

// String mirrors the dataclass `__repr__`. It is also the value used in
// action keys, so equal handles give equal keys (like `__hash__`).
func (h ARNetworkHandle) String() string {
	units := make([]string, len(h.Units))
	for i, u := range h.Units {
		units[i] = fmt.Sprintf("(%d, %s)", u.Unit, optInt(u.Slot))
	}
	tuple := "(" + strings.Join(units, ", ")
	if len(units) == 1 {
		tuple += ","
	}
	tuple += ")"
	prefix := "None"
	if h.ApPrefix != nil {
		prefix = strconv.Quote(*h.ApPrefix)
	}
	return fmt.Sprintf(
		"ARNetworkHandle(backend=%s, sdn_idx=%s, ap_prefix=%s, ap_idx=%s, units=%s)",
		h.Backend.Name(), optInt(h.SdnIdx), prefix, optInt(h.ApIdx), tuple,
	)
}

func eqInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func optInt(v *int) string {
	if v == nil {
		return "None"
	}
	return strconv.Itoa(*v)
}
