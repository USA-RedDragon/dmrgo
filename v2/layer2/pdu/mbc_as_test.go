package pdu_test

import (
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestMBCContinuation_As(t *testing.T) {
	t.Parallel()

	parms := layer3.CdefParms{Channel: 0xFFF, TXMHz: 451, TXKHz: 1000, RXMHz: 456, RXKHz: 1000}

	cgap := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: 7, CdefParms: parms}
	cont := pdu.DecodeMBCContinuation(must(cgap.Encode()))
	gotCGAP := cont.AsCGAP()
	res := gotCGAP.FEC
	if res.Uncorrectable {
		t.Error("CGAP CRC fails")
	}
	gotCGAP.FEC = fec.Result{}
	if !reflect.DeepEqual(gotCGAP, cgap) {
		t.Errorf("AsCGAP = %v, want %v", gotCGAP, cgap)
	}

	mvap := pdu.MVAPContinuation{LastBlock: true, Opcode: pdu.CSBKMove, ColorCode: 3, CdefParms: parms}
	cont = pdu.DecodeMBCContinuation(must(mvap.Encode()))
	if got := cont.AsMVAP(); got.FEC.Uncorrectable || got.CdefParms != parms || got.ColorCode != 3 {
		t.Errorf("AsMVAP = %v, %+v", got, res)
	}

	bcap := pdu.BCAPContinuation{LastBlock: true, Opcode: pdu.CSBKBroadcast, CdefParms: parms}
	cont = pdu.DecodeMBCContinuation(must(bcap.Encode()))
	if got := cont.AsBCAP(); got.FEC.Uncorrectable || got.CdefParms != parms {
		t.Errorf("AsBCAP = %v, %+v", got, res)
	}

	vnap := pdu.VNAPContinuation{LastBlock: true, Opcode: pdu.CSBKBroadcast, ColorCode: 5, CdefParms: parms}
	cont = pdu.DecodeMBCContinuation(must(vnap.Encode()))
	if got := cont.AsVNAP(); got.FEC.Uncorrectable || got.CdefParms != parms || got.ColorCode != 5 {
		t.Errorf("AsVNAP = %v, %+v", got, res)
	}

	cont.Data[10] ^= 1
	if res := cont.AsVNAP().FEC; !res.Uncorrectable {
		t.Error("AsVNAP of a corrupted block passes the CRC")
	}
}
