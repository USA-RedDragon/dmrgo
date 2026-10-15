package compute

import "math/bits"

// Golay23_12_7Encoding computes the systematic Golay(23,12,7)
// codeword (d << 11) | (d*x^11 mod g(x)) of each 12-bit data value d, with
// g(x) = 0xC75. Each entry is the codeword shifted left by 1.
func Golay23_12_7Encoding() []uint32 {
	const genPoly = 0xC75
	table := make([]uint32, 4096)

	for d := 0; d < 4096; d++ {
		dividend := uint32(d) << 11 //nolint:gosec // d is in [0,4095], fits in uint32
		for i := 11; i >= 0; i-- {
			if dividend&(1<<(i+11)) != 0 {
				dividend ^= uint32(genPoly) << i
			}
		}
		remainder := dividend & 0x7FF
		codeword := (uint32(d) << 11) | remainder //nolint:gosec // d is in [0,4095], fits in uint32
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
		dividend := uint32(d) << 11 //nolint:gosec // d is in [0,4095], fits in uint32
		for i := 11; i >= 0; i-- {
			if dividend&(1<<(i+11)) != 0 {
				dividend ^= uint32(genPoly) << i
			}
		}
		remainder := dividend & 0x7FF
		codeword23 := (uint32(d) << 11) | remainder        //nolint:gosec // d is in [0,4095], fits in uint32
		parity := uint32(bits.OnesCount32(codeword23)) & 1 //nolint:gosec // OnesCount32 returns [0,32], fits in uint32
		codeword24 := (codeword23 << 1) | parity
		table[d] = codeword24
	}

	return table
}

// QR16_7_6Syndrome computes the error pattern of weight 0 to 2 for each
// 9-bit syndrome of the QR(16,7,6) code, or 0xFFFF when there is none (ETSI
// TS 102 361-1 clause B.3.2).
func QR16_7_6Syndrome() [512]uint16 {
	var table [512]uint16
	for i := range table {
		table[i] = 0xFFFF
	}

	for weight := 0; weight <= 2; weight++ {
		if weight == 0 {
			table[0] = 0x0000
			continue
		}
		if weight == 1 {
			for p := 0; p < 16; p++ {
				errPattern := uint16(1) << p
				syndrome := qrSyndrome(errPattern)
				table[syndrome] = errPattern
			}
			continue
		}
		for p1 := 0; p1 < 16; p1++ {
			for p2 := p1 + 1; p2 < 16; p2++ {
				errPattern := uint16(1)<<p1 | uint16(1)<<p2
				syndrome := qrSyndrome(errPattern)
				if table[syndrome] == 0xFFFF {
					table[syndrome] = errPattern
				}
			}
		}
	}

	return table
}

// qrSyndrome computes the 9-bit syndrome of a QR(16,7,6) error pattern with
// data in bits 15 to 9 and parity in bits 8 to 0.
func qrSyndrome(errPattern uint16) int {
	var dataErr [7]byte
	for i := 0; i < 7; i++ {
		if errPattern&(1<<(15-i)) != 0 {
			dataErr[i] = 1
		}
	}

	calcParity := qrParityBits(dataErr)

	var parityErr [9]byte
	for i := 0; i < 9; i++ {
		if errPattern&(1<<(8-i)) != 0 {
			parityErr[i] = 1
		}
	}

	syndrome := 0
	for i := 0; i < 9; i++ {
		if calcParity[i] != parityErr[i] {
			syndrome |= 1 << (8 - i)
		}
	}

	return syndrome
}

// qrParityBits computes the 9 QR(16,7,6) parity bits of 7 data bits.
func qrParityBits(bits [7]byte) [9]byte {
	return [9]byte{
		bits[1] ^ bits[2] ^ bits[3] ^ bits[4],
		bits[2] ^ bits[3] ^ bits[4] ^ bits[5],
		bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6],
		bits[2] ^ bits[3] ^ bits[5] ^ bits[6],
		bits[1] ^ bits[2] ^ bits[6],
		bits[0] ^ bits[1] ^ bits[4],
		bits[0] ^ bits[1] ^ bits[2] ^ bits[5],
		bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6],
		bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6],
	}
}

