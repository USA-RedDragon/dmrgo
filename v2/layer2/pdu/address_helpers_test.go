package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestFLC_SetAddresses(t *testing.T) {
	t.Parallel()

	for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
		lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}
		noErr(lc.SetGroupVoice(pdu.FLCGroupVoice{}))
		if flco == enums.FLCOUnitToUnitVoiceChannelUser {
			noErr(lc.SetUnitToUnit(pdu.FLCUnitToUnit{}))
		}
		if lc.FLCO != flco {
			t.Fatalf("FLCO %s: Set made FLCO %s", flco, lc.FLCO)
		}
		if lc.SetAddresses(91, 3191868) != nil {
			t.Fatalf("FLCO %s: SetAddresses failed", flco)
		}
		decoded := pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeVoiceLCHeader)
		if target, source, ok := decoded.Addresses(); !ok || target != 91 || source != 3191868 {
			t.Errorf("FLCO %s: Addresses = %d %d %v", flco, target, source, ok)
		}
	}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo}
	if lc.SetAddresses(1, 2) == nil {
		t.Error("SetAddresses of a GPS Info LC succeeded")
	}
	lc = must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	if err := lc.SetAddresses(1<<24, 2); !isRangeError(err, "FullLinkControl", "TargetAddress", 24) {
		t.Errorf("SetAddresses of a 25-bit target: %v", err)
	}
	if p, ok := lc.AsGroupVoice(); !ok || p.GroupAddress != 91 {
		t.Errorf("a failed SetAddresses changed the LC: %+v %v", p, ok)
	}
}

func TestFLC_SetAddressesManufacturerFID(t *testing.T) {
	t.Parallel()

	for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
		lc := pdu.FullLinkControl{BurstDataType: enums.DataTypeTerminatorWithLC, FLCO: flco, FID: 0xC2}
		lc.RawPayload[0] = 1
		if lc.SetAddresses(3120101, 3120001) != nil {
			t.Fatalf("FLCO %s: SetAddresses failed", flco)
		}
		decoded := pdu.DecodeFullLinkControl(must(lc.Encode()), enums.DataTypeTerminatorWithLC)
		res := decoded.FEC
		if res.Uncorrectable || res.ErrorsCorrected != 0 {
			t.Fatalf("FLCO %s: FEC %+v", flco, res)
		}
		_, isGroup := decoded.AsGroupVoice()
		_, isUnit := decoded.AsUnitToUnit()
		if isGroup || isUnit || decoded.RawPayload[0] != 1 {
			t.Errorf("FLCO %s: decoded %v", flco, decoded)
		}
		if target, source, ok := decoded.Addresses(); !ok || target != 3120101 || source != 3120001 {
			t.Errorf("FLCO %s: Addresses = %d %d %v", flco, target, source, ok)
		}
	}
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, FID: 0xC2}
	if lc.SetAddresses(1, 2) == nil {
		t.Error("SetAddresses of a manufacturer FLCO 8 LC succeeded")
	}
}

func TestDataHeader_Addresses(t *testing.T) {
	t.Parallel()

	var dh pdu.DataHeader
	noErr(dh.SetConfirmed(pdu.ConfirmedDataHeader{}))
	if dh.SetAddresses(3120002, 3120001) != nil {
		t.Fatal("SetAddresses failed")
	}
	decoded := pdu.DecodeDataHeader(must(dh.Encode()))
	if dst, src, ok := decoded.Addresses(); !ok || dst != 3120002 || src != 3120001 {
		t.Errorf("Addresses = %d %d %v", dst, src, ok)
	}
	var proprietary pdu.DataHeader
	noErr(proprietary.SetProprietary(pdu.ProprietaryHeader{}))
	if _, _, ok := proprietary.Addresses(); ok || proprietary.SetAddresses(1, 2) == nil {
		t.Error("a proprietary header has addresses")
	}
	unknown := pdu.DataHeader{RawPayload: [10]byte{0x04}}
	if _, _, ok := unknown.Addresses(); ok || unknown.SetAddresses(1, 2) == nil {
		t.Error("a header of an unknown format has addresses")
	}
}
