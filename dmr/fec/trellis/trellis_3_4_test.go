package trellis34

import (
	"math/rand"
	"testing"
)

func TestTrellis34Decode(t *testing.T) {
	tr := New()

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
			// Find pair for symbol
			found := false
			for pair, s := range constellationPoints {
				if s == symbols[i] {
					orderedDibits[i*2] = pair[0]
					orderedDibits[i*2+1] = pair[1]
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("Symbol %d not found in constellation", symbols[i])
			}
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
		decodedBits, errs := tr.Decode(encodedBits)

		// 4. Verify
		// errors should be 0
		if errs != 0 {
			t.Errorf("Expected 0 errors, got %d", errs)
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

		// 5. Test Error Correction?
		// Flip one bit in encodedBits?
		// Deterministic single-symbol error: flip one dibit before conversion to bits.
		corrupt := permutedDibits
		symbolPos := rand.Intn(98) //nolint:gosec // test randomness only
		corrupt[symbolPos] ^= 0b10 // flip first bit of dibit to introduce one symbol error

		corruptBits := tr.dibitsToBits(corrupt)

		baselineDiff := 0
		for i := 0; i < 144; i++ {
			if corruptBits[i] != expectedBits[i] {
				baselineDiff++
			}
		}

		decodedCorrupt, _ := tr.Decode(corruptBits)

		diffCount := 0
		for i := 0; i < 144; i++ {
			if decodedCorrupt[i] != expectedBits[i] {
				diffCount++
			}
		}

		if diffCount > baselineDiff {
			t.Errorf("Decoder increased errors after single symbol flip (before=%d, after=%d)", baselineDiff, diffCount)
		}
	}
}
