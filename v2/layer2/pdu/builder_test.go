package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBuilderRejectsAndReturnsZero(t *testing.T) {
	t.Parallel()

	h, err := pdu.RawDataHeader{SAP: enums.SAPShortData, LLIDDestination: 1, LLIDSource: 2}.DataHeader()
	if err == nil || h != (pdu.DataHeader{}) {
		t.Fatalf("R_HEAD with Appended Blocks 0: %+v %v", h, err)
	}
	c, err := pdu.PreamblePDU{TargetAddress: 1 << 24}.CSBK()
	if err == nil || c != (pdu.CSBK{}) {
		t.Fatalf("Pre_CSBK with a 25-bit target: %+v %v", c, err)
	}
	lc, err := pdu.FLCTalkerAliasBlock{}.FullLinkControl(enums.FLCOGroupVoiceChannelUser)
	if err == nil || lc != (pdu.FullLinkControl{}) {
		t.Fatalf("talker alias block with FLCO 0: %+v %v", lc, err)
	}
}

func TestBuilderMotorolaVectors(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		vector string
		build  func() (pdu.CSBK, error)
	}{
		{"radio check", motoRadioCheckRequest, pdu.MotorolaRadioCheckPDU{RequesterAddress: 3140598, CheckedAddress: 3191868}.CSBK},
		{"radio check answer", motoRadioCheckAnswer, pdu.MotorolaRadioCheckPDU{Answer: true, RequesterAddress: 3140598, CheckedAddress: 3191868}.CSBK},
	} {
		c, err := tt.build()
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got, err := c.Encode()
		if err != nil || got != motoBytes(t, tt.vector) {
			t.Errorf("%s: %x %v, want %s", tt.name, got, err, tt.vector)
		}
	}
}
