package golay

import (
	"math/rand"
	"testing"
	"time"
)

func TestGolay23127(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Test all 4096 codewords
	for d := 0; d < 4096; d++ {
		data := uint16(d)

		// Encode
		// The decode function shifts encoding table value right by 1
		codeword24 := Golay_23_12_7_EncodingTable[d]
		codeword23 := codeword24 >> 1

		// Case 1: No Error
		decoded, errs, unc := DecodeGolay23127(codeword23)
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
			bit1 := uint(rng.Intn(23))
			cwErr1 := codeword23 ^ (1 << bit1)
			decoded, errs, unc = DecodeGolay23127(cwErr1)
			if unc {
				t.Errorf("Data 0x%03x (1 error): Reported uncorrectable", data)
			} else if errs != 1 {
				t.Errorf("Data 0x%03x (1 error): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (1 error): Decoded as 0x%03x", data, decoded)
			}

			// Case 3: 2 Errors
			bit2 := uint(rng.Intn(23))
			for bit2 == bit1 {
				bit2 = uint(rng.Intn(23))
			}
			cwErr2 := cwErr1 ^ (1 << bit2)
			decoded, errs, unc = DecodeGolay23127(cwErr2)
			if unc {
				t.Errorf("Data 0x%03x (2 errors): Reported uncorrectable", data)
			} else if errs != 2 {
				t.Errorf("Data 0x%03x (2 errors): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (2 errors): Decoded as 0x%03x", data, decoded)
			}

			// Case 4: 3 Errors
			bit3 := uint(rng.Intn(23))
			for bit3 == bit1 || bit3 == bit2 {
				bit3 = uint(rng.Intn(23))
			}
			cwErr3 := cwErr2 ^ (1 << bit3)
			decoded, errs, unc = DecodeGolay23127(cwErr3)
			if unc {
				t.Errorf("Data 0x%03x (3 errors): Reported uncorrectable", data)
			} else if errs != 3 {
				t.Errorf("Data 0x%03x (3 errors): Reported %d errors", data, errs)
			} else if decoded != data {
				t.Errorf("Data 0x%03x (3 errors): Decoded as 0x%03x", data, decoded)
			}
		}
	}
}
