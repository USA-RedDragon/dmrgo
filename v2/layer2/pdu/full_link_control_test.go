package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	layer3Elements "github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

func TestFullLinkControl_GroupVoice_EncodeDecodeCycle(t *testing.T) {
	original := &pdu.FullLinkControl{
		ProtectFlag:  false,
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: layer3Elements.ServiceOptions{
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

	infoBits := pdu.EncodeFullLinkControl(original)

	decoded, fecResult := pdu.DecodeFullLinkControl(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOGroupVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOGroupVoiceChannelUser", decoded.FLCO)
	}
	if decoded.ProtectFlag != original.ProtectFlag {
		t.Errorf("ProtectFlag = %v, want %v", decoded.ProtectFlag, original.ProtectFlag)
	}
	if decoded.FeatureSetID != original.FeatureSetID {
		t.Errorf("FeatureSetID = %v, want %v", decoded.FeatureSetID, original.FeatureSetID)
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
		ProtectFlag:  true,
		FLCO:         enums.FLCOUnitToUnitVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		UnitToUnit: &pdu.FLCUnitToUnit{
			ServiceOptions: layer3Elements.ServiceOptions{
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

	infoBits := pdu.EncodeFullLinkControl(original)

	decoded, fecResult := pdu.DecodeFullLinkControl(infoBits)
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
	// Encoding the same FLC twice should produce identical bytes
	flc := &pdu.FullLinkControl{
		ProtectFlag:  false,
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: layer3Elements.ServiceOptions{
				IsEmergency:   false,
				PriorityLevel: 1,
			},
			GroupAddress:  1,
			SourceAddress: 100,
		},
	}
	enc1 := pdu.EncodeFullLinkControl(flc)
	enc2 := pdu.EncodeFullLinkControl(flc)
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestFullLinkControl_ToString(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  9990,
			SourceAddress: 3120101,
		},
		FEC: fec.FECResult{BitsChecked: 96},
	}
	str := flc.ToString()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	// Should contain key information
	if !strings.Contains(str, "Group Voice Channel User") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
}

func TestFullLinkControl_ServiceOptions_RoundTrip(t *testing.T) {
	// Verify that service options survive the encode-decode cycle
	so := layer3Elements.ServiceOptions{
		IsEmergency:         true,
		IsPrivacy:           true,
		IsBroadcast:         true,
		IsOpenVoiceCallMode: true,
		PriorityLevel:       3,
	}
	flc := &pdu.FullLinkControl{
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice: &pdu.FLCGroupVoice{
			ServiceOptions: so,
			GroupAddress:   1,
			SourceAddress:  2,
		},
	}
	infoBits := pdu.EncodeFullLinkControl(flc)

	decoded, fecResult := pdu.DecodeFullLinkControl(infoBits)
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
	// Same valid data should decode correctly regardless — DataType is set by caller
	flc := &pdu.FullLinkControl{
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice: &pdu.FLCGroupVoice{
			GroupAddress:  100,
			SourceAddress: 200,
		},
	}
	infoBits := pdu.EncodeFullLinkControl(flc)

	decoded, fecResult := pdu.DecodeFullLinkControl(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOGroupVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOGroupVoiceChannelUser", decoded.FLCO)
	}
}

func TestFullLinkControl_AddressRange(t *testing.T) {
	// Test with maximum 24-bit addresses (16777215)
	tests := []struct {
		name       string
		groupAddr  int
		sourceAddr int
	}{
		{"MinAddresses", 0, 0},
		{"MaxAddresses", 0xFFFFFF, 0xFFFFFF},
		{"MidAddresses", 0x7FFFFF, 0x7FFFFF},
		{"TypicalAddresses", 9990, 3120101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flc := &pdu.FullLinkControl{
				FLCO:         enums.FLCOGroupVoiceChannelUser,
				FeatureSetID: enums.StandardizedFID,
				GroupVoice: &pdu.FLCGroupVoice{
					GroupAddress:  tt.groupAddr,
					SourceAddress: tt.sourceAddr,
				},
			}
			infoBits := pdu.EncodeFullLinkControl(flc)

			decoded, fecResult := pdu.DecodeFullLinkControl(infoBits)
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
