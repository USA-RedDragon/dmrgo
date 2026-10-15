package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCRCOnlyPDUReportsDetectionOnly(t *testing.T) {
	var zero [96]bit.Bit
	if res := pdu.DecodeCSBK(zero).FEC; res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("CSBK with a bad CRC: %+v, want only Uncorrectable", res)
	}
	if res := pdu.DecodeDataHeader(zero).FEC; res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("data header with a bad CRC: %+v, want only Uncorrectable", res)
	}

	csbk := pdu.CSBK{Opcode: 0x3F, RawPayload: [64]bit.Bit{1, 0, 1}}
	bits := must(csbk.Encode())
	if res := pdu.DecodeCSBK(bits).FEC; res != (fec.Result{}) {
		t.Errorf("CSBK with a good CRC: %+v, want a zero result", res)
	}
}

func TestCorrectingPDUsReportCorrectionsInFEC(t *testing.T) {
	st := pdu.SlotType{ColorCode: 3, DataType: enums.DataTypeCSBK}
	stBits := must(st.Encode())
	stBits[2] ^= 1
	emb := pdu.EmbeddedSignalling{ColorCode: 5, LCSS: enums.LCSSFirstFragment}
	embBits := must(emb.Encode())
	embBits[9] ^= 1
	tact := pdu.TACT{AccessType: true, LCSS: enums.LCSSLastFragment}
	tactBits := must(tact.Encode())
	tactBits[6] ^= 1
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, BurstDataType: enums.DataTypeVoiceLCHeader,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 9, SourceAddress: 3120001}}
	lcBits := must(lc.Encode())
	lcBits[30] ^= 1
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
