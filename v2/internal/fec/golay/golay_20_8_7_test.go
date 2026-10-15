package golay

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestGolay2087Decode(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		var data [8]bit.Bit
		for j := 0; j < 8; j++ {
			if rand.Intn(2) == 1 {
				data[j] = 1
			}
		}

		parity := parity2087(data)
		var cw [20]bit.Bit
		copy(cw[:], data[:])
		copy(cw[8:], parity[:])

		decoded, result := Decode2087(cw)
		if result.Uncorrectable || result.ErrorsCorrected != 0 {
			t.Errorf("Failed to decode clean codeword")
		}
		if decoded != cw {
			t.Errorf("Decoded clean codeword mismatch")
		}

		cwErr1 := cw
		pos := rand.Intn(20)
		cwErr1[pos] ^= 1
		decoded, result = Decode2087(cwErr1)
		if result.Uncorrectable || result.ErrorsCorrected != 1 {
			t.Errorf("Failed to correct 1 error")
		}
		if decoded != cw {
			t.Errorf("Decoded 1-error codeword mismatch")
		}

		cwErr2 := cw
		p1, p2 := rand.Intn(20), rand.Intn(20)
		for p1 == p2 {
			p2 = rand.Intn(20)
		}
		cwErr2[p1] ^= 1
		cwErr2[p2] ^= 1
		decoded, result = Decode2087(cwErr2)
		if result.Uncorrectable || result.ErrorsCorrected != 2 {
			t.Errorf("Failed to correct 2 errors")
		}
		if decoded != cw {
			t.Errorf("Decoded 2-error codeword mismatch")
		}

		cwErr3 := cw
		p1, p2, p3 := rand.Intn(20), rand.Intn(20), rand.Intn(20)
		for p1 == p2 {
			p2 = rand.Intn(20)
		}
		for p3 == p1 || p3 == p2 {
			p3 = rand.Intn(20)
		}
		cwErr3[p1] ^= 1
		cwErr3[p2] ^= 1
		cwErr3[p3] ^= 1
		decoded, result = Decode2087(cwErr3)
		if result.Uncorrectable || result.ErrorsCorrected != 3 {
			t.Errorf("Failed to correct 3 errors")
		}
		if decoded != cw {
			t.Errorf("Decoded 3-error codeword mismatch")
		}
	}
}
