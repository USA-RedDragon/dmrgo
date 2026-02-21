package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// golay23GenPoly is the generator polynomial for Golay(23,12,7):
// g(x) = x^11 + x^10 + x^6 + x^5 + x^4 + x^2 + 1
const golay23GenPoly = 0xC75

// golay23ComputeSyndrome computes the 11-bit syndrome for a 23-bit received word
// by polynomial division: syndrome = received(x) mod g(x).
func golay23ComputeSyndrome(received uint32) int {
	dividend := received
	for i := 11; i >= 0; i-- {
		if dividend&(1<<(i+11)) != 0 {
			dividend ^= golay23GenPoly << i
		}
	}
	return int(dividend & 0x7FF)
}

// DecodeGolay23127 decodes a 23-bit word using the Golay (23,12,7) code.
// It returns the corrected 12-bit data and FECResult.
func DecodeGolay23127(received uint32) (data uint16, result fec.FECResult) {
	result.BitsChecked = 23

	// Compute the 11-bit syndrome via polynomial division
	syndrome := golay23ComputeSyndrome(received)

	if syndrome == 0 {
		// No errors detected
		return uint16(received >> 11), result //nolint:gosec // received>>11 is at most 12 bits, fits in uint16
	}

	// Look up the error pattern from the syndrome table
	errPattern := golay_23_12_7_syndrome_table[syndrome]
	if errPattern == 0xFFFFFFFF {
		result.Uncorrectable = true
		return uint16(received >> 11), result //nolint:gosec // received>>11 is at most 12 bits, fits in uint16
	}

	// Apply correction
	corrected := received ^ errPattern
	result.ErrorsCorrected = bits.OnesCount32(errPattern)

	// Extract the 12 data bits (upper 12 bits of the 23-bit codeword)
	return uint16(corrected >> 11), result //nolint:gosec // corrected>>11 is at most 12 bits, fits in uint16
}
