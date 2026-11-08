// Integer converters for AsusRouter.
package converters

import "math/bits"

// IntToBits mirrors `int_to_bits`: return the bit positions that are set
// in value, sorted ascending (Python returns a frozenset).
func IntToBits(value int) []int {
	if value <= 0 {
		return []int{}
	}
	result := []int{}
	v := uint64(value)
	for v != 0 {
		result = append(result, bits.TrailingZeros64(v))
		v &= v - 1
	}
	return result
}
