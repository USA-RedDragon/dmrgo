package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestDecodeVocoder_AggregatesFrames(t *testing.T) {
	var data [216]bit.Bit
	for i := range 3 {
		var info [49]bit.Bit
		info[i] = 1
		slot := vocoder.EncodeAMBE72(info)
		copy(data[i*72:], slot[:])
	}
	data[5] ^= 1
	data[72+5] ^= 1
	data[144+5] ^= 1

	v, res := pdu.DecodeVocoder(data)
	want := fec.Result{BitsChecked: 141, ErrorsCorrected: 3}
	if res != want {
		t.Errorf("DecodeVocoder result %+v, want %+v", res, want)
	}
	if v.FECResult() != res {
		t.Errorf("Vocoder.FECResult() %+v, want %+v", v.FECResult(), res)
	}
}

func TestCRCOnlyPDUReportsDetectionOnly(t *testing.T) {
	var zero [96]bit.Bit
	if _, res := pdu.DecodeCSBK(zero, false); res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("CSBK with a bad CRC: %+v, want only Uncorrectable", res)
	}
	if _, res := pdu.DecodeDataHeader(zero); res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("data header with a bad CRC: %+v, want only Uncorrectable", res)
	}

	csbk := pdu.CSBK{CSBKOpcode: 0x3F, RawPayload: [64]bit.Bit{1, 0, 1}}
	bits := must(pdu.EncodeCSBK(&csbk))
	if _, res := pdu.DecodeCSBK(bits, false); res != (fec.Result{}) {
		t.Errorf("CSBK with a good CRC: %+v, want a zero result", res)
	}
}
