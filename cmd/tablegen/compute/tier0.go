package compute

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
