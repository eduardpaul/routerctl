package converters

import (
	"reflect"
	"testing"
)

func TestIntToBits(t *testing.T) {
	cases := []struct {
		value    int
		expected []int
	}{
		// Zero and negative -> early return
		{0, []int{}}, {-1, []int{}}, {-999, []int{}},
		// Single bit at various positions
		{1, []int{0}}, {2, []int{1}}, {4, []int{2}}, {1 << 10, []int{10}}, {1 << 30, []int{30}},
		// Multiple consecutive bits
		{3, []int{0, 1}}, {7, []int{0, 1, 2}}, {15, []int{0, 1, 2, 3}},
		// Multiple non-consecutive bits
		{5, []int{0, 2}}, {9, []int{0, 3}},
		// Real-world capability bitmask: WAN(0) + LAN(1) + DUALWAN_PRIMARY(30)
		{(1 << 0) | (1 << 1) | (1 << 30), []int{0, 1, 30}},
	}
	for _, c := range cases {
		if got := IntToBits(c.value); !reflect.DeepEqual(got, c.expected) {
			t.Errorf("IntToBits(%d) = %v, want %v", c.value, got, c.expected)
		}
	}
}
