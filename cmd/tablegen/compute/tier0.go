package compute

// ---- Tier 0: No dependencies, foundational tables ----

// ComputeAMBEScrambleTable generates the 4096-entry AMBE vocoder scramble table.
//
// Each 12-bit index i (the AMBE 'a' parameter) seeds a 16-bit Linear
// Congruential Generator (LCG) which produces 24 pseudo-random bits.
// These bits form a mask that is XORed with the Golay-encoded 'b' vector
// during AMBE frame encode/decode (after shifting right by 1).
//
// The LCG follows the recurrence:
//
//	p[n] = (173 * p[n-1] + 13849) mod 65536
//
// with seed p[0] = 16 * i. Output bit k (1-indexed) is the MSB of p[k]:
//
//	bit_k = p[k] >> 15
//
// The 24 output bits are packed MSB-first into a uint32.
func ComputeAMBEScrambleTable() [4096]uint32 {
	const (
		lcgMultiplier = 173
		lcgIncrement  = 13849
		lcgModulus    = 65536
		numBits       = 24
	)

	var table [4096]uint32
	for i := range 4096 {
		p := uint32(16 * i) // seed = i << 4
		var entry uint32
		for k := 0; k < numBits; k++ {
			p = (lcgMultiplier*p + lcgIncrement) & (lcgModulus - 1)
			entry = (entry << 1) | (p >> 15) // collect MSB
		}
		table[i] = entry
	}
	return table
}

// ComputeDM computes the BPTC(196,96) deinterleave matrix.
// dm[i] = (i * 181) % 196
func ComputeDM() [256]uint8 {
	var dm [256]uint8
	for i := range 256 {
		dm[i] = uint8((i * 181) % 196) //nolint:gosec // result is always < 196, fits in uint8
	}
	return dm
}

// ComputeHamming15_11Syndrome computes the syndrome-to-bit-position table
// for Hamming(15,11,3).
//
// The parity check matrix H has 4 rows and 15 columns. Each column is a
// 4-bit value. For syndrome s (1..15), the table entry is the column index
// whose H-column equals s. If no column matches (syndrome 0 = no error),
// the entry is -1.
//
// The syndrome equations from bptc_196_96.go are:
//
//	S0 = b0^b1^b2^b3^b5^b7^b8^b11
//	S1 = b1^b2^b3^b4^b6^b8^b9^b12
//	S2 = b2^b3^b4^b5^b7^b9^b10^b13
//	S3 = b0^b1^b2^b4^b6^b7^b10^b14
//
// The 4-bit syndrome is S0 | S1<<1 | S2<<2 | S3<<3.
// For a single-bit error at position p, the syndrome equals column p of H.
func ComputeHamming15_11Syndrome() [16]int {
	// H-matrix columns: for each bit position 0..14, set that bit to 1
	// and compute the syndrome.
	var hCols [15]int
	for p := 0; p < 15; p++ {
		var b [15]int
		b[p] = 1
		s0 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[5] ^ b[7] ^ b[8] ^ b[11]
		s1 := b[1] ^ b[2] ^ b[3] ^ b[4] ^ b[6] ^ b[8] ^ b[9] ^ b[12]
		s2 := b[2] ^ b[3] ^ b[4] ^ b[5] ^ b[7] ^ b[9] ^ b[10] ^ b[13]
		s3 := b[0] ^ b[1] ^ b[2] ^ b[4] ^ b[6] ^ b[7] ^ b[10] ^ b[14]
		hCols[p] = s0 | s1<<1 | s2<<2 | s3<<3
	}

	var table [16]int
	for i := range table {
		table[i] = -1
	}
	for p := 0; p < 15; p++ {
		table[hCols[p]] = p
	}
	return table
}

// ComputeHamming13_9Syndrome computes the syndrome-to-bit-position table
// for the shortened Hamming(13,9,3) code used in BPTC column correction.
//
// Syndrome equations from bptc_196_96.go:
//
//	S0 = b0^b1^b3^b5^b6^b9
//	S1 = b0^b1^b2^b4^b6^b7^b10
//	S2 = b0^b1^b2^b3^b5^b7^b8^b11
//	S3 = b0^b2^b4^b5^b8^b12
func ComputeHamming13_9Syndrome() [16]int {
	var hCols [13]int
	for p := 0; p < 13; p++ {
		var b [13]int
		b[p] = 1
		s0 := b[0] ^ b[1] ^ b[3] ^ b[5] ^ b[6] ^ b[9]
		s1 := b[0] ^ b[1] ^ b[2] ^ b[4] ^ b[6] ^ b[7] ^ b[10]
		s2 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[5] ^ b[7] ^ b[8] ^ b[11]
		s3 := b[0] ^ b[2] ^ b[4] ^ b[5] ^ b[8] ^ b[12]
		hCols[p] = s0 | s1<<1 | s2<<2 | s3<<3
	}

	var table [16]int
	for i := range table {
		table[i] = -1
	}
	for p := 0; p < 13; p++ {
		s := hCols[p]
		if s < 16 {
			table[s] = p
		}
	}
	return table
}

