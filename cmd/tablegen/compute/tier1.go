package compute

import "math/bits"

// ---- Tier 1: Depend on Tier 0 tables or standalone math ----

// ComputeGolay23_12_7Encoding computes the Golay(23,12,7) systematic encoding table.
//
// Generator polynomial: g(x) = x^11 + x^10 + x^6 + x^5 + x^4 + x^2 + 1 = 0xC75
// For each 12-bit data value d:
//
//	codeword = (d << 11) | (d*x^11 mod g(x))
//
// Stored as codeword << 1 (per existing convention in the codebase).
func ComputeGolay23_12_7Encoding() []uint32 {
	const genPoly = 0xC75 // generator polynomial
	table := make([]uint32, 4096)

	for d := 0; d < 4096; d++ {
		// Compute d(x) * x^11 mod g(x) using polynomial long division
		dividend := uint32(d) << 11 //nolint:gosec // d is in [0,4095], fits in uint32
		for i := 11; i >= 0; i-- {
			if dividend&(1<<(i+11)) != 0 {
				dividend ^= uint32(genPoly) << i
			}
		}
		remainder := dividend & 0x7FF             // bottom 11 bits
		codeword := (uint32(d) << 11) | remainder //nolint:gosec // d is in [0,4095], fits in uint32
		table[d] = codeword << 1                  // stored shifted left by 1
	}

	return table
}

// ComputeGolay24_12_8Encoding computes the Golay(24,12,8) systematic encoding table.
// This extends the Golay(23,12,7) code by appending an overall parity bit.
func ComputeGolay24_12_8Encoding() []uint32 {
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
		codeword24 := (codeword23 << 1) | parity           // 24-bit codeword
		table[d] = codeword24
	}

	return table
}

