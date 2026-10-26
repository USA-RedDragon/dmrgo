package golay

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

var (
	rows24 = [12]uint16{0x8eb, 0x93e, 0xa97, 0xdc6, 0x367, 0x6cd, 0xd99, 0x3da, 0x7b4, 0xf68, 0x63b, 0xc75}
	cols24 = [12]uint16{0xc75, 0x49f, 0x93e, 0x6e3, 0xdc6, 0xf13, 0xab9, 0x1ed, 0x3da, 0x7b4, 0xf68, 0xa4f}
)

func parity24(data uint32) uint32 {
	var p uint32
	for k := range 12 {
		p ^= uint32(rows24[k]) & -(data >> k & 1)
	}
	return p
}

func transpose24(s uint32) uint32 {
	var q uint32
	for k := range 12 {
		q ^= uint32(cols24[k]) & -(s >> k & 1)
	}
	return q
}

func errors24(received uint32) (uint32, bool) {
	s := parity24(received>>12) ^ received&0xFFF
	if bits.OnesCount32(s) <= 3 {
		return s, true
	}
	for k := range 12 {
		if e := s ^ uint32(rows24[k]); bits.OnesCount32(e) <= 2 {
			return 1<<(12+k) | e, true
		}
	}
	q := transpose24(s)
	if bits.OnesCount32(q) <= 3 {
		return q << 12, true
	}
	for k := range 12 {
		if e := q ^ uint32(cols24[k]); bits.OnesCount32(e) <= 2 {
			return e<<12 | 1<<k, true
		}
	}
	return 0, false
}

// Encode24128 returns the 24-bit Golay (24,12,8) codeword of the low
// 12 bits of data: the Golay (23,12,7) codeword in bits 23 to 1 and the
// overall parity bit in bit 0.
func Encode24128(data uint16) uint32 {
	d := uint32(data) & 0xFFF
	return d<<12 | parity24(d)
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

	errPattern, ok := errors24(received)
	if !ok {
		data, _ = Decode23127(received >> 1)
		result.Uncorrectable = true
		return data, result
	}

	result.ErrorsCorrected = bits.OnesCount32(errPattern)
	return uint16((received ^ errPattern) >> 12 & 0xFFFF), result
}
