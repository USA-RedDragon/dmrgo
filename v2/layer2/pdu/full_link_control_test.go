package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestFullLinkControl_GroupVoice_EncodeDecodeCycle(t *testing.T) {
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: layer3.ServiceOptions{
				IsEmergency:         false,
				IsPrivacy:           false,
				IsBroadcast:         false,
				IsOpenVoiceCallMode: false,
				PriorityLevel:       0,
			},
			GroupAddress:  9990,
			SourceAddress: 3120101,
		},
	}

	infoBits := must(original.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOGroupVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOGroupVoiceChannelUser", decoded.FLCO)
	}
	if decoded.ProtectFlag != original.ProtectFlag {
		t.Errorf("ProtectFlag = %v, want %v", decoded.ProtectFlag, original.ProtectFlag)
	}
	if decoded.FID != original.FID {
		t.Errorf("FeatureSetID = %v, want %v", decoded.FID, original.FID)
	}
	if decoded.GroupVoice == nil {
		t.Fatal("GroupVoice is nil")
	}
	if decoded.GroupVoice.GroupAddress != original.GroupVoice.GroupAddress {
		t.Errorf("GroupAddress = %d, want %d", decoded.GroupVoice.GroupAddress, original.GroupVoice.GroupAddress)
	}
	if decoded.GroupVoice.SourceAddress != original.GroupVoice.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", decoded.GroupVoice.SourceAddress, original.GroupVoice.SourceAddress)
	}
}

func TestFullLinkControl_UnitToUnit_EncodeDecodeCycle(t *testing.T) {
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   true,
		FLCO:          enums.FLCOUnitToUnitVoiceChannelUser,
		FID:           enums.FIDStandardized,
		UnitToUnit: &pdu.FLCUnitToUnit{
			ServiceOptions: layer3.ServiceOptions{
				IsEmergency:         true,
				IsPrivacy:           false,
				IsBroadcast:         false,
				IsOpenVoiceCallMode: false,
				PriorityLevel:       2,
			},
			TargetAddress: 1234567,
			SourceAddress: 7654321,
		},
	}

	infoBits := must(original.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOUnitToUnitVoiceChannelUser", decoded.FLCO)
	}
	if decoded.ProtectFlag != true {
		t.Errorf("ProtectFlag = %v, want true", decoded.ProtectFlag)
	}
	if decoded.UnitToUnit == nil {
		t.Fatal("UnitToUnit is nil")
	}
	if decoded.UnitToUnit.TargetAddress != original.UnitToUnit.TargetAddress {
		t.Errorf("TargetAddress = %d, want %d", decoded.UnitToUnit.TargetAddress, original.UnitToUnit.TargetAddress)
	}
	if decoded.UnitToUnit.SourceAddress != original.UnitToUnit.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", decoded.UnitToUnit.SourceAddress, original.UnitToUnit.SourceAddress)
	}
	if decoded.UnitToUnit.ServiceOptions.IsEmergency != true {
		t.Error("ServiceOptions.IsEmergency should be true")
	}
	if decoded.UnitToUnit.ServiceOptions.PriorityLevel != 2 {
		t.Errorf("ServiceOptions.PriorityLevel = %d, want 2", decoded.UnitToUnit.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_EncodeStability(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: layer3.ServiceOptions{
				IsEmergency:   false,
				PriorityLevel: 1,
			},
			GroupAddress:  1,
			SourceAddress: 100,
		},
	}
	enc1 := must(flc.Encode())
	enc2 := must(flc.Encode())
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestFullLinkControl_String(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  9990,
			SourceAddress: 3120101,
		},
		FEC: fec.Result{BitsChecked: 96},
	}
	str := flc.String()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	if !strings.Contains(str, "Group Voice Channel User") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
}

