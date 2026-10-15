// Package lowbit clears all but the low bit of bit.Bit values, so the code
// of this module reads a soft-decision or out-of-range Bit as 0 or 1.
package lowbit

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// Mask clears all but the low bit of each element of bits in place.
func Mask(bits []bit.Bit) {
	for i := range bits {
		bits[i] &= 1
	}
}
