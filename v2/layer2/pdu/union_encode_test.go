package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSetRawDataHeaderRejectsNoAppendedBlocks(t *testing.T) {
	t.Parallel()

	dh := pdu.DataHeader{RawPayload: [10]byte{1, 2, 3}}
	before := dh
	err := dh.SetRawData(pdu.RawDataHeader{})
	var rangeErr *dmrgo.FieldRangeError
	if !errors.As(err, &rangeErr) || rangeErr.Type != "RawDataHeader" || rangeErr.Field != "AppendedBlocks" || rangeErr.Min != 1 || rangeErr.Max != 63 {
		t.Fatalf("SetRawData AB 0: %v", err)
	}
	if dh != before {
		t.Errorf("SetRawData changed the header on an error: %v", dh)
	}
}

func TestSetTalkerAliasBlockWrongOpcode(t *testing.T) {
	t.Parallel()

	var lc pdu.FullLinkControl
	if err := lc.SetTalkerAliasBlock(enums.FLCOGroupVoiceChannelUser, pdu.FLCTalkerAliasBlock{}); !errors.Is(err, pdu.ErrWrongOpcode) {
		t.Fatalf("SetTalkerAliasBlock with FLCO 0: %v", err)
	}
	if lc != (pdu.FullLinkControl{}) {
		t.Errorf("SetTalkerAliasBlock changed the LC on an error: %v", lc.String())
	}
}

func TestNoFullLCMask(t *testing.T) {
	t.Parallel()

	var lc pdu.FullLinkControl
	if _, err := lc.Encode(); !errors.Is(err, pdu.ErrNoFullLCMask) {
		t.Errorf("Encode with no BurstDataType: %v", err)
	}
	if _, err := (pdu.LCBytes{}).Codeword(enums.DataTypeCSBK); !errors.Is(err, pdu.ErrNoFullLCMask) {
		t.Errorf("Codeword for a CSBK: %v", err)
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
	if err := dh.SetRawData(pdu.RawDataHeader{AppendedBlocks: 3}); err != nil {
		t.Fatal(err)
	}
	gotDH := pdu.DecodeDataHeader(must(dh.Encode()))
	if r, ok := gotDH.AsRawData(); !ok || r.AppendedBlocks != 3 {
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
