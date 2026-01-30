package golay

import (
	"math/rand"
	"testing"
	"time"
)

func TestGolay24128(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Test all 4096 codewords
	for d := 0; d < 4096; d++ {
		data := uint16(d)

		// Encode
		codeword := Golay_24_12_8_EncodingTable[d]

		// Case 1: No Error
		decoded, errs, unc := DecodeGolay24128(codeword)
		if unc {
			t.Fatalf("Data 0x%03x: Clean codeword reported as uncorrectable", data)
		}
		if errs != 0 {
			t.Fatalf("Data 0x%03x: Clean codeword reported %d errors", data, errs)
		}
		if decoded != data {
			t.Fatalf("Data 0x%03x: Clean codeword decoded as 0x%03x", data, decoded)
		}

		// Test random errors
		for k := 0; k < 5; k++ {
			// Case 2: 1 Error
			bit1 := uint(rng.Intn(24))
			cwErr1 := codeword ^ (1 << bit1)
			decoded, errs, unc = DecodeGolay24128(cwErr1)
			if unc {
				t.Errorf("Data 0x%03x (1 error): Reported uncorrectable", data)
			} else if errs != 1 {
				t.Errorf("Data 0x%03x (1 error): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (1 error): Decoded as 0x%03x", data, decoded)
			}

			// Case 3: 2 Errors
			bit2 := uint(rng.Intn(24))
			for bit2 == bit1 {
				bit2 = uint(rng.Intn(24))
			}
			cwErr2 := cwErr1 ^ (1 << bit2)
			decoded, errs, unc = DecodeGolay24128(cwErr2)
			if unc {
				t.Errorf("Data 0x%03x (2 errors): Reported uncorrectable", data)
			} else if errs != 2 {
				t.Errorf("Data 0x%03x (2 errors): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (2 errors): Decoded as 0x%03x", data, decoded)
			}

			// Case 4: 3 Errors
			bit3 := uint(rng.Intn(24))
			for bit3 == bit1 || bit3 == bit2 {
				bit3 = uint(rng.Intn(24))
			}
			cwErr3 := cwErr2 ^ (1 << bit3)
			decoded, errs, unc = DecodeGolay24128(cwErr3)
			if unc {
				t.Errorf("Data 0x%03x (3 errors): Reported uncorrectable", data)
			} else if errs != 3 {
				t.Errorf("Data 0x%03x (3 errors): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (3 errors): Decoded as 0x%03x", data, decoded)
			}

			// Case 5: 4 Errors (Should be uncorrectable as d_min=8)
			bit4 := uint(rng.Intn(24))
			for bit4 == bit1 || bit4 == bit2 || bit4 == bit3 {
				bit4 = uint(rng.Intn(24))
			}
			cwErr4 := cwErr3 ^ (1 << bit4)
			_, _, unc = DecodeGolay24128(cwErr4)
			if !unc {
				t.Errorf("Data 0x%03x (4 errors): Expected uncorrectable, got corrected", data)
			}
		}
	}
}
