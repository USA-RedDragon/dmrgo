package trellis34

import (
	"math/rand"
	"testing"
)

func TestTrellis34Decode(t *testing.T) {
	tr := New()

	symbolToDibits := make(map[byte][2]int8)
	for pair, sym := range constellationPoints {
		symbolToDibits[sym] = pair
	}

	for run := 0; run < 100; run++ {
		// 1. Generate random inputs (49 tribits)
		// We use 49 because the trellis processes 49 steps/symbols to produce 144 bits?
		// Actually, 48 input tribits generate 144 bits.
		// The 49th symbol is just for termination or flushing properties?
		// We'll generate 49 inputs.
		var inputs [49]byte
		for i := 0; i < 49; i++ {
			inputs[i] = byte(rand.Intn(8)) //nolint:gosec // pseudo-random adequate for tests
		}

		// 2. Encoder Model
		// Encode to symbols
		var symbols [49]byte
		state := 0
		for i := 0; i < 49; i++ {
			in := int(inputs[i])
			// encoderStateTransition maps State(row) + Input(col) -> Symbol
			symbol := encoderStateTransition[state*8+in]
			symbols[i] = symbol

			// Next state is the input tribit
			state = in
		}

		// Map symbols -> Dibits (constellation)
		var orderedDibits [98]int8
		for i := 0; i < 49; i++ {
			pair, ok := symbolToDibits[symbols[i]]
			if !ok {
				t.Fatalf("Symbol %d not found in constellation", symbols[i])
			}
			orderedDibits[i*2] = pair[0]
			orderedDibits[i*2+1] = pair[1]
		}

		// Interleave
		// deinterleave logic: out[interleaveMatrix[i]] = in[i] (where out is ordered, in is permuted)
		// We need to produce 'permuted' such that when deinterleaved, it matches 'ordered'.
		// permuted[i] = ordered[interleaveMatrix[i]]
		var permutedDibits [98]int8
		for i := 0; i < 98; i++ {
			permutedDibits[i] = orderedDibits[interleaveMatrix[i]]
		}

		// Dibits To Bits
		// bitsToDibits reverse
		// We can just use the dibits to construct the bit array.
		// bitsToDibits groups 2 bits -> 1 dibit.
		// We need to expand 1 dibit -> 2 bits.
		// This is effectively dibitsToBits (which exists in the class).
		// Wait, t.dibitsToBits(dibits) -> bits.
		// But verify if dibitsToBits matches bitsToDibits logic.
		// dibitsToBits:
		// 3 -> 0,1
		// 1 -> 0,0
		// -1 -> 1,0
		// -3 -> 1,1
		// bitsToDibits:
		// 0,1 -> 3
		// 0,0 -> 1
		// 1,0 -> -1
		// 1,1 -> -3
		// Matches. Use the helper.

		encodedBits := tr.dibitsToBits(permutedDibits)

		// 3. Decode
		decodedBits, result := tr.Decode(encodedBits)

		// 4. Verify
		// errors should be 0
		if result.ErrorsCorrected != 0 {
			t.Errorf("Expected 0 errors, got %d", result.ErrorsCorrected)
		}

		// Check payload.
		// The decoded bits should match the input tribits converted to bits.
		// The decoder output is 144 bytes (bits).
		// tribitsToBits converts the tribits to bits.
		// tribitsToBits takes [49]byte tribits.
		// We should match against what tribitsToBits(inputs) produces.

		expectedBits := tr.tribitsToBits(inputs)

		if decodedBits != expectedBits {
			t.Errorf("Decoded bits do not match expected bits")
			// Optional: Print diff
		}

		// 5. Single-symbol corruption that is guaranteed invalid for current state
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
			corruptPermutedDibits[i] = corruptOrderedDibits[interleaveMatrix[i]]
		}

		corruptBits := tr.dibitsToBits(corruptPermutedDibits)
		_, corruptResult := tr.Decode(corruptBits)

		if corruptResult.ErrorsCorrected == 0 {
			t.Errorf("Expected decoder to flag an error for invalid symbol at %d", corruptIndex)
		}
	}
}
