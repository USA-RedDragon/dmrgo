package pdu_test

import (
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDecodedEqualsBuilt(t *testing.T) {
	t.Parallel()

	csbk := pdu.CSBK{LastBlock: true}
	if err := csbk.SetPreamble(pdu.PreamblePDU{Data: true, CSBKBlocksToFollow: 3, TargetAddress: 3120101, SourceAddress: 3120001}); err != nil {
		t.Fatal(err)
	}
	gotCSBK := pdu.DecodeCSBK(must(csbk.Encode()))
	gotCSBK.FEC = fec.Result{}
	if !reflect.DeepEqual(gotCSBK, csbk) {
		t.Errorf("CSBK: got %v, want %v", gotCSBK, csbk)
	}

	var slc pdu.ShortLC
	if err := slc.SetActivityUpdate(pdu.ShortLCActivityUpdate{TS1ActivityID: 8}); err != nil {
		t.Fatal(err)
	}
	gotSLC := pdu.DecodeShortLC(must(slc.Encode()))
	gotSLC.FEC = fec.Result{}
	if !reflect.DeepEqual(gotSLC, slc) {
		t.Errorf("ShortLC: got %v, want %v", gotSLC, slc)
	}

	var mbc pdu.MBCHeader
	if err := mbc.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: 9, SourceAddress: 3120001}); err != nil {
		t.Fatal(err)
	}
	gotMBC := pdu.DecodeMBCHeader(must(mbc.Encode()))
	gotMBC.FEC = fec.Result{}
	if !reflect.DeepEqual(gotMBC, mbc) {
		t.Errorf("MBCHeader: got %v, want %v", gotMBC, mbc)
	}

	usbd := pdu.UnifiedSingleBlockData{ServiceType: 1}
	usbd.Payload[3] = 1
	gotUSBD := pdu.DecodeUnifiedSingleBlockData(must(usbd.Encode()))
	gotUSBD.FEC = fec.Result{}
	if !reflect.DeepEqual(gotUSBD, usbd) {
		t.Errorf("USBD: got %v, want %v", gotUSBD, usbd)
	}
}
