package pdu_test

import (
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestMBCContinuation_As(t *testing.T) {
	parms := layer3.CdefParms{Channel: 0xFFF, TXMHz: 451, TXKHz: 1000, RXMHz: 456, RXKHz: 1000}

	cgap := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: 7, CdefParms: parms}
	cont, _ := pdu.DecodeMBCContinuation(must(cgap.Encode()))
	gotCGAP, res := cont.AsCGAP()
	if res.Uncorrectable {
		t.Error("CGAP CRC fails")
	}
	gotCGAP.FEC = fec.Result{}
	if !reflect.DeepEqual(gotCGAP, cgap) {
		t.Errorf("AsCGAP = %v, want %v", gotCGAP, cgap)
	}

	mvap := pdu.MVAPContinuation{LastBlock: true, Opcode: pdu.CSBKMove, ColorCode: 3, CdefParms: parms}
	cont, _ = pdu.DecodeMBCContinuation(must(mvap.Encode()))
	if got, res := cont.AsMVAP(); res.Uncorrectable || got.CdefParms != parms || got.ColorCode != 3 {
		t.Errorf("AsMVAP = %v, %+v", got, res)
	}

	bcap := pdu.BCAPContinuation{LastBlock: true, Opcode: pdu.CSBKBroadcast, CdefParms: parms}
	cont, _ = pdu.DecodeMBCContinuation(must(bcap.Encode()))
	if got, res := cont.AsBCAP(); res.Uncorrectable || got.CdefParms != parms {
		t.Errorf("AsBCAP = %v, %+v", got, res)
	}

	vnap := pdu.VNAPContinuation{LastBlock: true, Opcode: pdu.CSBKBroadcast, ColorCode: 5, CdefParms: parms}
	cont, _ = pdu.DecodeMBCContinuation(must(vnap.Encode()))
	if got, res := cont.AsVNAP(); res.Uncorrectable || got.CdefParms != parms || got.ColorCode != 5 {
		t.Errorf("AsVNAP = %v, %+v", got, res)
	}

	cont.Data[10] ^= 1
	if _, res := cont.AsVNAP(); !res.Uncorrectable {
		t.Error("AsVNAP of a corrupted block passes the CRC")
	}
}
