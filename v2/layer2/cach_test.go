package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestCACH_DecodeTACT(t *testing.T) {
	t.Parallel()
	tactData := [4]bit.Bit{1, 0, 0, 1}
	tactCodeword := hamming.Encode743(tactData)

	var cachBits [layer2.CACHBits]bit.Bit
	copy(cachBits[:layer2.TACTBits], tactCodeword[:])

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
	var cachBits [layer2.CACHBits]bit.Bit
	for i := layer2.TACTBits; i < layer2.CACHBits; i++ {
		cachBits[i] = bit.Bit((i - layer2.TACTBits) % 2)
	}

	cach := layer2.DecodeCACH(cachBits)
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		expected := bit.Bit(i % 2)
		if cach.Payload[i] != expected {
			t.Errorf("payload bit %d: expected %d, got %d", i, expected, cach.Payload[i])
		}
	}
}

func TestCACH_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	tactData := [4]bit.Bit{0, 1, 1, 0}
	tactCodeword := hamming.Encode743(tactData)

	var cachBits [layer2.CACHBits]bit.Bit
	copy(cachBits[:layer2.TACTBits], tactCodeword[:])
	for i := layer2.TACTBits; i < layer2.CACHBits; i++ {
		cachBits[i] = 1
	}

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
	tactData := [4]bit.Bit{1, 1, 0, 0}
	tactCodeword := hamming.Encode743(tactData)

	var cachBits [layer2.CACHBits]bit.Bit
	copy(cachBits[:layer2.TACTBits], tactCodeword[:])

	cachBits[2] ^= 1

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
