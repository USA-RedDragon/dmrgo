package compute

import "math/bits"

// PRFillBits returns the 96 pseudo-random fill bits of an idle burst, most
// significant bit of each octet first (ETSI TS 102 361-1 Table D.2).
func PRFillBits() [96]uint8 {
	packed := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	var out [96]uint8
	for i := range out {
		out[i] = packed[i/8] >> (7 - i%8) & 1
	}
	return out
}

// ColorCodeErrorValues returns the values above 15 that NewColorCode
// refuses, in increasing order (ETSI TS 102 361-1 clause 9.3.1).
func ColorCodeErrorValues() [240]uint8 {
	var out [240]uint8
	for i := range out {
		out[i] = uint8(i + 16)
	}
	return out
}

// EmbeddedLCPatterns returns the 16-bit row error patterns of weight up to 4
// of the embedded LC BPTC, grouped by Hamming(16,11,4) syndrome and weight,
// with bit c for column c. The patterns of syndrome s and weight w are
// patterns[start[s*5+w]:start[s*5+w+1]], in increasing order. minWeight is
// the least weight of each syndrome, or -1 when no pattern has it.
func EmbeddedLCPatterns() (patterns []uint16, start [161]int, minWeight [32]int) {
	var columns [16]int
	for col := range columns {
		var b [16]int
		b[col] = 1
		s0 := b[0] ^ b[1] ^ b[2] ^ b[3] ^ b[5] ^ b[7] ^ b[8] ^ b[11]
		s1 := b[1] ^ b[2] ^ b[3] ^ b[4] ^ b[6] ^ b[8] ^ b[9] ^ b[12]
		s2 := b[2] ^ b[3] ^ b[4] ^ b[5] ^ b[7] ^ b[9] ^ b[10] ^ b[13]
		s3 := b[0] ^ b[1] ^ b[2] ^ b[4] ^ b[6] ^ b[7] ^ b[10] ^ b[14]
		s4 := b[0] ^ b[2] ^ b[5] ^ b[6] ^ b[8] ^ b[9] ^ b[10] ^ b[15]
		columns[col] = s0 | s1<<1 | s2<<2 | s3<<3 | s4<<4
	}
	var groups [32][5][]uint16
	for s := range minWeight {
		minWeight[s] = -1
	}
	for m := range 1 << 16 {
		p := uint16(m & 0xFFFF)
		w := bits.OnesCount16(p)
		if w > 4 {
			continue
		}
		syndrome := 0
		for col := range columns {
			if p>>col&1 == 1 {
				syndrome ^= columns[col]
			}
		}
		groups[syndrome][w] = append(groups[syndrome][w], p)
		if minWeight[syndrome] < 0 || w < minWeight[syndrome] {
			minWeight[syndrome] = w
		}
	}
	for s := range groups {
		for w := range groups[s] {
			start[s*5+w] = len(patterns)
			patterns = append(patterns, groups[s][w]...)
		}
	}
	start[160] = len(patterns)
	return patterns, start, minWeight
}
