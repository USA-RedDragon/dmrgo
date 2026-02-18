package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

// DecodeGolay23127 decodes a 23-bit word using the Golay (23,12,7) code.
// It returns the corrected 12-bit data and FECResult.
func DecodeGolay23127(received uint32) (data uint16, result fec.FECResult) {
	result.BitsChecked = 23
	minDist := 23
	bestData := uint16(0)

	// Brute-force search
	for d, codeword := range Golay_23_12_7_table {
		// The encoding table stores values shifted left by 1 (bit 0 is always 0),
		// but the transmission discards bit 0 (>> 1).
		// So we must compare the received value against the table value shifted right.
		dist := bits.OnesCount32(received ^ (codeword >> 1))
		if dist < minDist {
			minDist = dist
			bestData = uint16(d) //nolint:gosec // d is a table index (0-4095), fits in uint16

			if dist == 0 {
				return bestData, result
			}
		}
	}

	// Golay (23,12,7) can correct up to 3 errors (d_min = 7, t = floor((7-1)/2) = 3)
	if minDist <= 3 {
		result.ErrorsCorrected = minDist
		return bestData, result
	}

	result.ErrorsCorrected = minDist
	result.Uncorrectable = true
	return bestData, result
}
