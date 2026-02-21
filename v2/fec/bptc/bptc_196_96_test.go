package bptc

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestBPTC19696Decode(t *testing.T) {
	bptcDiff := BPTC19696{}

	for run := 0; run < 100; run++ {
		// 1. Generate Random Data
		var data [96]bit.Bit
		for i := 0; i < 96; i++ {
			data[i] = bit.Bit(rand.Intn(2)) //nolint:gosec // deterministic PRNG fine for tests
		}

		// 2. Encode
		encoded := Encode(data)

		// 3. Decode Clean
		decoded, result := bptcDiff.DeinterleaveDataBits(encoded)

		if result.Uncorrectable {
			t.Errorf("Failed to decode clean codeword: uncorrectable")
		}
		// BPTC decoder runs single pass in current impl?
		// We expect 0 errors corrected if clean.
		// Note: The loop count `errors` is incremented when bit flipped.
		// If clean, 0.
		if result.ErrorsCorrected != 0 {
			t.Errorf("Decoded clean codeword with %d errors", result.ErrorsCorrected)
		}

		if decoded != data {
			t.Errorf("Decoded payload mismatch")
		}

		// 4. Test Single Error
		// Flip a bit in the encoded stream
		// We should pick a bit that maps to Data or Parity.
		// Note: If we flip a bit, DeinterleaveDataBits returns correct data.
		encodedErr := encoded
		pos := rand.Intn(196)   //nolint:gosec // deterministic PRNG fine for tests
		for pos == int(dm[0]) { // dm[0] maps to grid position 0 (unused corner); flipping it is unobservable
			pos = rand.Intn(196) //nolint:gosec // deterministic PRNG fine for tests
		}
		encodedErr[pos] ^= 1

		decoded, result = bptcDiff.DeinterleaveDataBits(encodedErr)

		if result.Uncorrectable {
			t.Errorf("Failed to correct 1 error")
		}

		// Note: `result.ErrorsCorrected` returned by DeinterleaveDataBits is "how many flips it performed".
		// It should be 1.
		if result.ErrorsCorrected != 1 {
			t.Errorf("Expected 1 error corrected, got %d", result.ErrorsCorrected)
		}

		if decoded != data {
			t.Errorf("Decoded 1-error payload mismatch")
		}

		// 5. Test Uncorrectable?
		// BPTC (196,96) has large distance.
		// But decoder might satisfy simple checks.
		// Let's not stress test uncorrectable here, just basic functionality.
	}
}
