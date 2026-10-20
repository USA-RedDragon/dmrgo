package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Encode24128 returns the 24-bit Golay (24,12,8) codeword of the low
// 12 bits of data: the Golay (23,12,7) codeword in bits 23 to 1 and the
// overall parity bit in bit 0.
func Encode24128(data uint16) uint32 {
	return golay_24_12_8_table[data&0xFFF]
}

// Decode24128 decodes a 24-bit Golay (24,12,8) word into 12 data bits. When
// the overall parity shows an uncorrectable error, it sets Uncorrectable and
// returns the data of the nearest Golay (23,12) codeword, as MMDVMHost and
// mbelib do. It ignores bits above bit 23 and sets Uncorrectable if any is set.
func Decode24128(received uint32) (data uint16, result fec.Result) {
	result.BitsChecked = 24
	if received>>24 != 0 {
		received &= 1<<24 - 1
		data, result = Decode24128(received)
		result.Uncorrectable = true
		return data, result
	}

	err23 := received >> 1
	polySyndrome := golay23ComputeSyndrome(err23)
	parityErr := bits.OnesCount32(received) & 1
	syndrome := (polySyndrome << 1) | parityErr

	if syndrome == 0 {
		return uint16(received >> 12), result
	}

	errPattern := golay_24_12_8_syndrome_table[syndrome]
	if errPattern == 0xFFFFFFFF {
		data, _ = Decode23127(err23)
		result.Uncorrectable = true
		return data, result
	}

	corrected := received ^ errPattern
	result.ErrorsCorrected = bits.OnesCount32(errPattern)

	return uint16(corrected >> 12 & 0xFFFF), result
}
