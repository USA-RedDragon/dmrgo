package pdu_test

import (
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TestDecodedEqualsBuilt checks that a decoded PDU, with its FEC result
// cleared, is reflect.DeepEqual to the value it was built from.
func TestDecodedEqualsBuilt(t *testing.T) {
	t.Parallel()

	csbk := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPreamble,
		PreamblePDU: &pdu.PreamblePDU{Data: true, CSBKBlocksToFollow: 3, TargetAddress: 3120101, SourceAddress: 3120001}}
	gotCSBK := pdu.DecodeCSBK(must(csbk.Encode()))
	gotCSBK.FEC = fec.Result{}
	if !reflect.DeepEqual(gotCSBK, csbk) {
		t.Errorf("CSBK: got %v, want %v", gotCSBK, csbk)
	}

	slc := pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &pdu.ShortLCActivityUpdate{TS1ActivityID: 8}}
	gotSLC := pdu.DecodeShortLC(must(slc.Encode()))
	gotSLC.FEC = fec.Result{}
	if !reflect.DeepEqual(gotSLC, slc) {
		t.Errorf("ShortLC: got %v, want %v", gotSLC, slc)
	}

	mbc := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: 9, SourceAddress: 3120001}}
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