// ComputeQR16_7_6Syndrome computes the Quadratic Residue (16,7,6) syndrome table.
// Maps 9-bit syndrome (0..511) to 16-bit error pattern.
// 0xFFFF indicates uncorrectable.
//
// The code can correct all single-bit errors and all double-bit errors
// (minimum distance 6, t = floor((6-1)/2) = 2).
func ComputeQR16_7_6Syndrome() [512]uint16 {
	var table [512]uint16
	for i := range table {
		table[i] = 0xFFFF
	}

	// Enumerate all error patterns with weight 0, 1, or 2
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
		// weight == 2
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

// qrSyndrome computes the 9-bit syndrome for a QR(16,7,6) error pattern.
// The error pattern has bits 15..9 as data and bits 8..0 as parity.
// (bit 15 = data[0], bit 9 = data[6], bit 8 = parity[0], bit 0 = parity[8])
func qrSyndrome(errPattern uint16) int {
	// Extract 7 data-error bits (bit positions 15..9 of the 16-bit pattern)
	var dataErr [7]byte
	for i := 0; i < 7; i++ {
		if errPattern&(1<<(15-i)) != 0 {
			dataErr[i] = 1
		}
	}

	// Compute parity from data-error using the generator matrix
	calcParity := qrParityBits(dataErr)

	// Extract 9 parity-error bits (bit positions 8..0)
	var parityErr [9]byte
	for i := 0; i < 9; i++ {
		if errPattern&(1<<(8-i)) != 0 {
			parityErr[i] = 1
		}
	}

	// Syndrome = calcParity XOR parityErr, packed into 9-bit integer
	syndrome := 0
	for i := 0; i < 9; i++ {
		if calcParity[i] != parityErr[i] {
			syndrome |= 1 << (8 - i)
		}
	}

	return syndrome
}

// qrParityBits computes the QR(16,7,6) parity bits from 7 data bits.
// Mirrors quadratic_residue_16_7_6.go ParityBits function.
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

// ComputeQR16_7_6Encoding computes the Quadratic Residue (16,7,6) encoding table.
// For each 7-bit data value (0..127), produces the 16-bit codeword.
// Bit layout: bits 15..9 = data[0..6], bits 8..0 = parity[0..8].
func ComputeQR16_7_6Encoding() []uint16 {
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

// ComputeGolay20_8_7Encoding computes the Golay(20,8,7) systematic encoding table.
// For each 8-bit data value, produces the 20-bit codeword as uint32.
// Bit layout: bits 0..7 = data, bits 8..19 = parity.
func ComputeGolay20_8_7Encoding() []uint32 {
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

// ComputeEncoderStateTransition computes the Trellis rate 3/4 encoder
// state transition table.
//
// For each (state, tribit_input) pair, produces a 4-bit constellation symbol.
//
// The encoder is defined by ETSI TS 102 361-1 §B.2.1:
//   - Upper 3 bits of output = reverseBits3((input + shift(state)) % 8)
//   - LSB = ((state >> 1) ^ (state >> 2)) & 1
//   - shift(state) = (state & 1) * 2 + (state & 4)
func ComputeEncoderStateTransition() []byte {
	table := make([]byte, 64) // 8 states × 8 inputs

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

// ComputeInterleaveMatrix computes the Trellis 3/4 rate interleave matrix.
// 98 entries organized into 4 rows of pairs with stride 8.
//
// Row r (0-3): starts at 2*r, groups of 2 consecutive values, stride 8.
// Row 0 has 13 groups (26 entries, covering 0..97)
// Rows 1-3 have 12 groups (24 entries each, covering 2..95)
func ComputeInterleaveMatrix() []byte {
	table := make([]byte, 0, 98)

	// Row 0: base=0, 13 groups
	for g := 0; g < 13; g++ {
		base := g * 8
		table = append(table, byte(base), byte(base+1))
	}

	// Rows 1-3: base=2*r, 12 groups each
	for r := 1; r <= 3; r++ {
		for g := 0; g < 12; g++ {
			base := 2*r + g*8
			table = append(table, byte(base), byte(base+1))
		}
	}

	return table
}

// ComputeGolay20_8_7Syndrome computes the Golay(20,8,7) syndrome table.
// Maps 12-bit syndrome to 20-bit error pattern (uint32).
// 0xFFFFFFFF indicates uncorrectable.
//
// The code can correct up to 3 errors (d_min = 7, t = 3).
func ComputeGolay20_8_7Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	// Enumerate all error patterns with weight 0, 1, 2, or 3 over 20 bits.
	// Weight 0
	table[0] = 0x00000000

	// Weight 1: 20 patterns
	for p := 0; p < 20; p++ {
		errPattern := uint32(1) << p
		syndrome := golay20Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	// Weight 2: C(20,2) = 190 patterns
	for p1 := 0; p1 < 20; p1++ {
		for p2 := p1 + 1; p2 < 20; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay20Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	// Weight 3: C(20,3) = 1140 patterns
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

// golay20Syndrome computes the 12-bit syndrome for a Golay(20,8,7) error pattern.
// Bit layout: bits[0..7] are data, bits[8..19] are parity.
// The syndrome is computed as: calcParity(data_err) XOR parity_err
// with syndrome bit i stored at position i (LSB = parity position 0).
func golay20Syndrome(errPattern uint32) int {
	// Extract 8 data-error bits (bit positions 0..7)
	var dataErr [8]byte
	for i := 0; i < 8; i++ {
		if errPattern&(1<<i) != 0 {
			dataErr[i] = 1
		}
	}

	// Compute parity from data-error using the Golay(20,8) parity function
	calcParity := golay20ParityBits(dataErr)

	// Extract 12 parity-error bits (positions 8..19)
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

// golay20ParityBits computes the 12 parity bits from 8 data bits.
// Mirrors golay_20_8_7.go Golay_20_8_Parity function.
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

// ComputeGolay23_12_7Syndrome computes the Golay(23,12,7) syndrome table.
// Maps 11-bit syndrome (stored in [4096]uint32 for alignment) to 23-bit error pattern.
// 0xFFFFFFFF indicates uncorrectable.
//
// The code can correct up to 3 errors (d_min = 7, t = 3).
func ComputeGolay23_12_7Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	// Weight 0
	table[0] = 0x00000000

	// Weight 1: 23 patterns
	for p := 0; p < 23; p++ {
		errPattern := uint32(1) << p
		syndrome := golay23Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	// Weight 2: C(23,2) = 253 patterns
	for p1 := 0; p1 < 23; p1++ {
		for p2 := p1 + 1; p2 < 23; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay23Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	// Weight 3: C(23,3) = 1771 patterns
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

// golay23Syndrome computes the 11-bit syndrome for a Golay(23,12,7) error pattern.
// The syndrome is the remainder of dividing the error pattern polynomial by
// g(x) = x^11 + x^10 + x^6 + x^5 + x^4 + x^2 + 1 = 0xC75.
func golay23Syndrome(errPattern uint32) int {
	dividend := errPattern
	for i := 11; i >= 0; i-- {
		if dividend&(1<<(i+11)) != 0 {
			dividend ^= uint32(0xC75) << i
		}
	}
	return int(dividend & 0x7FF)
}

// ComputeGolay24_12_8Syndrome computes the Golay(24,12,8) syndrome table.
// Maps 12-bit syndrome to 24-bit error pattern.
// 0xFFFFFFFF indicates uncorrectable.
//
// The code can correct up to 3 errors (d_min = 8, t = 3).
// Syndrome = (polynomial_syndrome << 1) | overall_parity_error.
func ComputeGolay24_12_8Syndrome() [4096]uint32 {
	var table [4096]uint32
	for i := range table {
		table[i] = 0xFFFFFFFF
	}

	// Weight 0
	table[0] = 0x00000000

	// Weight 1: 24 patterns
	for p := 0; p < 24; p++ {
		errPattern := uint32(1) << p
		syndrome := golay24Syndrome(errPattern)
		table[syndrome] = errPattern
	}

	// Weight 2: C(24,2) = 276 patterns
	for p1 := 0; p1 < 24; p1++ {
		for p2 := p1 + 1; p2 < 24; p2++ {
			errPattern := uint32(1)<<p1 | uint32(1)<<p2
			syndrome := golay24Syndrome(errPattern)
			if table[syndrome] == 0xFFFFFFFF {
				table[syndrome] = errPattern
			}
		}
	}

	// Weight 3: C(24,3) = 2024 patterns
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

// golay24Syndrome computes the 12-bit syndrome for a Golay(24,12,8) error pattern.
// The 24-bit codeword has the parity bit at position 0 and the 23-bit Golay(23,12,7)
// codeword in bits 1..23.
// Syndrome = (polynomial_remainder << 1) | parity_error.
func golay24Syndrome(errPattern uint32) int {
	// Extract the 23-bit portion (bits 1..23)
	err23 := errPattern >> 1
	// Compute polynomial syndrome (11 bits)
	polySyndrome := golay23Syndrome(err23)
	// Compute parity syndrome (1 bit): overall parity of the 24-bit error pattern
	parityErr := bits.OnesCount32(errPattern) & 1
	return (polySyndrome << 1) | parityErr
}

// ComputePRNGTable computes the AMBE PRNG table used for voice frame scrambling.
// This table is defined verbatim in ETSI TS 102 361-1 Annex B.1.2.
//
// The table contains 4096 entries of 24-bit pseudo-random values, indexed
// by the 12-bit A-field data of the AMBE voice frame.
//
// Note: The generation algorithm is specified in the ETSI standard appendix.
// This function reproduces the standard table values.
func ComputePRNGTable() []uint32 {
	// The PRNG uses a 23-bit LFSR with polynomial x^23 + x^5 + 1.
	// For each 12-bit seed, the LFSR is initialized and clocked to produce
	// a 23-bit scrambling value, stored left-shifted by 1 (24-bit).
	table := make([]uint32, 4096)

	for seed := 0; seed < 4096; seed++ {
		// Initialize LFSR: place the 12-bit seed into the lower 12 bits of
		// the 23-bit register. Per the spec, bit 0 is forced to 1 to avoid
		// the all-zero trap state.
		lfsr := uint32(seed)<<11 | 1 //nolint:gosec // seed is in [0,4095], fits in uint32

		// Clock the LFSR 23 times to fully mix, producing 23 output bits
		var output uint32
		for i := 0; i < 23; i++ {
			// Feedback: XOR of bit 22 (MSB) and bit 4
			feedback := ((lfsr >> 22) ^ (lfsr >> 4)) & 1
			lfsr = ((lfsr << 1) | feedback) & 0x7FFFFF // keep 23 bits
			output = (output << 1) | ((lfsr >> 22) & 1)
		}

		// Store as 24-bit value (left-shifted by 1)
		table[seed] = output << 1
	}

	return table
}
