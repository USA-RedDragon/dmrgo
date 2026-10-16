package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestFLC_SetAddresses(t *testing.T) {
	t.Parallel()

	for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
		lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: flco}
		if lc.SetAddresses(91, 3191868) {
			t.Fatalf("FLCO %s: SetAddresses without a payload field succeeded", flco)
		}
		lc.GroupVoice, lc.UnitToUnit = &pdu.FLCGroupVoice{}, nil
		if flco == enums.FLCOUnitToUnitVoiceChannelUser {
			lc.GroupVoice, lc.UnitToUnit = nil, &pdu.FLCUnitToUnit{}
		}
		if !lc.SetAddresses(91, 3191868) {
			t.Fatalf("FLCO %s: SetAddresses failed", flco)
		}
		decoded := pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeVoiceLCHeader)
		if target, source, ok := decoded.Addresses(); !ok || target != 91 || source != 3191868 {
			t.Errorf("FLCO %s: Addresses = %d %d %v", flco, target, source, ok)
		}
	}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo}
	if lc.SetAddresses(1, 2) {
		t.Error("SetAddresses of a GPS Info LC succeeded")
	}
	lc = pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser}
	if lc.SetAddresses(1<<24, 2) || lc.GroupVoice != nil {
		t.Error("SetAddresses of a 25-bit target succeeded")
	}
}

func TestFLC_SetAddressesManufacturerFID(t *testing.T) {
	t.Parallel()

	for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
		lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeTerminatorWithLC, FLCO: flco, FID: 0xC2}
		lc.RawPayload[0] = 1
		if !lc.SetAddresses(3120101, 3120001) {
			t.Fatalf("FLCO %s: SetAddresses failed", flco)
		}
		decoded := pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeTerminatorWithLC)
		res := decoded.FEC
		if res.Uncorrectable || res.ErrorsCorrected != 0 {
			t.Fatalf("FLCO %s: FEC %+v", flco, res)
		}
		if decoded.GroupVoice != nil || decoded.UnitToUnit != nil || decoded.RawPayload[0] != 1 {
			t.Errorf("FLCO %s: decoded %v", flco, decoded)
		}
		if target, source, ok := decoded.Addresses(); !ok || target != 3120101 || source != 3120001 {
			t.Errorf("FLCO %s: Addresses = %d %d %v", flco, target, source, ok)
		}
	}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, FID: 0xC2}
	if lc.SetAddresses(1, 2) {
		t.Error("SetAddresses of a manufacturer FLCO 8 LC succeeded")
	}
}

func TestDataHeader_Addresses(t *testing.T) {
	t.Parallel()

	dh := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{}}
	if !dh.SetAddresses(3120002, 3120001) {
		t.Fatal("SetAddresses failed")
	}
	decoded := pdu.DecodeDataHeader(must(dh.Encode()))
	if dst, src, ok := decoded.Addresses(); !ok || dst != 3120002 || src != 3120001 {
		t.Errorf("Addresses = %d %d %v", dst, src, ok)
	}
	var empty pdu.DataHeader
	if _, _, ok := empty.Addresses(); ok || empty.SetAddresses(1, 2) {
		t.Error("an empty header has addresses")
	}
}
