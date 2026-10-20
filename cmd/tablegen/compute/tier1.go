package compute

import "math/bits"

// Golay23_12_7Encoding computes the systematic Golay(23,12,7)
// codeword (d << 11) | (d*x^11 mod g(x)) of each 12-bit data value d, with
// g(x) = 0xC75. Each entry is the codeword shifted left by 1.
func Golay23_12_7Encoding() []uint32 {
	const genPoly = 0xC75
	table := make([]uint32, 4096)

	for d := 0; d < 4096; d++ {
		dividend := uint32(d) << 11
		for i := 11; i >= 0; i-- {
			if dividend&(1<<(i+11)) != 0 {
				dividend ^= uint32(genPoly) << i
			}
		}
		remainder := dividend & 0x7FF
		codeword := (uint32(d) << 11) | remainder
		table[d] = codeword << 1
	}

	return table
}

// Golay24_12_8Encoding computes the Golay(24,12,8) codeword of each
// 12-bit data value: the Golay(23,12,7) codeword shifted left by 1, with an
// even parity bit in bit 0.
func Golay24_12_8Encoding() []uint32 {
	const genPoly = 0xC75
	table := make([]uint32, 4096)

	for d := 0; d < 4096; d++ {
		dividend := uint32(d) << 11
		for i := 11; i >= 0; i-- {
			if dividend&(1<<(i+11)) != 0 {
				dividend ^= uint32(genPoly) << i
			}
		}
		remainder := dividend & 0x7FF
		codeword23 := (uint32(d) << 11) | remainder
		parity := uint32(bits.OnesCount32(codeword23) & 1)
		codeword24 := (codeword23 << 1) | parity
		table[d] = codeword24
	}

	return table
}

// EncoderStateTransition computes the 4-bit symbol of the rate 3/4
// trellis encoder at index state*8+tribit (ETSI TS 102 361-1 clause B.2.4).
func EncoderStateTransition() []byte {
	table := make([]byte, 64)

	for state := 0; state < 8; state++ {
		shift := (state&1)*2 + (state & 4)
		lsb := byte(((state >> 1) ^ (state >> 2)) & 1)

		for input := 0; input < 8; input++ {
			upper3 := reverseBits3(byte((input + shift) % 8))
			symbol := (upper3 << 1) | lsb
			table[state*8+input] = symbol
		}
	}

	return table
}

func reverseBits3(b byte) byte {
	return ((b & 1) << 2) | (b & 2) | ((b & 4) >> 2)
}

// InterleaveMatrix computes the rate 3/4 trellis interleave order of
// the 98 dibits: pairs of consecutive indexes in steps of 8, in 4 rows that
// start at 0, 2, 4 and 6 (ETSI TS 102 361-1 clause B.2.4).
func InterleaveMatrix() []byte {
	table := make([]byte, 0, 98)

	for g := 0; g < 13; g++ {
		base := g * 8
		table = append(table, byte(base), byte(base+1))
	}

	for r := 1; r <= 3; r++ {
		for g := 0; g < 12; g++ {
			base := 2*r + g*8
			table = append(table, byte(base), byte(base+1))
		}
	}

	return table
}

// Golay23_12_7Syndrome computes the error pattern of weight 0 to 3
// for each 11-bit syndrome of the Golay(23,12,7) code. Entries 2048 and up
// are 0xFFFFFFFF.
func Golay23_12_7Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	table[0] = 0x00000000

	for p := 0; p < 23; p++ {
		errPattern := uint32(1) << p
		syndrome := golay23Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	for p1 := 0; p1 < 23; p1++ {
		for p2 := p1 + 1; p2 < 23; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay23Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	for p1 := 0; p1 < 23; p1++ {
		for p2 := p1 + 1; p2 < 23; p2++ {
			for p3 := p2 + 1; p3 < 23; p3++ {
				errPattern := uint32(1)<<p1 | uint32(1)<<p2 | uint32(1)<<p3
				syndrome := golay23Syndrome(errPattern)
				if table[syndrome] == 0xFFFFFFFF {
					table[syndrome] = errPattern
				}
			}
		}
	}

	return table
}

func golay23Syndrome(errPattern uint32) int {
	dividend := errPattern
	for i := 11; i >= 0; i-- {
		if dividend&(1<<(i+11)) != 0 {
			dividend ^= uint32(0xC75) << i
		}
	}
	return int(dividend & 0x7FF)
}

// Golay24_12_8Syndrome computes the error pattern of weight 0 to 3
// for each 12-bit syndrome of the Golay(24,12,8) code, or 0xFFFFFFFF when
// there is none. The syndrome is the Golay(23,12,7) syndrome shifted left by
// 1, with the overall parity error in bit 0.
func Golay24_12_8Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	table[0] = 0x00000000

	for p := 0; p < 24; p++ {
		errPattern := uint32(1) << p
		syndrome := golay24Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	for p1 := 0; p1 < 24; p1++ {
		for p2 := p1 + 1; p2 < 24; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay24Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	for p1 := 0; p1 < 24; p1++ {
		for p2 := p1 + 1; p2 < 24; p2++ {
			for p3 := p2 + 1; p3 < 24; p3++ {
				errPattern := uint32(1)<<p1 | uint32(1)<<p2 | uint32(1)<<p3
				syndrome := golay24Syndrome(errPattern)
				if table[syndrome] == 0xFFFFFFFF {
					table[syndrome] = errPattern
				}
			}
		}
	}

	return table
}

func golay24Syndrome(errPattern uint32) int {
	err23 := errPattern >> 1
	polySyndrome := golay23Syndrome(err23)
	parityErr := bits.OnesCount32(errPattern) & 1
	return (polySyndrome << 1) | parityErr
}
