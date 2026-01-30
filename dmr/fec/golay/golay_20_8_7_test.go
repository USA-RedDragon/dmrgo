package golay

import (
	"math/rand"
	"testing"
)

func TestGolay2087Decode(t *testing.T) {
	// Test basic correction capabilities
	// 1. No error
	// 2. 1 bit error
	// 3. 2 bit errors
	// 4. 3 bit errors
	// 5. Uncorrectable (4 errors)

	for i := 0; i < 100; i++ {
		// Generate random data
		var data [8]byte
		for j := 0; j < 8; j++ {
			if rand.Intn(2) == 1 { //nolint:gosec // deterministic PRNG sufficient for tests
				data[j] = 1
			}
		}

		// Calculate parity
		parity := Golay_20_8_Parity(data)
		var cw [20]byte
		copy(cw[:], data[:])
		copy(cw[8:], parity[:])

		// Case 1: No Error
		decoded, errs, unc := DecodeGolay2087(cw)
		if unc || errs != 0 {
			t.Errorf("Failed to decode clean codeword")
		}
		if decoded != cw {
			t.Errorf("Decoded clean codeword mismatch")
		}

		// Case 2: 1 Error
		cwErr1 := cw
		pos := rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		cwErr1[pos] ^= 1
		decoded, errs, unc = DecodeGolay2087(cwErr1)
		if unc || errs != 1 {
			t.Errorf("Failed to correct 1 error")
		}
		if decoded != cw {
			t.Errorf("Decoded 1-error codeword mismatch")
		}

		// Case 3: 2 Errors
		cwErr2 := cw
		p1, p2 := rand.Intn(20), rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		for p1 == p2 {
			p2 = rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		}
		cwErr2[p1] ^= 1
		cwErr2[p2] ^= 1
		decoded, errs, unc = DecodeGolay2087(cwErr2)
		if unc || errs != 2 {
			t.Errorf("Failed to correct 2 errors")
		}
		if decoded != cw {
			t.Errorf("Decoded 2-error codeword mismatch")
		}

		// Case 4: 3 Errors
		cwErr3 := cw
		p1, p2, p3 := rand.Intn(20), rand.Intn(20), rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		for p1 == p2 {
			p2 = rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		}
		for p3 == p1 || p3 == p2 {
			p3 = rand.Intn(20) //nolint:gosec // deterministic PRNG sufficient for tests
		}
		cwErr3[p1] ^= 1
		cwErr3[p2] ^= 1
		cwErr3[p3] ^= 1
		decoded, errs, unc = DecodeGolay2087(cwErr3)
		if unc || errs != 3 {
			t.Errorf("Failed to correct 3 errors")
		}
		if decoded != cw {
			t.Errorf("Decoded 3-error codeword mismatch")
		}
	}
}
