package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// EncodeGolay24128 returns the 24-bit Golay (24,12,8) codeword of the low
// 12 bits of data: the Golay (23,12,7) codeword in bits 23 to 1 and the
// overall parity bit in bit 0.
func EncodeGolay24128(data uint16) uint32 {
	return golay_24_12_8_table[data&0xFFF]
}

// DecodeGolay24128 decodes a 24-bit word using the Golay (24,12,8) code.
// It returns the corrected 12-bit data and FECResult. When the overall parity
// shows an uncorrectable error, it still returns the nearest Golay (23,12)
// codeword's data and sets Uncorrectable, as MMDVMHost and mbelib do for the
// AMBE voice FEC.
func DecodeGolay24128(received uint32) (data uint16, result fec.FECResult) {
	result.BitsChecked = 24

	// Compute the 12-bit syndrome:
	// - Upper 11 bits: polynomial syndrome of the 23-bit portion (bits 1..23)
	// - Lowest bit: overall parity error
	err23 := received >> 1
	polySyndrome := golay23ComputeSyndrome(err23)
	parityErr := bits.OnesCount32(received) & 1
	syndrome := (polySyndrome << 1) | parityErr

	if syndrome == 0 {
		// No errors detected
		return uint16(received >> 12), result //nolint:gosec // received>>12 is at most 12 bits, fits in uint16
	}

	// Look up the error pattern from the syndrome table
	errPattern := golay_24_12_8_syndrome_table[syndrome]
	if errPattern == 0xFFFFFFFF {
		data, _ = DecodeGolay23127(err23)
		result.Uncorrectable = true
		return data, result
	}

	// Apply correction
	corrected := received ^ errPattern
	result.ErrorsCorrected = bits.OnesCount32(errPattern)

	// Extract the 12 data bits (upper 12 bits of the 24-bit codeword)
	return uint16(corrected >> 12), result //nolint:gosec // corrected>>12 is at most 12 bits, fits in uint16
}
