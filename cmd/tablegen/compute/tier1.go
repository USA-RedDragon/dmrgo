package compute

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
