package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

const golay23GenPoly = 0xC75

func golay23ComputeSyndrome(received uint32) int {
	dividend := received
	for i := 11; i >= 0; i-- {
		if dividend&(1<<(i+11)) != 0 {
			dividend ^= golay23GenPoly << i
		}
	}
	return int(dividend & 0x7FF)
}

// Encode23127 returns the 23-bit Golay (23,12,7) codeword of the low
// 12 bits of data: the data bits in bits 22 to 11, the parity in bits 10 to 0.
func Encode23127(data uint16) uint32 {
	return golay_23_12_7_table[data&0xFFF] >> 1
}

// Decode23127 decodes a 23-bit Golay (23,12,7) word into 12 data bits. The
// code is perfect, so it does not set Uncorrectable for a 23-bit word, and a
// word with 4 or more errors decodes to wrong data with ErrorsCorrected at
// most 3. It ignores bits above bit 22 and sets Uncorrectable if any is set.
func Decode23127(received uint32) (data uint16, result fec.Result) {
	result.BitsChecked = 23
	if received>>23 != 0 {
		received &= 1<<23 - 1
		data, result = Decode23127(received)
		result.Uncorrectable = true
		return data, result
	}

	syndrome := golay23ComputeSyndrome(received)

	if syndrome == 0 {
		return uint16(received >> 11), result
	}

	errPattern := golay_23_12_7_syndrome_table[syndrome]
	if errPattern == 0xFFFFFFFF {
		result.Uncorrectable = true
		return uint16(received >> 11), result
	}

	corrected := received ^ errPattern
	result.ErrorsCorrected = bits.OnesCount32(errPattern)

	return uint16(corrected >> 11 & 0xFFFF), result
}
