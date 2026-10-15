package trellis

import (
	"math/rand"
	"testing"
)

func TestTrellis34Decode(t *testing.T) {
	t.Parallel()

	symbolToDibits := make(map[byte][2]int8)
	for pair, sym := range trellis34_constellation {
		symbolToDibits[sym] = pair
	}

	for run := 0; run < 100; run++ {
		// 48 random data tribits and the 000 tail tribit (ETSI TS 102 361-1
		// clause B.2.4).
		var inputs [49]byte
		for i := 0; i < 48; i++ {
			inputs[i] = byte(rand.Intn(8))
		}

		var symbols [49]byte
		state := 0
		for i := 0; i < 49; i++ {
			in := int(inputs[i])
			symbol := encoder_state_transition_table[state*8+in]
			symbols[i] = symbol

			state = in
		}

		var orderedDibits [98]int8
		for i := 0; i < 49; i++ {
			pair, ok := symbolToDibits[symbols[i]]
			if !ok {
				t.Fatalf("Symbol %d not found in constellation", symbols[i])
			}
			orderedDibits[i*2] = pair[0]
			orderedDibits[i*2+1] = pair[1]
		}

		var permutedDibits [98]int8
		for i := 0; i < 98; i++ {
			permutedDibits[i] = orderedDibits[interleave_matrix[i]]
		}

		encodedBits := dibitsToBits(permutedDibits)

		decodedBits, result := Decode(encodedBits)

		if result.ErrorsCorrected != 0 {
			t.Errorf("Expected 0 errors, got %d", result.ErrorsCorrected)
		}

		expectedBits := tribitsToBits(inputs)

		if decodedBits != expectedBits {
			t.Errorf("Decoded bits do not match expected bits")
		}

		// Corrupt one symbol so that it is invalid for the current state.
		corruptSymbols := symbols
		corruptIndex := 10 % len(corruptSymbols)
		prevState := byte(0)
		if corruptIndex > 0 {
			prevState = inputs[corruptIndex-1]
		}

		invalidSymbol := byte(0xFF)
		for sym := byte(0); sym < 16; sym++ {
			if trellis34_transition_table[prevState][sym] == 0xFF {
				invalidSymbol = sym
				break
			}
		}
		if invalidSymbol == 0xFF {
			t.Fatalf("Failed to find invalid symbol for state %d", prevState)
		}

		corruptSymbols[corruptIndex] = invalidSymbol

		var corruptOrderedDibits [98]int8
		for i := 0; i < 49; i++ {
			pair, ok := symbolToDibits[corruptSymbols[i]]
			if !ok {
				t.Fatalf("Symbol %d not found in constellation (corrupt path)", corruptSymbols[i])
			}
			corruptOrderedDibits[i*2] = pair[0]
			corruptOrderedDibits[i*2+1] = pair[1]
		}

		var corruptPermutedDibits [98]int8
		for i := 0; i < 98; i++ {
			corruptPermutedDibits[i] = corruptOrderedDibits[interleave_matrix[i]]
		}

		corruptBits := dibitsToBits(corruptPermutedDibits)
		_, corruptResult := Decode(corruptBits)

		if corruptResult.ErrorsCorrected == 0 {
			t.Errorf("Expected decoder to flag an error for invalid symbol at %d", corruptIndex)
		}
	}
}
