package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

func TestEmbeddedSignalling_EncodeDecodeRoundTrip(t *testing.T) {
	original := &pdu.EmbeddedSignalling{
		ColorCode:                          9,
		PreemptionAndPowerControlIndicator: true,
		LCSS:                               enums.FirstFragmentLC,
	}

	encoded := original.Encode()
	decoded := pdu.NewEmbeddedSignallingFromBits(encoded)

	if decoded.ColorCode != original.ColorCode {
		t.Errorf("ColorCode = %d, want %d", decoded.ColorCode, original.ColorCode)
	}
	if decoded.PreemptionAndPowerControlIndicator != original.PreemptionAndPowerControlIndicator {
		t.Errorf("PPCI = %v, want %v", decoded.PreemptionAndPowerControlIndicator, original.PreemptionAndPowerControlIndicator)
	}
	if decoded.LCSS != original.LCSS {
		t.Errorf("LCSS = %v, want %v", decoded.LCSS, original.LCSS)
	}
	if decoded.FEC.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean encoded data")
	}
}

func TestEmbeddedSignalling_AllColorCodes(t *testing.T) {
	for cc := 0; cc < 16; cc++ {
		es := &pdu.EmbeddedSignalling{
			ColorCode: cc,
			LCSS:      enums.SingleFragmentLCorCSBK,
		}
		encoded := es.Encode()
		decoded := pdu.NewEmbeddedSignallingFromBits(encoded)
		if decoded.ColorCode != cc {
			t.Errorf("cc=%d: got ColorCode=%d", cc, decoded.ColorCode)
		}
		if decoded.FEC.Uncorrectable {
			t.Errorf("cc=%d: FEC uncorrectable", cc)
		}
	}
}

func TestEmbeddedSignalling_AllLCSS(t *testing.T) {
	lcssValues := []enums.LCSS{
		enums.SingleFragmentLCorCSBK,
		enums.FirstFragmentLC,
		enums.LastFragmentLCorCSBK,
		enums.ContinuationFragmentLCorCSBK,
	}

	for _, lcss := range lcssValues {
		es := &pdu.EmbeddedSignalling{
			ColorCode: 1,
			LCSS:      lcss,
		}
		encoded := es.Encode()
		decoded := pdu.NewEmbeddedSignallingFromBits(encoded)
		if decoded.LCSS != lcss {
			t.Errorf("lcss=%d: got LCSS=%d", lcss, decoded.LCSS)
		}
	}
}

func TestEmbeddedSignalling_ToString(t *testing.T) {
	es := pdu.NewEmbeddedSignallingFromBits([16]bit.Bit{})
	s := es.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestEmbeddedSignalling_PPCI(t *testing.T) {
	for _, ppci := range []bool{false, true} {
		es := &pdu.EmbeddedSignalling{
			ColorCode:                          3,
			PreemptionAndPowerControlIndicator: ppci,
			LCSS:                               enums.SingleFragmentLCorCSBK,
		}
		encoded := es.Encode()
		decoded := pdu.NewEmbeddedSignallingFromBits(encoded)
		if decoded.PreemptionAndPowerControlIndicator != ppci {
			t.Errorf("ppci=%v: got %v", ppci, decoded.PreemptionAndPowerControlIndicator)
		}
	}
}
