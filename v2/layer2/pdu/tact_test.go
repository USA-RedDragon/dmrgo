package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestTACT_DecodeEncode_RoundTrip(t *testing.T) {
	t.Parallel()
	// Build a valid TACT codeword: AT=1, TC=0, LCSS=01 (FirstFragmentLC)
	codeword := [1]byte{hamming.Encode743(0b1001) << 1}

	tact := pdu.DecodeTACT(codeword)
	fecResult := tact.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if !tact.AccessType {
		t.Error("expected AccessType=true")
	}
	if tact.TDMAChannel {
		t.Error("expected TDMAChannel=false")
	}
	if tact.LCSS != enums.LCSSFirstFragment {
		t.Errorf("expected LCSS=FirstFragmentLC, got %d", tact.LCSS)
	}

	// Re-encode and verify round-trip
	encoded := must(tact.Encode())
	if encoded != codeword {
		t.Errorf("round-trip failed: encoded %v != original %v", encoded, codeword)
	}
}

func TestTACT_DecodeEncode_AllCombinations(t *testing.T) {
	t.Parallel()
	// There are only 16 possible 4-bit data values
	for i := 0; i < 16; i++ {
		codeword := [1]byte{hamming.Encode743(uint8(i)) << 1}

		tact := pdu.DecodeTACT(codeword)
		fecResult := tact.FEC
		if fecResult.Uncorrectable {
			t.Errorf("data=%d: unexpected uncorrectable error", i)
			continue
		}

		encoded := must(tact.Encode())
		if encoded != codeword {
			t.Errorf("data=%d: round-trip failed", i)
		}
	}
}

func TestTACT_SingleBitError(t *testing.T) {
	t.Parallel()
	codeword := [1]byte{hamming.Encode743(0b0110) << 1}

	// Flip each bit position and verify correction
	for pos := 0; pos < 7; pos++ {
		corrupted := codeword
		corrupted[0] ^= 0x80 >> pos

		tact := pdu.DecodeTACT(corrupted)
		fecResult := tact.FEC
		if fecResult.Uncorrectable {
			t.Errorf("bit %d: should be correctable", pos)
			continue
		}
		if fecResult.ErrorsCorrected != 1 {
			t.Errorf("bit %d: expected 1 error corrected, got %d", pos, fecResult.ErrorsCorrected)
		}
		// Verify corrected data matches original
		if tact.TDMAChannel != true || tact.AccessType != false {
			t.Errorf("bit %d: corrected data doesn't match original", pos)
		}
	}
}

func TestTACT_BitsChecked(t *testing.T) {
	t.Parallel()
	var codeword [1]byte
	fecResult := pdu.DecodeTACT(codeword).FEC
	if fecResult.BitsChecked != 7 {
		t.Errorf("expected BitsChecked=7, got %d", fecResult.BitsChecked)
	}
}

func TestTACT_LCSS_Values(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lcss enums.LCSS
		data uint8
	}{
		{enums.LCSSSingleFragment, 0b0000},
		{enums.LCSSFirstFragment, 0b0001},
		{enums.LCSSLastFragment, 0b0010},
		{enums.LCSSContinuationFragment, 0b0011},
	}

	for _, tt := range tests {
		codeword := [1]byte{hamming.Encode743(tt.data) << 1}
		tact := pdu.DecodeTACT(codeword)
		fecResult := tact.FEC
		if fecResult.Uncorrectable {
			t.Errorf("LCSS=%d: unexpected uncorrectable", tt.lcss)
			continue
		}
		if tact.LCSS != tt.lcss {
			t.Errorf("expected LCSS=%d, got %d", tt.lcss, tact.LCSS)
		}
	}
}

func TestTACT_String(t *testing.T) {
	t.Parallel()
	codeword := [1]byte{hamming.Encode743(0b1100) << 1}
	tact := pdu.DecodeTACT(codeword)
	s := tact.String()
	if s == "" {
		t.Error("String should not be empty")
	}
}