func TestFullLinkControl_ServiceOptions_RoundTrip(t *testing.T) {
	so := layer3.ServiceOptions{
		IsEmergency:         true,
		IsPrivacy:           true,
		IsBroadcast:         true,
		IsOpenVoiceCallMode: true,
		PriorityLevel:       3,
	}
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: so,
			GroupAddress:   1,
			SourceAddress:  2,
		},
	}
	infoBits := must(flc.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, flc.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.GroupVoice == nil {
		t.Fatal("GroupVoice is nil")
	}
	if decoded.GroupVoice.ServiceOptions.IsEmergency != true {
		t.Error("IsEmergency should survive round-trip")
	}
	if decoded.GroupVoice.ServiceOptions.IsPrivacy != true {
		t.Error("IsPrivacy should survive round-trip")
	}
	if decoded.GroupVoice.ServiceOptions.IsBroadcast != true {
		t.Error("IsBroadcast should survive round-trip")
	}
	if decoded.GroupVoice.ServiceOptions.IsOpenVoiceCallMode != true {
		t.Error("IsOpenVoiceCallMode should survive round-trip")
	}
	if decoded.GroupVoice.ServiceOptions.PriorityLevel != 3 {
		t.Errorf("PriorityLevel = %d, want 3", decoded.GroupVoice.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_Decode_BothDataTypes(t *testing.T) {
	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  100,
			SourceAddress: 200,
		},
	}
	infoBits := must(flc.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, flc.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOGroupVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOGroupVoiceChannelUser", decoded.FLCO)
	}
}

func TestFullLinkControl_AddressRange(t *testing.T) {
	tests := []struct {
		name       string
		groupAddr  uint32
		sourceAddr uint32
	}{
		{"MinAddresses", 0, 0},
		{"MaxAddresses", 0xFFFFFF, 0xFFFFFF},
		{"MidAddresses", 0x7FFFFF, 0x7FFFFF},
		{"TypicalAddresses", 9990, 3120101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flc := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				FLCO:          enums.FLCOGroupVoiceChannelUser,
				FID:           enums.FIDStandardized,
				GroupVoice: &pdu.FLCGroupVoice{
					GroupAddress:  tt.groupAddr,
					SourceAddress: tt.sourceAddr,
				},
			}
			infoBits := must(flc.Encode())

			decoded := pdu.DecodeFullLinkControl(infoBits, flc.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			if decoded.GroupVoice == nil {
				t.Fatal("GroupVoice is nil")
			}
			if decoded.GroupVoice.GroupAddress != tt.groupAddr {
				t.Errorf("GroupAddress = %d, want %d", decoded.GroupVoice.GroupAddress, tt.groupAddr)
			}
			if decoded.GroupVoice.SourceAddress != tt.sourceAddr {
				t.Errorf("SourceAddress = %d, want %d", decoded.GroupVoice.SourceAddress, tt.sourceAddr)
			}
		})
	}
}

func TestFullLinkControl_TalkerAliasBlock_EncodeDecodeCycle(t *testing.T) {
	flcoVariants := []enums.FLCO{
		enums.FLCOTalkerAliasBlock1,
		enums.FLCOTalkerAliasBlock2,
		enums.FLCOTalkerAliasBlock3,
	}

	for _, flco := range flcoVariants {
		t.Run(flco.String(), func(t *testing.T) {
			var aliasData [56]bit.Bit
			for i := 0; i < 56; i++ {
				if i%3 == 0 {
					aliasData[i] = 1
				}
			}

			original := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				ProtectFlag:   false,
				FLCO:          flco,
				FID:           enums.FIDStandardized,
				TalkerAliasBlock: &pdu.FLCTalkerAliasBlock{
					TalkerAliasData: [7]byte(bit.Pack(aliasData[:])),
				},
			}

			infoBits := must(original.Encode())
			decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			if decoded.FLCO != flco {
				t.Errorf("FLCO = %v, want %v", decoded.FLCO, flco)
			}
			if decoded.TalkerAliasBlock == nil {
				t.Fatal("TalkerAliasBlock is nil")
			}
			if decoded.TalkerAliasBlock.TalkerAliasData != [7]byte(bit.Pack(aliasData[:])) {
				t.Error("TalkerAliasData mismatch after encode-decode cycle")
			}
		})
	}
}

func TestFullLinkControl_TalkerAliasBlock_String(t *testing.T) {
	var aliasData [56]bit.Bit
	aliasData[0] = 1
	aliasData[7] = 1

	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOTalkerAliasBlock1,
		FID:           enums.FIDStandardized,
		TalkerAliasBlock: &pdu.FLCTalkerAliasBlock{
			TalkerAliasData: [7]byte(bit.Pack(aliasData[:])),
		},
		FEC: fec.Result{BitsChecked: 96},
	}
	str := flc.String()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	if !strings.Contains(str, "Talker Alias Block") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
}

