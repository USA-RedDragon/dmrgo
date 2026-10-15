package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func lcHeaderFrame(t *testing.T) [33]byte {
	t.Helper()
	lc := pdu.FullLinkControl{
		DataType:   elements.DataTypeVoiceLCHeader,
		FLCO:       enums.FLCOGroupVoiceChannelUser,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120101},
	}
	var lcBytes [12]byte
	lcBits := must(pdu.EncodeFullLinkControl(&lc))
	copy(lcBytes[:], bit.PackBits(lcBits[:]))
	return must(layer2.BuildLCDataBurst(lcBytes, elements.DataTypeVoiceLCHeader, 1))
}

func TestBurstFEC_LCHeaderCountsPayloadBitsOnce(t *testing.T) {
	frame := lcHeaderFrame(t)
	frame[5] ^= 0x10
	frame[30] ^= 0x01

	b := must(layer2.NewBurstFromBytes(frame))
	want := fec.FECResult{BitsChecked: 20 + 196, ErrorsCorrected: 2}
	if got := b.FEC.Aggregate(); got != want {
		t.Errorf("Aggregate %+v, want %+v (FEC %+v)", got, want, b.FEC)
	}
	var ber fec.BERCalculator
	ber.AddBurst(b.FEC)
	if ber.TotalBitsChecked() != 216 || ber.TotalErrors() != 2 {
		t.Errorf("BER totals %d/%d, want 2/216", ber.TotalErrors(), ber.TotalBitsChecked())
	}
}

func TestBurstFEC_CSBKCountsNoCRCBits(t *testing.T) {
	csbk := pdu.CSBK{CSBKOpcode: 0x3F}
	var lcBytes [12]byte
	csbkBits := must(pdu.EncodeCSBK(&csbk))
	copy(lcBytes[:], bit.PackBits(csbkBits[:]))
	frame := must(layer2.BuildLCDataBurst(lcBytes, elements.DataTypeCSBK, 1))

	b := must(layer2.NewBurstFromBytes(frame))
	want := fec.FECResult{BitsChecked: 20 + 196}
	if got := b.FEC.Aggregate(); got != want {
		t.Errorf("Aggregate %+v, want %+v (FEC %+v)", got, want, b.FEC)
	}
}
