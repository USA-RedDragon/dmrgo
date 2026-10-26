package compute

// DM computes the BPTC(196,96) deinterleave matrix. Entry i is
// (i * 181) mod 196.
func DM() [256]uint8 {
	var dm [256]uint8
	for i := range 256 {
		dm[i] = uint8((i * 181) % 196)
	}
	return dm
}

// Hamming15_11Syndrome computes the bit position of a single-bit
// error for each 4-bit syndrome of the Hamming(15,11,3) code. A syndrome that
// matches no bit maps to -1 (ETSI TS 102 361-1 clause B.3.4).
func Hamming15_11Syndrome() [16]int {
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

// Hamming13_9Syndrome computes the bit position of a single-bit
// error for each 4-bit syndrome of the shortened Hamming(13,9,3) code that
// corrects the BPTC(196,96) columns. A syndrome that matches no bit maps to -1
// (ETSI TS 102 361-1 clause B.3.4).
func Hamming13_9Syndrome() [16]int {
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

// CRC7Table computes the byte-at-a-time CRC-7 lookup table for polynomial
// x⁷ + x⁵ + x² + x + 1 (ETSI TS 102 361-1 clause B.3.13), for a register
// that holds the 7 CRC bits in its top bits.
func CRC7Table() [256]uint8 {
	const poly = 0x27 << 1
	var table [256]uint8
	for i := 0; i < 256; i++ {
		crc := uint8(i)
		for j := 0; j < 8; j++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// CRC8Table computes the byte-at-a-time CRC-8 lookup table for polynomial
// x⁸ + x² + x + 1 (ETSI TS 102 361-1 clause B.3.7).
func CRC8Table() [256]uint8 {
	var table [256]uint8
	for i := 0; i < 256; i++ {
		crc := uint8(i)
		for j := 0; j < 8; j++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ 0x07
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// CRC9Table computes the byte-at-a-time CRC-9 lookup table for polynomial
// x⁹ + x⁶ + x⁴ + x³ + 1 (ETSI TS 102 361-1 clause B.3.10), for a register
// that holds the 9 CRC bits in its top bits.
func CRC9Table() [256]uint16 {
	const poly = 0x59 << 7
	var table [256]uint16
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

// ConstellationPoints computes the 4-bit rate 3/4 trellis symbol
// index of each pair of dibit symbol values (ETSI TS 102 361-1 clause B.2.4).
func ConstellationPoints() map[[2]int8]byte {
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

// ReverseConstellation computes the dibit pair of each symbol index.
func ReverseConstellation() [][]int8 {
	fwd := ConstellationPoints()
	rev := make([][]int8, 16)
	for pair, sym := range fwd {
		rev[sym] = pair[:]
	}
	return rev
}

// Hamming16_11Syndrome computes the bit position of a single-bit
// error for each 5-bit syndrome of the Hamming(16,11,4) code of the embedded
// signalling BPTC. A syndrome that matches no bit maps to -1 (ETSI TS 102
// 361-1 clause B.3.4, Table B.16).
func Hamming16_11Syndrome() [32]int {
	var hCols [16]int
	for p := 0; p < 16; p++ {
		var b [16]int
		b[p] = 1
		s0 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[5] ^ b[7] ^ b[8] ^ b[11]
		s1 := b[1] ^ b[2] ^ b[3] ^ b[4] ^ b[6] ^ b[8] ^ b[9] ^ b[12]
		s2 := b[2] ^ b[3] ^ b[4] ^ b[5] ^ b[7] ^ b[9] ^ b[10] ^ b[13]
		s3 := b[0] ^ b[1] ^ b[2] ^ b[4] ^ b[6] ^ b[7] ^ b[10] ^ b[14]
		s4 := b[0] ^ b[2] ^ b[5] ^ b[6] ^ b[8] ^ b[9] ^ b[10] ^ b[15]
		hCols[p] = s0 | s1<<1 | s2<<2 | s3<<3 | s4<<4
	}

	var table [32]int
	for i := range table {
		table[i] = -1
	}
	for p := 0; p < 16; p++ {
		table[hCols[p]] = p
	}
	return table
}

// SingleBurstBPTCInterleave computes the encode matrix index of each
// transmit index of the single burst BPTC, where the transmit index is the
// matrix index times 17 mod 32 (ETSI TS 102 361-1 clause B.2.2).
func SingleBurstBPTCInterleave() [32]uint8 {
	var table [32]uint8
	for i := 0; i < 32; i++ {
		table[(i*17)%32] = uint8(i)
	}
	return table
}

// CRC32Table computes the byte-at-a-time CRC-32 lookup table for
// polynomial 0x04C11DB7 (ETSI TS 102 361-1 clause B.3.9).
func CRC32Table() [256]uint32 {
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

// Hamming743Syndrome computes the bit position of a single-bit error
// for each 3-bit syndrome s0<<2 | s1<<1 | s2 of the Hamming(7,4,3) code. A
// syndrome that matches no bit maps to -1 (ETSI TS 102 361-1 clause B.3.5).
func Hamming743Syndrome() [8]int {
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

// Hamming17_12Syndrome computes the bit position of a single-bit
// error for each 5-bit syndrome of the Hamming(17,12,3) code with generator
// x^5 + x^2 + 1. A syndrome that matches no bit maps to -1 (ETSI TS 102
// 361-1 clause B.3.3, Table B.13).
func Hamming17_12Syndrome() [32]int {
	var hCols [17]int
	for p := 0; p < 17; p++ {
		var b [17]int
		b[p] = 1
		s0 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[6] ^ b[7] ^ b[9] ^ b[12]
		s1 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[4] ^ b[7] ^ b[8] ^ b[10] ^ b[13]
		s2 := b[1] ^ b[2] ^ b[3] ^ b[4] ^ b[5] ^ b[8] ^ b[9] ^ b[11] ^ b[14]
		s3 := b[0] ^ b[1] ^ b[4] ^ b[5] ^ b[7] ^ b[10] ^ b[15]
		s4 := b[0] ^ b[1] ^ b[2] ^ b[5] ^ b[6] ^ b[8] ^ b[11] ^ b[16]
		hCols[p] = s0 | s1<<1 | s2<<2 | s3<<3 | s4<<4
	}

	var table [32]int
	for i := range table {
		table[i] = -1
	}
	for p := 0; p < 17; p++ {
		table[hCols[p]] = p
	}
	return table
}

// GaloisTables computes the GF(2^8) exponent and logarithm tables for
// Reed-Solomon(12,9,4) with primitive polynomial 0x11D (ETSI TS 102 361-1
// clause B.3.6).
func GaloisTables() (exp [256]uint8, log [256]uint8) {
	val := uint16(1)
	for i := 0; i < 256; i++ {
		exp[i] = uint8(val)
		if i < 255 {
			log[uint8(val)] = uint8(i)
		}
		val <<= 1
		if val&0x100 != 0 {
			val ^= 0x11D
		}
	}
	return exp, log
}
