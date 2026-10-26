package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// Encode23127 returns the 23-bit Golay (23,12,7) codeword of the low
// 12 bits of data: the data bits in bits 22 to 11, the parity in bits 10 to 0.
func Encode23127(data uint16) uint32 {
	return Encode24128(data) >> 1
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

	errPattern, ok := errors24(received<<1 | uint32((bits.OnesCount32(received)+1)&1))
	if !ok {
		result.Uncorrectable = true
		return uint16(received >> 11), result
	}
	errPattern >>= 1
	result.ErrorsCorrected = bits.OnesCount32(errPattern)
	return uint16((received ^ errPattern) >> 11 & 0xFFFF), result
}