// ComputeCRCCCITT computes the CRC-CCITT lookup table with polynomial 0x1021.
func ComputeCRCCCITT() [256]uint16 {
	var table [256]uint16
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8 //nolint:gosec // i is in [0,255], fits in uint16
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// ComputeConstellationPoints computes the QPSK-like constellation mapping
// from dibit pairs to 4-bit symbol indices, per ETSI TS 102 361-1.
func ComputeConstellationPoints() map[[2]int8]byte {
	// The constellation is defined by the spec. These are the 16 points.
	return map[[2]int8]byte{
		{1, -1}:  0,
		{-1, -1}: 1,
		{3, -3}:  2,
		{-3, -3}: 3,
		{-3, -1}: 4,
		{3, -1}:  5,
		{-1, -3}: 6,
		{1, -3}:  7,
		{-3, 3}:  8,
		{3, 3}:   9,
		{-1, 1}:  10,
		{1, 1}:   11,
		{1, 3}:   12,
		{-1, 3}:  13,
		{3, 1}:   14,
		{-3, 1}:  15,
	}
}

// ComputeReverseConstellation computes the reverse mapping: symbol index → dibit pair.
func ComputeReverseConstellation() [][]int8 {
	fwd := ComputeConstellationPoints()
	rev := make([][]int8, 16)
	for pair, sym := range fwd {
		rev[sym] = pair[:]
	}
	return rev
}

// ComputeVocoderTables computes the AMBE vocoder frame interleaving tables.
// The pattern is stride-4 across 72 bit positions, split into 3 groups (A, B, C).
//
// A: 24 positions starting at offset 0, stride 4, wrapping at 72
// B: 23 positions starting at offset 1 (relative to A's end wrap), stride 4
// C: 25 positions (remaining)
func ComputeVocoderTables() (aTable, bTable, cTable []int) {
	// A-table: start at 0, increment by 4, wrap mod 72
	// Positions: 0, 4, 8, ..., 68, then 1, 5, 9, ..., 21
	// That's 18 positions (0..68 step 4) plus 6 more (1,5,9,13,17,21) = 24 total
	aTable = make([]int, 24)
	pos := 0
	for i := 0; i < 24; i++ {
		aTable[i] = pos
		pos += 4
		if pos >= 72 {
			pos -= 72
			pos++ // shift to next starting column
		}
	}

	// B-table: continues from where A left off
	bTable = make([]int, 23)
	for i := 0; i < 23; i++ {
		bTable[i] = pos
		pos += 4
		if pos >= 72 {
			pos -= 72
			pos++
		}
	}

	// C-table: continues from where B left off
	cTable = make([]int, 25)
	for i := 0; i < 25; i++ {
		cTable[i] = pos
		pos += 4
		if pos >= 72 {
			pos -= 72
			pos++
		}
	}

	return aTable, bTable, cTable
}

// ComputeCRC32Table computes the 256-entry CRC-32 lookup table for
// byte-at-a-time processing with polynomial 0x04C11DB7.
//
// For each byte value i, the table entry is the CRC remainder of i
// processed through the 32-bit shift register. This is the standard
// CRC-32 table construction used for DMR 32-bit CRC (ETSI TS 102 361-1, B.3.9).
func ComputeCRC32Table() [256]uint32 {
	const poly = uint32(0x04C11DB7)
	var table [256]uint32
	for i := 0; i < 256; i++ {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// ComputeHamming743Syndrome computes the syndrome-to-bit-position table
// for Hamming(7,4,3).
//
// The parity check matrix H has 3 rows and 7 columns. Each column is a
// 3-bit value. For syndrome s (1..7), the table entry is the column index
// whose H-column equals s. Syndrome 0 = no error → entry -1.
//
// The syndrome equations from the spec (ETSI TS 102 361-1, B.3.5):
//
//	s₀ = r₀ ⊕ r₁ ⊕ r₂ ⊕ r₄
//	s₁ = r₁ ⊕ r₂ ⊕ r₃ ⊕ r₅
//	s₂ = r₀ ⊕ r₁ ⊕ r₃ ⊕ r₆
//
// The 3-bit syndrome is s₀<<2 | s₁<<1 | s₂.
func ComputeHamming743Syndrome() [8]int {
	var hCols [7]int
	for p := 0; p < 7; p++ {
		var b [7]int
		b[p] = 1
		s0 := b[0] ^ b[1] ^ b[2] ^ b[4]
		s1 := b[1] ^ b[2] ^ b[3] ^ b[5]
		s2 := b[0] ^ b[1] ^ b[3] ^ b[6]
		hCols[p] = s0<<2 | s1<<1 | s2
	}

	var table [8]int
	for i := range table {
		table[i] = -1
	}
	for p := 0; p < 7; p++ {
		table[hCols[p]] = p
	}
	return table
}

// ComputeGaloisTables computes the GF(2^8) exp and log tables
// for Reed-Solomon(12,9,4) with primitive polynomial 0x11D (x^8 + x^4 + x^3 + x^2 + 1).
func ComputeGaloisTables() (exp [256]uint8, log [256]uint8) {
	// exp[i] = α^i in GF(2^8)
	val := uint16(1)
	for i := 0; i < 256; i++ {
		exp[i] = uint8(val) //nolint:gosec // val is always < 256 in GF(2^8)
		// log is the inverse: log[α^i] = i
		if i < 255 {
			log[uint8(val)] = uint8(i) //nolint:gosec // i is in [0,254], fits in uint8
		}
		val <<= 1
		if val&0x100 != 0 {
			val ^= 0x11D
		}
	}
	// log[0] is undefined but left as 0
	// exp[255] should wrap back to exp[0] = 1
	return exp, log
}
