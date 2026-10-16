package compute

// Trellis34Transition inverts encoderStateTransition into the rate
// 3/4 trellis decoder table: the tribit for each [state][symbol], or 0xFF
// when the encoder does not send that symbol from that state.
func Trellis34Transition(encoderStateTransition []byte) [][]byte {
	table := make([][]byte, 8)
	for s := range table {
		table[s] = make([]byte, 16)
		for sym := range table[s] {
			table[s][sym] = 0xFF
		}
	}

	for state := 0; state < 8; state++ {
		for tribit := 0; tribit < 8; tribit++ {
			symbol := encoderStateTransition[state*8+tribit]
			if symbol < 16 {
				table[state][symbol] = byte(tribit)
			}
		}
	}

	return table
}

// trellis34DibitBits maps a ±1/±3 dibit symbol to its two transmitted bits
// (ETSI TS 102 361-1 Table 10.3).
func trellis34DibitBits(d int8) byte {
	switch d {
	case 3:
		return 0b01
	case -1:
		return 0b10
	case -3:
		return 0b11
	default:
		return 0b00
	}
}

// Trellis34BranchDibits computes the two transmitted dibits of the rate 3/4
// trellis branch at index state*8+tribit, as the 4-bit value first<<2 |
// second.
func Trellis34BranchDibits(encoderStateTransition []byte, reverseConstellation [][]int8) []byte {
	table := make([]byte, len(encoderStateTransition))
	for i, point := range encoderStateTransition {
		pair := reverseConstellation[point]
		table[i] = trellis34DibitBits(pair[0])<<2 | trellis34DibitBits(pair[1])
	}
	return table
}

// Trellis34BranchMetric computes the Viterbi branch metric of the rate 3/4
// trellis decoder: for each received dibit pair (first<<2 | second) and
// each state, the cost of the two received dibits against the two branch
// dibits of each tribit t, in byte t of a uint64.
//
// The cost of one dibit depends on the number of 4FSK deviation steps
// between the received and the sent dibit, and is about -ln of the
// probability of that error, scaled so that a 1-step error costs 7. The
// probabilities were measured on 1.3 million dibits that an XPR 8400 sent and
// dmrrf received over the air: 1.13e-3 for a given 1-step error, 5.1e-5 for
// a 2-step error and 1.33e-4 for a 3-step error. FM clicks make a 3-step
// error more likely than a 2-step error.
func Trellis34BranchMetric(branchDibits []byte) [][]uint64 {
	stepCost := [4]uint64{0, 7, 10, 9}
	level := [4]int{1, 3, -1, -3}
	cost := func(a, b byte) uint64 {
		d := level[a&3] - level[b&3]
		if d < 0 {
			d = -d
		}
		return stepCost[d/2]
	}
	table := make([][]uint64, 16)
	for rx := range table {
		table[rx] = make([]uint64, 8)
		for s := range table[rx] {
			for t := range 8 {
				b := branchDibits[s*8+t]
				table[rx][s] |= (cost(byte(rx)>>2, b>>2) + cost(byte(rx), b)) << (8 * t)
			}
		}
	}
	return table
}
