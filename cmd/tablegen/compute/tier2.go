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