func TestFullLinkControl_TalkerAliasHeader_DataLength_RoundTrip(t *testing.T) {
	// ETSI TS 102 361-2 Table 7.4: 5-bit data length (bits 2-6), then the
	// 49-bit data whose MSB is bit 7.
	tests := []struct {
		name       string
		dataLength int
		msb        bool
	}{
		{"Zero", 0, false},
		{"Small", 7, true},
		{"Medium", 15, false},
		{"Max", 31, false},
		{"MaxWithMSB", 31, true},
		{"ZeroWithMSB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var aliasData [48]bit.Bit
			aliasData[0] = 1
			aliasData[47] = 1

			original := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				ProtectFlag:   false,
				FLCO:          enums.FLCOTalkerAliasHeader,
				FID:           enums.FIDStandardized,
				TalkerAliasHeader: &pdu.FLCTalkerAliasHeader{
					TalkerAliasDataLength: tt.dataLength,
					TalkerAliasData:       msbOf(tt.msb) | bit.Uint(aliasData[:]),
				},
			}

			infoBits := must(original.Encode())
			decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			if decoded.TalkerAliasHeader == nil {
				t.Fatal("TalkerAliasHeader is nil")
			}
			if decoded.TalkerAliasHeader.TalkerAliasDataLength != tt.dataLength {
				t.Errorf("TalkerAliasDataLength = %d, want %d",
					decoded.TalkerAliasHeader.TalkerAliasDataLength, tt.dataLength)
			}
			if decoded.TalkerAliasHeader.TalkerAliasData != msbOf(tt.msb)|bit.Uint(aliasData[:]) {
				t.Error("TalkerAliasData mismatch after encode-decode cycle")
			}
		})
	}
}

func msbOf(msb bool) uint64 {
	if msb {
		return 1 << 48
	}
	return 0
}

func TestFLC_TerminatorDataLinkControl_RoundTrip(t *testing.T) {
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOTerminatorDataLinkControl,
		FID:           enums.FIDStandardized,
		TerminatorDataLinkControl: &pdu.FLCTerminatorDataLinkControl{
			LLIDDestination:    1234567,
			LLIDSource:         7654321,
			GroupOrIndividual:  true,
			ResponseRequested:  true,
			FullMessageFlag:    true,
			ReSynchronizeFlag:  false,
			SendSequenceNumber: 5,
		},
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOTerminatorDataLinkControl {
		t.Errorf("FLCO = %v, want FLCOTerminatorDataLinkControl", decoded.FLCO)
	}
	if decoded.TerminatorDataLinkControl == nil {
		t.Fatal("TerminatorDataLinkControl is nil")
	}
	tdlc := decoded.TerminatorDataLinkControl
	if tdlc.LLIDDestination != 1234567 {
		t.Errorf("LLIDDestination = %d, want 1234567", tdlc.LLIDDestination)
	}
	if tdlc.LLIDSource != 7654321 {
		t.Errorf("LLIDSource = %d, want 7654321", tdlc.LLIDSource)
	}
	if !tdlc.GroupOrIndividual {
		t.Error("GroupOrIndividual should be true")
	}
	if !tdlc.ResponseRequested {
		t.Error("ResponseRequested should be true")
	}
	if !tdlc.FullMessageFlag {
		t.Error("FullMessageFlag should be true")
	}
	if tdlc.ReSynchronizeFlag {
		t.Error("ReSynchronizeFlag should be false")
	}
	if tdlc.SendSequenceNumber != 5 {
		t.Errorf("SendSequenceNumber = %d, want 5", tdlc.SendSequenceNumber)
	}

	str := decoded.String()
	if !strings.Contains(str, "Terminator Data Link Control") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
}

func TestFullLinkControl_RSErrorsCountBits(t *testing.T) {
	lc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeTerminatorWithLC,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120101},
	}
	clean := must(lc.Encode())
	for _, flips := range []int{1, 3, 8} {
		rx := clean
		for i := range flips {
			rx[40+i] ^= 1
		}
		got := pdu.DecodeFullLinkControl(rx, lc.DataType())
		res := got.FEC
		want := fec.Result{BitsChecked: 96, ErrorsCorrected: flips}
		if res != want || got.FEC != want {
			t.Errorf("%d bit errors in one octet: result %+v, want %+v", flips, res, want)
		}
		if got.GroupVoice == nil || got.GroupVoice.GroupAddress != 91 {
			t.Errorf("%d bit errors in one octet: LC not corrected", flips)
		}
	}
}
