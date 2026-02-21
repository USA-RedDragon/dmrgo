package hamming

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestHamming743_Parity(t *testing.T) {
	// Test parity equations against known generator matrix
	// Data [1,0,0,0] → parity from first row of P: [1,0,1]
	data := [4]bit.Bit{1, 0, 0, 0}
	p := Parity743(data)
	if p != [3]bit.Bit{1, 0, 1} {
		t.Errorf("Parity([1,0,0,0]) = %v, want [1,0,1]", p)
	}

	// Data [0,1,0,0] → [1,1,1]
	data = [4]bit.Bit{0, 1, 0, 0}
	p = Parity743(data)
	if p != [3]bit.Bit{1, 1, 1} {
		t.Errorf("Parity([0,1,0,0]) = %v, want [1,1,1]", p)
	}

	// Data [0,0,1,0] → [1,1,0]
	data = [4]bit.Bit{0, 0, 1, 0}
	p = Parity743(data)
	if p != [3]bit.Bit{1, 1, 0} {
		t.Errorf("Parity([0,0,1,0]) = %v, want [1,1,0]", p)
	}

	// Data [0,0,0,1] → [0,1,1]
	data = [4]bit.Bit{0, 0, 0, 1}
	p = Parity743(data)
	if p != [3]bit.Bit{0, 1, 1} {
		t.Errorf("Parity([0,0,0,1]) = %v, want [0,1,1]", p)
	}

	// Data [0,0,0,0] → [0,0,0]
	data = [4]bit.Bit{0, 0, 0, 0}
	p = Parity743(data)
	if p != [3]bit.Bit{0, 0, 0} {
		t.Errorf("Parity([0,0,0,0]) = %v, want [0,0,0]", p)
	}
}

func TestHamming743_Check(t *testing.T) {
	// Valid codeword: [1,0,0,0, 1,0,1]
	if !Check743([7]bit.Bit{1, 0, 0, 0, 1, 0, 1}) {
		t.Error("Check743 should accept valid codeword [1,0,0,0, 1,0,1]")
	}

	// Invalid codeword: flip one bit
	if Check743([7]bit.Bit{0, 0, 0, 0, 1, 0, 1}) {
		t.Error("Check743 should reject invalid codeword [0,0,0,0, 1,0,1]")
	}
}

func TestHamming743_Encode(t *testing.T) {
	cw := Encode743([4]bit.Bit{1, 0, 1, 1})
	if !Check743(cw) {
		t.Errorf("Encode743 produced invalid codeword: %v", cw)
	}

	// Verify data bits are preserved
	if cw[0] != 1 || cw[1] != 0 || cw[2] != 1 || cw[3] != 1 {
		t.Errorf("Encode743 data bits mismatch: got %v", cw[:4])
	}
}

func TestHamming743_DecodeNoError(t *testing.T) {
	cw := Encode743([4]bit.Bit{1, 1, 0, 1})
	decoded, result := Decode743(cw)
	if result.ErrorsCorrected != 0 || result.Uncorrectable {
		t.Errorf("Expected no errors, got corrected=%d uncorrectable=%v",
			result.ErrorsCorrected, result.Uncorrectable)
	}
	if decoded != cw {
		t.Errorf("Decode mismatch: got %v, want %v", decoded, cw)
	}
}

func TestHamming743_DecodeSingleError(t *testing.T) {
	// Test correction of a single bit error at every position
	for _, data := range [][4]bit.Bit{
		{0, 0, 0, 0},
		{1, 0, 0, 0},
		{0, 1, 0, 1},
		{1, 1, 1, 0},
		{1, 1, 1, 1},
	} {
		cw := Encode743(data)
		for errPos := 0; errPos < 7; errPos++ {
			corrupted := cw
			corrupted[errPos] ^= 1

			decoded, result := Decode743(corrupted)
			if result.Uncorrectable {
				t.Errorf("data=%v errPos=%d: got uncorrectable", data, errPos)
				continue
			}
			if result.ErrorsCorrected != 1 {
				t.Errorf("data=%v errPos=%d: ErrorsCorrected=%d, want 1",
					data, errPos, result.ErrorsCorrected)
			}
			if decoded != cw {
				t.Errorf("data=%v errPos=%d: decoded=%v, want %v",
					data, errPos, decoded, cw)
			}
		}
	}
}

func TestHamming743_DecodeRandom(t *testing.T) {
	// Fuzz with random data, inject 0 or 1 errors, verify correction
	for i := 0; i < 200; i++ {
		var data [4]bit.Bit
		for j := 0; j < 4; j++ {
			if rand.Intn(2) == 1 { //nolint:gosec // deterministic PRNG sufficient for tests
				data[j] = 1
			}
		}
		cw := Encode743(data)

		// No error
		decoded, result := Decode743(cw)
		if result.ErrorsCorrected != 0 || result.Uncorrectable {
			t.Fatalf("iteration %d no-error: unexpected result %+v", i, result)
		}
		if decoded != cw {
			t.Fatalf("iteration %d no-error: decoded mismatch", i)
		}

		// Single error
		errPos := rand.Intn(7) //nolint:gosec // deterministic PRNG sufficient for tests
		corrupted := cw
		corrupted[errPos] ^= 1
		decoded, result = Decode743(corrupted)
		if result.Uncorrectable {
			t.Fatalf("iteration %d single-error pos %d: got uncorrectable", i, errPos)
		}
		if result.ErrorsCorrected != 1 {
			t.Fatalf("iteration %d single-error pos %d: ErrorsCorrected=%d", i, errPos, result.ErrorsCorrected)
		}
		if decoded != cw {
			t.Fatalf("iteration %d single-error pos %d: decoded mismatch", i, errPos)
		}
	}
}

func TestHamming743_EncodeDecode_AllCodewords(t *testing.T) {
	// Exhaustively test all 16 possible 4-bit data words
	for val := 0; val < 16; val++ {
		data := [4]bit.Bit{
			bit.Bit((val >> 3) & 1),
			bit.Bit((val >> 2) & 1),
			bit.Bit((val >> 1) & 1),
			bit.Bit(val & 1),
		}
		cw := Encode743(data)
		if !Check743(cw) {
			t.Errorf("val=%d: encoded codeword fails check", val)
		}
		decoded, result := Decode743(cw)
		if result.HasErrors() {
			t.Errorf("val=%d: clean decode has errors", val)
		}
		if decoded != cw {
			t.Errorf("val=%d: clean decode mismatch", val)
		}
	}
}

func TestHamming743_BitsChecked(t *testing.T) {
	cw := Encode743([4]bit.Bit{0, 0, 0, 0})
	_, result := Decode743(cw)
	if result.BitsChecked != 7 {
		t.Errorf("BitsChecked = %d, want 7", result.BitsChecked)
	}
}
