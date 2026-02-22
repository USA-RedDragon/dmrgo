package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestTACT_DecodeEncode_RoundTrip(t *testing.T) {
	t.Parallel()
	// Build a valid TACT codeword: AT=1, TC=0, LCSS=01 (FirstFragmentLC)
	data := [4]bit.Bit{1, 0, 0, 1}
	codeword := hamming.Encode743(data)

	tact, fecResult := pdu.DecodeTACT(codeword)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if !tact.AccessType {
		t.Error("expected AccessType=true")
	}
	if tact.TDMAChannel {
		t.Error("expected TDMAChannel=false")
	}
	if tact.LCSS != enums.FirstFragmentLC {
		t.Errorf("expected LCSS=FirstFragmentLC, got %d", tact.LCSS)
	}

	// Re-encode and verify round-trip
	encoded := pdu.EncodeTACT(&tact)
	if encoded != codeword {
		t.Errorf("round-trip failed: encoded %v != original %v", encoded, codeword)
	}
}

func TestTACT_DecodeEncode_AllCombinations(t *testing.T) {
	t.Parallel()
	// There are only 16 possible 4-bit data values
	for i := 0; i < 16; i++ {
		data := [4]bit.Bit{
			bit.Bit((i >> 3) & 1),
			bit.Bit((i >> 2) & 1),
			bit.Bit((i >> 1) & 1),
			bit.Bit(i & 1),
		}
		codeword := hamming.Encode743(data)

		tact, fecResult := pdu.DecodeTACT(codeword)
		if fecResult.Uncorrectable {
			t.Errorf("data=%d: unexpected uncorrectable error", i)
			continue
		}

		encoded := pdu.EncodeTACT(&tact)
		if encoded != codeword {
			t.Errorf("data=%d: round-trip failed", i)
		}
	}
}

func TestTACT_SingleBitError(t *testing.T) {
	t.Parallel()
	data := [4]bit.Bit{0, 1, 1, 0}
	codeword := hamming.Encode743(data)

	// Flip each bit position and verify correction
	for pos := 0; pos < 7; pos++ {
		corrupted := codeword
		corrupted[pos] ^= 1

		tact, fecResult := pdu.DecodeTACT(corrupted)
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
	var codeword [7]bit.Bit
	_, fecResult := pdu.DecodeTACT(codeword)
	if fecResult.BitsChecked != 7 {
		t.Errorf("expected BitsChecked=7, got %d", fecResult.BitsChecked)
	}
}

func TestTACT_LCSS_Values(t *testing.T) {
	t.Parallel()
	tests := []struct {
		lcss enums.LCSS
		bits [4]bit.Bit
	}{
		{enums.SingleFragmentLCorCSBK, [4]bit.Bit{0, 0, 0, 0}},
		{enums.FirstFragmentLC, [4]bit.Bit{0, 0, 0, 1}},
		{enums.LastFragmentLCorCSBK, [4]bit.Bit{0, 0, 1, 0}},
		{enums.ContinuationFragmentLCorCSBK, [4]bit.Bit{0, 0, 1, 1}},
	}

	for _, tt := range tests {
		codeword := hamming.Encode743(tt.bits)
		tact, fecResult := pdu.DecodeTACT(codeword)
		if fecResult.Uncorrectable {
			t.Errorf("LCSS=%d: unexpected uncorrectable", tt.lcss)
			continue
		}
		if tact.LCSS != tt.lcss {
			t.Errorf("expected LCSS=%d, got %d", tt.lcss, tact.LCSS)
		}
	}
}

func TestTACT_ToString(t *testing.T) {
	t.Parallel()
	data := [4]bit.Bit{1, 1, 0, 0}
	codeword := hamming.Encode743(data)
	tact, _ := pdu.DecodeTACT(codeword)
	s := tact.ToString()
	if s == "" {
		t.Error("ToString should not be empty")
	}
}
