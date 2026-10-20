package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestCACH_DecodeTACT(t *testing.T) {
	t.Parallel()
	cachBits := [3]byte{hamming.Encode743(0b1001) << 1}

	cach := layer2.DecodeCACH(cachBits)
	if cach.TACT.FEC.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if !cach.TACT.AccessType {
		t.Error("expected AccessType=true")
	}
	if cach.TACT.TDMAChannel {
		t.Error("expected TDMAChannel=false")
	}
	if cach.TACT.LCSS != enums.LCSSFirstFragment {
		t.Errorf("expected LCSS=FirstFragmentLC, got %d", cach.TACT.LCSS)
	}
}

func TestCACH_PayloadExtraction(t *testing.T) {
	t.Parallel()
	var cachBits [3]byte
	for i := layer2.TACTBits; i < layer2.CACHBits; i++ {
		bits.SetBit(cachBits[:], i, byte((i-layer2.TACTBits)%2))
	}

	cach := layer2.DecodeCACH(cachBits)
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		expected := byte(i % 2)
		if got := bits.Bit(cach.Payload[:], i); got != expected {
			t.Errorf("payload bit %d: expected %d, got %d", i, expected, got)
		}
	}
	if cach.Payload[2]&0x7F != 0 {
		t.Errorf("payload padding bits = %#02x, want 0", cach.Payload[2]&0x7F)
	}
}

func TestCACH_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	cachBits := [3]byte{hamming.Encode743(0b0110) << 1}
	bits.SetField(cachBits[:], layer2.TACTBits, layer2.CACHPayloadBits, 1<<layer2.CACHPayloadBits-1)

	cach := layer2.DecodeCACH(cachBits)
	encoded := must(cach.Encode())

	if encoded != cachBits {
		t.Error("encode/decode round-trip failed")
	}
}

func TestCACH_Constants(t *testing.T) {
	t.Parallel()
	if layer2.CACHBits != 24 {
		t.Errorf("CACHBits should be 24, got %d", layer2.CACHBits)
	}
	if layer2.TACTBits != 7 {
		t.Errorf("TACTBits should be 7, got %d", layer2.TACTBits)
	}
	if layer2.CACHPayloadBits != 17 {
		t.Errorf("CACHPayloadBits should be 17, got %d", layer2.CACHPayloadBits)
	}
	if layer2.TACTBits+layer2.CACHPayloadBits != layer2.CACHBits {
		t.Error("TACT + Payload should equal CACH total")
	}
}

func TestCACH_TACTErrorCorrection(t *testing.T) {
	t.Parallel()
	cachBits := [3]byte{hamming.Encode743(0b1100) << 1}

	cachBits[0] ^= 0x20

	cach := layer2.DecodeCACH(cachBits)
	if cach.TACT.FEC.Uncorrectable {
		t.Fatal("single bit error should be correctable")
	}
	if cach.TACT.FEC.ErrorsCorrected != 1 {
		t.Errorf("expected 1 error corrected, got %d", cach.TACT.FEC.ErrorsCorrected)
	}
	if !cach.TACT.AccessType || !cach.TACT.TDMAChannel {
		t.Error("corrected TACT should have AT=1, TC=1")
	}
}

func TestCACH_EncodeRejectsPayloadPadding(t *testing.T) {
	t.Parallel()
	for i := layer2.CACHPayloadBits; i < 8*len(layer2.CACH{}.Payload); i++ {
		var c layer2.CACH
		bits.SetBit(c.Payload[:], i, 1)
		_, err := c.Encode()
		var fre *dmrgo.FieldRangeError
		if !errors.As(err, &fre) {
			t.Errorf("padding bit %d: err = %v, want *dmrgo.FieldRangeError", i, err)
		}
	}
}
