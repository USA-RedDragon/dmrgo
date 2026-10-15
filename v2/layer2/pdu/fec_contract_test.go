package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCRCOnlyPDUReportsDetectionOnly(t *testing.T) {
	var zero [96]bit.Bit
	if _, res := pdu.DecodeCSBK(zero, false); res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("CSBK with a bad CRC: %+v, want only Uncorrectable", res)
	}
	if _, res := pdu.DecodeDataHeader(zero); res != (fec.Result{Uncorrectable: true}) {
		t.Errorf("data header with a bad CRC: %+v, want only Uncorrectable", res)
	}

	csbk := pdu.CSBK{CSBKOpcode: 0x3F, RawPayload: [64]bit.Bit{1, 0, 1}}
	bits := must(csbk.Encode())
	if _, res := pdu.DecodeCSBK(bits, false); res != (fec.Result{}) {
		t.Errorf("CSBK with a good CRC: %+v, want a zero result", res)
	}
}
