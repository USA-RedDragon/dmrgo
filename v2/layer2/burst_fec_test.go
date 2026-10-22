package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func lcHeaderFrame(t *testing.T) [33]byte {
	t.Helper()
	lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}
	mustSet(lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120101}))
	return must(lcBurst(must(lc.Encode()), enums.DataTypeVoiceLCHeader, 1))
}

func TestBurstFEC_LCHeaderCountsPayloadBitsOnce(t *testing.T) {
	t.Parallel()

	frame := lcHeaderFrame(t)
	frame[5] ^= 0x10
	frame[30] ^= 0x01

	b := must(layer2.NewBurstFromBytes(frame))
	want := fec.Result{BitsChecked: 20 + 196, ErrorsCorrected: 2}
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
	t.Parallel()

	csbk := pdu.CSBK{Opcode: 0x3F}
	frame := must(lcBurst(must(csbk.Encode()), enums.DataTypeCSBK, 1))

	b := must(layer2.NewBurstFromBytes(frame))
	want := fec.Result{BitsChecked: 20 + 196}
	if got := b.FEC.Aggregate(); got != want {
		t.Errorf("Aggregate %+v, want %+v (FEC %+v)", got, want, b.FEC)
	}
}
