package compute

// ---- Tier 2: Depend on Tier 1 tables ----

// ComputeTrellis34Transition computes the Trellis 3/4 state transition
// look-up table by inverting the encoder state transition table.
//
// Maps [CurrentState][ReceivedSymbol] -> Tribit (0-7), or 0xFF if invalid.
func ComputeTrellis34Transition(encoderStateTransition []byte) [][]byte {
	table := make([][]byte, 8)
	for s := range table {
		table[s] = make([]byte, 16)
		for sym := range table[s] {
			table[s][sym] = 0xFF
		}
	}

	// Invert the encoder mapping:
	// encoder: [state*8 + tribit] -> symbol
	// decoder: [state][symbol] -> tribit
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
