package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSetRawDataHeaderRejectsNoAppendedBlocks(t *testing.T) {
	t.Parallel()

	dh := pdu.DataHeader{RawPayload: [10]byte{1, 2, 3}}
	before := dh
	err := dh.SetRawDataHeader(pdu.RawDataHeader{})
	expectEncodeError(t, err, "DataHeader.SetRawDataHeader: the payload does not meet the conditions of AsRawDataHeader")
	if dh != before {
		t.Errorf("SetRawDataHeader changed the header on an error: %v", dh)
	}
}

func TestEncodeAcceptsMatchingPayload(t *testing.T) {
	t.Parallel()

	var csbk pdu.CSBK
	if err := csbk.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{}); err != nil {
		t.Fatal(err)
	}
	got := pdu.DecodeCSBK(must(csbk.Encode()))
	if _, ok := got.AsTalkgroupVoiceGrant(); !ok {
		t.Error("TV_GRANT did not decode as TalkgroupVoiceGrantPDU")
	}

	var dh pdu.DataHeader
	if err := dh.SetRawDataHeader(pdu.RawDataHeader{AppendedBlocks: 3}); err != nil {
		t.Fatal(err)
	}
	gotDH := pdu.DecodeDataHeader(must(dh.Encode()))
	if r, ok := gotDH.AsRawDataHeader(); !ok || r.AppendedBlocks != 3 {
		t.Errorf("R_HEAD round trip = %+v, %v", r, ok)
	}

	lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeTerminatorWithLC}
	if err := lc.SetTalkerAliasBlock(enums.FLCOTalkerAliasBlock2, pdu.FLCTalkerAliasBlock{}); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.Encode(); err != nil {
		t.Errorf("Talker Alias block 2: %v", err)
	}
}
