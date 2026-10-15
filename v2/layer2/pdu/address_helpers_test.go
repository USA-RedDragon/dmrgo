package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestFLC_SetAddresses(t *testing.T) {
	for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
		lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: flco}
		if !lc.SetAddresses(91, 3191868) {
			t.Fatalf("FLCO %s: SetAddresses failed", flco)
		}
		decoded, _ := pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeVoiceLCHeader)
		if target, source, ok := decoded.Addresses(); !ok || target != 91 || source != 3191868 {
			t.Errorf("FLCO %s: Addresses = %d %d %v", flco, target, source, ok)
		}
	}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo}
	if lc.SetAddresses(1, 2) {
		t.Error("SetAddresses of a GPS Info LC succeeded")
	}
}

func TestDataHeader_Addresses(t *testing.T) {
	dh := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{}}
	if !dh.SetAddresses(3120002, 3120001) {
		t.Fatal("SetAddresses failed")
	}
	decoded, _ := pdu.DecodeDataHeader(must(dh.Encode()))
	if dst, src, ok := decoded.Addresses(); !ok || dst != 3120002 || src != 3120001 {
		t.Errorf("Addresses = %d %d %v", dst, src, ok)
	}
	var empty pdu.DataHeader
	if _, _, ok := empty.Addresses(); ok || empty.SetAddresses(1, 2) {
		t.Error("an empty header has addresses")
	}
}
