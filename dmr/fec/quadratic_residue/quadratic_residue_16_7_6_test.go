package quadratic_residue1676

import (
	"math/rand"
	"testing"
)

func TestQuadraticResidue1676Decode(t *testing.T) {
	// Test correction capabilities for QR(16,7,6)
	// It should correct up to 2 errors. (d=6, floor((6-1)/2) = 2)

	for i := 0; i < 1000; i++ {
		// 1. Generate random data (7 bits)
		var data [7]byte
		for j := 0; j < 7; j++ {
			if rand.Intn(2) == 1 { //nolint:gosec // pseudo-random adequate for fuzz-style test
				data[j] = 1
			}
		}

		// 2. Calculate Parity
		parity := ParityBits(data)
		var cw [16]byte
		copy(cw[:], data[:])
		copy(cw[7:], parity[:])

		// Case 1: No Error
		decoded, errs, unc := Decode(cw)
		if unc || errs != 0 {
			t.Errorf("Failed to decode clean codeword")
		}
		if decoded != cw {
			t.Errorf("Decoded clean codeword mismatch")
		}

		// Case 2: 1 Error
		cwErr1 := cw
		pos := rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		cwErr1[pos] ^= 1
		decoded, errs, unc = Decode(cwErr1)
		if unc || errs != 1 {
			t.Errorf("Failed to correct 1 error")
		}
		correctedData := decoded
		// Check data part specifically if needed, but checking full codeword is better
		if correctedData != cw {
			t.Errorf("Decoded 1-error codeword mismatch")
		}

		// Case 3: 2 Errors
		cwErr2 := cw
		p1, p2 := rand.Intn(16), rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		for p1 == p2 {
			p2 = rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		}
		cwErr2[p1] ^= 1
		cwErr2[p2] ^= 1
		decoded, errs, unc = Decode(cwErr2)
		if unc || errs != 2 {
			t.Errorf("Failed to correct 2 errors")
		}
		if decoded != cw {
			t.Errorf("Decoded 2-error codeword mismatch")
		}

		// Case 4: 3 Errors (Uncorrectable)
		// Distance is 6. 3 errors might be detectable but not correctable,
		// or might alias to another codeword (decoder failure or decoding error).
		// QR(16,7,6) can detect up to 5 errors?
		// The Decode function returns 'uncorrectable' if syndrome maps to 0xFFFF.
		cwErr3 := cw
		p1, p2, p3 := rand.Intn(16), rand.Intn(16), rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		for p1 == p2 {
			p2 = rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		}
		for p3 == p1 || p3 == p2 {
			p3 = rand.Intn(16) //nolint:gosec // pseudo-random adequate for fuzz-style test
		}
		cwErr3[p1] ^= 1
		cwErr3[p2] ^= 1
		cwErr3[p3] ^= 1
		decoded, _, unc = Decode(cwErr3)
		// It SHOULD be uncorrectable or miscorrected.
		// If it says uncorrectable, good. If it corrects to WRONG codeword, that's life.
		// But it shouldn't say it corrected 3 errors to the RIGHT codeword.
		if !unc && decoded == cw {
			// This is theoretically impossible for d=6 code with minimum weight decoding
			t.Errorf("Appeared to correct 3 errors (impossible d=6)")
		}
	}
}
