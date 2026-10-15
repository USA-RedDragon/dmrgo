package golay

import (
	"math/bits"
	"math/rand"
	"testing"
	"time"
)

func TestGolay23127(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // pseudo-random adequate for tests

	for d := 0; d < 4096; d++ {
		data := uint16(d) //nolint:gosec // d limited to 12-bit range

		codeword23 := Encode23127(data)

		decoded, result := Decode23127(codeword23)
		if result.Uncorrectable {
			t.Fatalf("Data 0x%03x: Clean codeword reported as uncorrectable", data)
		}
		if result.ErrorsCorrected != 0 {
			t.Fatalf("Data 0x%03x: Clean codeword reported %d errors", data, result.ErrorsCorrected)
		}
		if decoded != data {
			t.Fatalf("Data 0x%03x: Clean codeword decoded as 0x%03x", data, decoded)
		}

		for k := 0; k < 5; k++ {
			bit1 := uint(rng.Intn(23)) //nolint:gosec // bounded conversion for test use
			cwErr1 := codeword23 ^ (1 << bit1)
			decoded, result = Decode23127(cwErr1)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (1 error): Reported uncorrectable", data)
			case result.ErrorsCorrected != 1:
				t.Errorf("Data 0x%03x (1 error): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (1 error): Decoded as 0x%03x", data, decoded)
			}

			bit2 := uint(rng.Intn(23)) //nolint:gosec // bounded conversion for test use
			for bit2 == bit1 {
				bit2 = uint(rng.Intn(23)) //nolint:gosec // bounded conversion for test use
			}
			cwErr2 := cwErr1 ^ (1 << bit2)
			decoded, result = Decode23127(cwErr2)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (2 errors): Reported uncorrectable", data)
			case result.ErrorsCorrected != 2:
				t.Errorf("Data 0x%03x (2 errors): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (2 errors): Decoded as 0x%03x", data, decoded)
			}

			bit3 := uint(rng.Intn(23)) //nolint:gosec // bounded conversion for test use
			for bit3 == bit1 || bit3 == bit2 {
				bit3 = uint(rng.Intn(23)) //nolint:gosec // bounded conversion for test use
			}
			cwErr3 := cwErr2 ^ (1 << bit3)
			decoded, result = Decode23127(cwErr3)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (3 errors): Reported uncorrectable", data)
			case result.ErrorsCorrected != 3:
				t.Errorf("Data 0x%03x (3 errors): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (3 errors): Decoded as 0x%03x", data, decoded)
			}
		}
	}
}

// Golay (23,12,7) is perfect: each of the 2048 syndromes maps to one error
// pattern of weight 3 or less, so no received word is uncorrectable.
func TestGolay23127IsPerfect(t *testing.T) {
	t.Parallel()

	for s := range 2048 {
		pattern := golay_23_12_7_syndrome_table[s]
		if pattern == 0xFFFFFFFF || bits.OnesCount32(pattern) > 3 || golay23ComputeSyndrome(pattern) != s {
			t.Fatalf("syndrome %#x maps to pattern %#x", s, pattern)
		}
	}
}
