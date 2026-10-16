package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCRCOnlyPDUReportsDetectionOnly(t *testing.T) {
	t.Parallel()

	var zero [12]byte
	if res := pdu.DecodeCSBK(zero).FEC; res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("CSBK with a bad CRC: %+v, want only Uncorrectable", res)
	}
	if res := pdu.DecodeDataHeader(zero).FEC; res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("data header with a bad CRC: %+v, want only Uncorrectable", res)
	}

	csbk := pdu.CSBK{Opcode: 0x3F, RawPayload: [8]byte{0xA0}}
	bits := must(csbk.Encode())
	if res := pdu.DecodeCSBK(bits).FEC; res != (fec.Result{}) {
		t.Errorf("CSBK with a good CRC: %+v, want a zero result", res)
	}
}

func TestCorrectingPDUsReportCorrectionsInFEC(t *testing.T) {
	t.Parallel()

	st := pdu.SlotType{ColorCode: 3, DataType: enums.DataTypeCSBK}
	stBits := must(st.Encode())
	stBits[0] ^= 0x20
	emb := pdu.EmbeddedSignalling{ColorCode: 5, LCSS: enums.LCSSFirstFragment}
	embBits := must(emb.Encode())
	embBits[1] ^= 0x40
	tact := pdu.TACT{AccessType: true, LCSS: enums.LCSSLastFragment}
	tactBits := must(tact.Encode())
	tactBits[0] ^= 0x02
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, BurstDataType: enums.DataTypeVoiceLCHeader,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 9, SourceAddress: 3120001}}
	lcBits := must(lc.Encode())
	lcBits[3] ^= 0x02
	tests := []struct {
		name string
		got  fec.Result
		want fec.Result
	}{
		{"SlotType", pdu.DecodeSlotType(stBits).FEC, fec.Result{BitsChecked: 20, ErrorsCorrected: 1}},
		{"EmbeddedSignalling", pdu.DecodeEmbeddedSignalling(embBits).FEC, fec.Result{BitsChecked: 16, ErrorsCorrected: 1}},
		{"TACT", pdu.DecodeTACT(tactBits).FEC, fec.Result{BitsChecked: 7, ErrorsCorrected: 1}},
		{"FullLinkControl", pdu.DecodeFullLinkControl(lcBits, enums.DataTypeVoiceLCHeader).FEC, fec.Result{BitsChecked: 96, ErrorsCorrected: 1}},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s with one bit error: FEC %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}
