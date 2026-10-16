package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestEmbeddedSignalling_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	original := &pdu.EmbeddedSignalling{
		ColorCode:                          9,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSFirstFragment,
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeEmbeddedSignalling(encoded)
	fecResult := decoded.FEC

	if decoded.ColorCode != original.ColorCode {
		t.Errorf("ColorCode = %d, want %d", decoded.ColorCode, original.ColorCode)
	}
	if decoded.PreemptionAndPowerControlIndicator != original.PreemptionAndPowerControlIndicator {
		t.Errorf("PPCI = %v, want %v", decoded.PreemptionAndPowerControlIndicator, original.PreemptionAndPowerControlIndicator)
	}
	if decoded.LCSS != original.LCSS {
		t.Errorf("LCSS = %v, want %v", decoded.LCSS, original.LCSS)
	}
	if fecResult.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean encoded data")
	}
}

func TestEmbeddedSignalling_AllColorCodes(t *testing.T) {
	t.Parallel()

	for cc := 0; cc < 16; cc++ {
		es := &pdu.EmbeddedSignalling{
			ColorCode: enums.ColorCode(cc),
			LCSS:      enums.LCSSSingleFragment,
		}
		encoded := must(es.Encode())
		decoded := pdu.DecodeEmbeddedSignalling(encoded)
		fecResult := decoded.FEC
		if decoded.ColorCode != enums.ColorCode(cc) {
			t.Errorf("cc=%d: got ColorCode=%d", cc, decoded.ColorCode)
		}
		if fecResult.Uncorrectable {
			t.Errorf("cc=%d: FEC uncorrectable", cc)
		}
	}
}

func TestEmbeddedSignalling_AllLCSS(t *testing.T) {
	t.Parallel()

	lcssValues := []enums.LCSS{
		enums.LCSSSingleFragment,
		enums.LCSSFirstFragment,
		enums.LCSSLastFragment,
		enums.LCSSContinuationFragment,
	}

	for _, lcss := range lcssValues {
		es := &pdu.EmbeddedSignalling{
			ColorCode: 1,
			LCSS:      lcss,
		}
		encoded := must(es.Encode())
		decoded := pdu.DecodeEmbeddedSignalling(encoded)
		if decoded.LCSS != lcss {
			t.Errorf("lcss=%d: got LCSS=%d", lcss, decoded.LCSS)
		}
	}
}

func TestEmbeddedSignalling_String(t *testing.T) {
	t.Parallel()

	es := pdu.DecodeEmbeddedSignalling([2]byte{})
	s := es.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestEmbeddedSignalling_PPCI(t *testing.T) {
	t.Parallel()

	for _, ppci := range []bool{false, true} {
		es := &pdu.EmbeddedSignalling{
			ColorCode:                          3,
			PreemptionAndPowerControlIndicator: ppci,
			LCSS:                               enums.LCSSSingleFragment,
		}
		encoded := must(es.Encode())
		decoded := pdu.DecodeEmbeddedSignalling(encoded)
		if decoded.PreemptionAndPowerControlIndicator != ppci {
			t.Errorf("ppci=%v: got %v", ppci, decoded.PreemptionAndPowerControlIndicator)
		}
	}
}

func TestEncodeEmbeddedSignalling_FullRoundTrip(t *testing.T) {
	t.Parallel()

	es := pdu.EmbeddedSignalling{
		ColorCode:                          9,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.LCSSFirstFragment,
	}
	encoded := must(es.Encode())
	// Verify the full [2]byte round trip through decode
	decoded := pdu.DecodeEmbeddedSignalling(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean encoded data")
	}
	if decoded.ColorCode != es.ColorCode {
		t.Errorf("ColorCode = %d, want %d", decoded.ColorCode, es.ColorCode)
	}
	if decoded.PreemptionAndPowerControlIndicator != es.PreemptionAndPowerControlIndicator {
		t.Errorf("PPCI = %v, want %v", decoded.PreemptionAndPowerControlIndicator, es.PreemptionAndPowerControlIndicator)
	}
	if decoded.LCSS != es.LCSS {
		t.Errorf("LCSS = %v, want %v", decoded.LCSS, es.LCSS)
	}
}
