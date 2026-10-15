package layer2_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// capturedEmbeddedLC is the embedded LC of every superframe in
// testdata/voice.bin and testdata/parrot_kerchunk.bin: UU_V_Ch_Usr from
// 3191868 to 9990.
var capturedEmbeddedLC = []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C} //nolint:gochecknoglobals

func capturedFragments() [4][32]bit.Bit {
	var fr [4][32]bit.Bit
	for i, word := range [4][4]byte{
		{0x00, 0x00, 0x11, 0x09},
		{0x0F, 0x12, 0x96, 0x96},
		{0x09, 0x0C, 0x35, 0x95},
		{0x84, 0xAA, 0x2B, 0xA5},
	} {
		copy(fr[i][:], bit.UnpackBits(word[:]))
	}
	return fr
}

func capturedLC() pdu.FullLinkControl {
	return pdu.FullLinkControl{
		DataType: elements.DataTypeVoiceLCHeader,
		FLCO:     enums.FLCOUnitToUnitVoiceChannelUser,
		UnitToUnit: &pdu.FLCUnitToUnit{
			TargetAddress: 9990,
			SourceAddress: 3191868,
		},
	}
}

func TestEmbeddedLC_DecodeCapture(t *testing.T) {
	lc, res := layer2.DecodeEmbeddedLC(capturedFragments())
	if res.Uncorrectable || res.ErrorsCorrected != 0 || res.BitsChecked != 128 {
		t.Fatalf("FEC = %+v, want a clean 128-bit check", res)
	}
	if got := bit.PackBits(lc[:]); !bytes.Equal(got, capturedEmbeddedLC) {
		t.Errorf("LC % X, want % X", got, capturedEmbeddedLC)
	}

	flc, res := layer2.DecodeEmbeddedFullLinkControl(capturedFragments())
	if res.Uncorrectable || flc.FEC != res {
		t.Fatalf("FEC = %+v, LC FEC = %+v", res, flc.FEC)
	}
	target, source, ok := flc.Addresses()
	if !ok || flc.FLCO != enums.FLCOUnitToUnitVoiceChannelUser || target != 9990 || source != 3191868 {
		t.Errorf("decoded %s", flc.ToString())
	}
	if flc.DataType != elements.DataTypeVoiceLCHeader {
		t.Errorf("DataType = %v", flc.DataType)
	}
}

func TestEmbeddedLC_EncodeCapture(t *testing.T) {
	lc := capturedLC()
	got, err := layer2.EncodeEmbeddedFullLinkControl(&lc)
	if err != nil {
		t.Fatal(err)
	}
	if got != capturedFragments() {
		t.Errorf("fragments differ from the capture")
	}
	var bits [72]bit.Bit
	copy(bits[:], bit.UnpackBits(capturedEmbeddedLC))
	if layer2.EncodeEmbeddedLC(bits) != capturedFragments() {
		t.Errorf("EncodeEmbeddedLC differs from the capture")
	}
}

func TestEmbeddedLC_RoundTrip(t *testing.T) {
	for seed := 0; seed < 64; seed++ {
		var lc [72]bit.Bit
		for i := range lc {
			lc[i] = bit.Bit(((i*31 + seed*17) >> 3 & 1) ^ (seed >> (i % 6) & 1))
		}
		got, res := layer2.DecodeEmbeddedLC(layer2.EncodeEmbeddedLC(lc))
		if res.HasErrors() || got != lc {
			t.Fatalf("seed %d: got %v (%+v), want %v", seed, got, res, lc)
		}
	}
}

// A BPTC codeword whose checksum bits do not match its LC decodes cleanly
// in the BPTC but must be reported as uncorrectable.
func TestEmbeddedLC_ChecksumMismatch(t *testing.T) {
	fr := capturedFragments()
	info, _ := bptc.DecodeEmbeddedLC(fr)
	info[2*11+10] ^= 1
	_, res := layer2.DecodeEmbeddedLC(bptc.EncodeEmbeddedLC(info))
	if !res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Errorf("FEC = %+v, want uncorrectable from the checksum alone", res)
	}
}

func TestEmbeddedLC_EncodeRejectsBadLC(t *testing.T) {
	lc := pdu.FullLinkControl{FLCO: enums.FLCO(64)}
	if _, err := layer2.EncodeEmbeddedFullLinkControl(&lc); err == nil {
		t.Fatal("FLCO 64 should be rejected")
	}
}