// QR16_7_6Encoding computes the QR(16,7,6) codeword of each 7-bit data
// value, with data in bits 15 to 9 and parity in bits 8 to 0 (ETSI TS 102
// 361-1 clause B.3.2).
func QR16_7_6Encoding() []uint16 {
	table := make([]uint16, 128)

	for d := 0; d < 128; d++ {
		var bits [7]byte
		for i := 0; i < 7; i++ {
			if d&(1<<(6-i)) != 0 {
				bits[i] = 1
			}
		}
		parity := qrParityBits(bits)

		codeword := uint16(d) << 9 //nolint:gosec // d is in [0,127], fits in uint16
		for i := 0; i < 9; i++ {
			codeword |= uint16(parity[i]) << (8 - i)
		}
		table[d] = codeword
	}

	return table
}

// Golay20_8_7Encoding computes the Golay(20,8,7) codeword of each 8-bit
// data value, with data in bits 0 to 7 and parity in bits 8 to 19 (ETSI TS
// 102 361-1 clause B.3.1).
func Golay20_8_7Encoding() []uint32 {
	table := make([]uint32, 256)

	for d := 0; d < 256; d++ {
		var bits [8]byte
		for i := 0; i < 8; i++ {
			if d&(1<<i) != 0 {
				bits[i] = 1
			}
		}
		parity := golay20ParityBits(bits)
		codeword := uint32(d) //nolint:gosec // d is in [0,255], fits in uint32
		for i := 0; i < 12; i++ {
			codeword |= uint32(parity[i]) << (8 + i)
		}
		table[d] = codeword
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

// reverseBits3 reverses the 3 least significant bits.
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

// Golay20_8_7Syndrome computes the error pattern of weight 0 to 3 for
// each 12-bit syndrome of the Golay(20,8,7) code, or 0xFFFFFFFF when there is
// none (ETSI TS 102 361-1 clause B.3.1).
func Golay20_8_7Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	table[0] = 0x00000000

	for p := 0; p < 20; p++ {
		errPattern := uint32(1) << p
		syndrome := golay20Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	for p1 := 0; p1 < 20; p1++ {
		for p2 := p1 + 1; p2 < 20; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay20Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	for p1 := 0; p1 < 20; p1++ {
		for p2 := p1 + 1; p2 < 20; p2++ {
			for p3 := p2 + 1; p3 < 20; p3++ {
				errPattern := uint32(1)<<p1 | uint32(1)<<p2 | uint32(1)<<p3
				syndrome := golay20Syndrome(errPattern)
				if table[syndrome] == 0xFFFFFFFF {
					table[syndrome] = errPattern
				}
			}
		}
	}

	return table
}

// golay20Syndrome computes the 12-bit syndrome of a Golay(20,8,7) error
// pattern with data in bits 0 to 7 and parity in bits 8 to 19.
func golay20Syndrome(errPattern uint32) int {
	var dataErr [8]byte
	for i := 0; i < 8; i++ {
		if errPattern&(1<<i) != 0 {
			dataErr[i] = 1
		}
	}

	calcParity := golay20ParityBits(dataErr)

	syndrome := 0
	for i := 0; i < 12; i++ {
		parityErr := byte(0)
		if errPattern&(1<<(8+i)) != 0 {
			parityErr = 1
		}
		if calcParity[i] != parityErr {
			syndrome |= 1 << i
		}
	}

	return syndrome
}

// golay20ParityBits computes the 12 Golay(20,8,7) parity bits of 8 data bits.
func golay20ParityBits(bits [8]byte) [12]byte {
	return [12]byte{
		bits[1] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7],
		bits[1] ^ bits[2] ^ bits[4],
		bits[0] ^ bits[2] ^ bits[3] ^ bits[5],
		bits[0] ^ bits[1] ^ bits[3] ^ bits[4] ^ bits[6],
		bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[7],
		bits[0] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[7],
		bits[3] ^ bits[6] ^ bits[7],
		bits[0] ^ bits[1] ^ bits[5] ^ bits[6],
		bits[0] ^ bits[1] ^ bits[2] ^ bits[6] ^ bits[7],
		bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6],
		bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7],
		bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7],
	}
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

// golay23Syndrome computes the 11-bit syndrome of a Golay(23,12,7) error
// pattern: its remainder modulo g(x) = 0xC75.
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

// golay24Syndrome computes the 12-bit syndrome of a Golay(24,12,8) error
// pattern with the parity bit in bit 0 and the Golay(23,12,7) codeword in
// bits 1 to 23.
func golay24Syndrome(errPattern uint32) int {
	err23 := errPattern >> 1
	polySyndrome := golay23Syndrome(err23)
	parityErr := bits.OnesCount32(errPattern) & 1
	return (polySyndrome << 1) | parityErr
}
