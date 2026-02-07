package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
	layer3Elements "github.com/USA-RedDragon/dmrgo/dmr/layer3/elements"
)

// buildInfoBits converts 12 packed bytes into 96 unpacked bits (one bit per byte).
func buildInfoBits(packed [12]byte) []byte {
	bits := make([]byte, 96)
	for i := 0; i < 12; i++ {
		for j := 0; j < 8; j++ {
			if (packed[i]>>(7-j))&1 == 1 {
				bits[i*8+j] = 1
			}
		}
	}
	return bits
}

func TestFullLinkControl_GroupVoice_EncodeDecodeCycle(t *testing.T) {
	original := &pdu.FullLinkControl{
		ProtectFlag:  false,
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		ServiceOptions: layer3Elements.ServiceOptions{
			IsEmergency:         false,
			IsPrivacy:           false,
			IsBroadcast:         false,
			IsOpenVoiceCallMode: false,
			PriorityLevel:       0,
		},
		GroupAddress:  9990,
		SourceAddress: 3120101,
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if len(encoded) != 12 {
		t.Fatalf("Encode returned %d bytes, want 12", len(encoded))
	}

	// Convert encoded bytes to info bits
	var packed [12]byte
	copy(packed[:], encoded)
	infoBits := buildInfoBits(packed)

	var decoded pdu.FullLinkControl
	ok := decoded.DecodeFromBits(infoBits, elements.DataTypeVoiceLCHeader)
	if !ok {
		t.Fatal("DecodeFromBits failed")
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
	if decoded.GroupAddress != original.GroupAddress {
		t.Errorf("GroupAddress = %d, want %d", decoded.GroupAddress, original.GroupAddress)
	}
	if decoded.SourceAddress != original.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", decoded.SourceAddress, original.SourceAddress)
	}
}

func TestFullLinkControl_UnitToUnit_EncodeDecodeCycle(t *testing.T) {
	original := &pdu.FullLinkControl{
		ProtectFlag:  true,
		FLCO:         enums.FLCOUnitToUnitVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		ServiceOptions: layer3Elements.ServiceOptions{
			IsEmergency:         true,
			IsPrivacy:           false,
			IsBroadcast:         false,
			IsOpenVoiceCallMode: false,
			PriorityLevel:       2,
		},
		TargetAddress: 1234567,
		SourceAddress: 7654321,
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var packed [12]byte
	copy(packed[:], encoded)
	infoBits := buildInfoBits(packed)

	var decoded pdu.FullLinkControl
	ok := decoded.DecodeFromBits(infoBits, elements.DataTypeVoiceLCHeader)
	if !ok {
		t.Fatal("DecodeFromBits failed")
	}
	if decoded.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		t.Errorf("FLCO = %v, want FLCOUnitToUnitVoiceChannelUser", decoded.FLCO)
	}
	if decoded.ProtectFlag != true {
		t.Errorf("ProtectFlag = %v, want true", decoded.ProtectFlag)
	}
	if decoded.TargetAddress != original.TargetAddress {
		t.Errorf("TargetAddress = %d, want %d", decoded.TargetAddress, original.TargetAddress)
	}
	if decoded.SourceAddress != original.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", decoded.SourceAddress, original.SourceAddress)
	}
	if decoded.ServiceOptions.IsEmergency != true {
		t.Error("ServiceOptions.IsEmergency should be true")
	}
	if decoded.ServiceOptions.PriorityLevel != 2 {
		t.Errorf("ServiceOptions.PriorityLevel = %d, want 2", decoded.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_EncodeStability(t *testing.T) {
	// Encoding the same FLC twice should produce identical bytes
	flc := &pdu.FullLinkControl{
		ProtectFlag:  false,
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		ServiceOptions: layer3Elements.ServiceOptions{
			IsEmergency:   false,
			PriorityLevel: 1,
		},
		GroupAddress:  1,
		SourceAddress: 100,
	}
	enc1, err := flc.Encode()
	if err != nil {
		t.Fatalf("Encode 1 failed: %v", err)
	}
	enc2, err := flc.Encode()
	if err != nil {
		t.Fatalf("Encode 2 failed: %v", err)
	}
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("byte %d: first=0x%02X, second=0x%02X", i, enc1[i], enc2[i])
		}
	}
}

func TestFullLinkControl_DecodeFromBits_InvalidLength(t *testing.T) {
	var flc pdu.FullLinkControl
	ok := flc.DecodeFromBits([]byte{0, 1, 2}, elements.DataTypeVoiceLCHeader)
	if ok {
		t.Error("DecodeFromBits should return false for invalid length")
	}
}

func TestFullLinkControl_DecodeFromBits_InvalidDataType(t *testing.T) {
	infoBits := make([]byte, 96)
	var flc pdu.FullLinkControl
	ok := flc.DecodeFromBits(infoBits, elements.DataTypeCSBK)
	if ok {
		t.Error("DecodeFromBits should return false for invalid dataType")
	}
}

func TestFullLinkControl_Encode_UnsupportedFLCO(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO: enums.FLCOGPSInfo,
	}
	_, err := flc.Encode()
	if err == nil {
		t.Error("Encode should return error for unsupported FLCO (GPSInfo)")
	}
}

func TestFullLinkControl_ToString(t *testing.T) {
	flc := &pdu.FullLinkControl{
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.StandardizedFID,
		GroupAddress:  9990,
		SourceAddress: 3120101,
		ParityOK:      true,
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
		FLCO:           enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:   enums.StandardizedFID,
		ServiceOptions: so,
		GroupAddress:   1,
		SourceAddress:  2,
	}
	encoded, err := flc.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	var packed [12]byte
	copy(packed[:], encoded)
	infoBits := buildInfoBits(packed)

	var decoded pdu.FullLinkControl
	ok := decoded.DecodeFromBits(infoBits, elements.DataTypeVoiceLCHeader)
	if !ok {
		t.Fatal("DecodeFromBits failed")
	}
	if decoded.ServiceOptions.IsEmergency != true {
		t.Error("IsEmergency should survive round-trip")
	}
	if decoded.ServiceOptions.IsPrivacy != true {
		t.Error("IsPrivacy should survive round-trip")
	}
	if decoded.ServiceOptions.IsBroadcast != true {
		t.Error("IsBroadcast should survive round-trip")
	}
	if decoded.ServiceOptions.IsOpenVoiceCallMode != true {
		t.Error("IsOpenVoiceCallMode should survive round-trip")
	}
	if decoded.ServiceOptions.PriorityLevel != 3 {
		t.Errorf("PriorityLevel = %d, want 3", decoded.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_DecodeFromBits_BothDataTypes(t *testing.T) {
	// Same valid data should decode correctly with both accepted data types
	flc := &pdu.FullLinkControl{
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.StandardizedFID,
		GroupAddress:  100,
		SourceAddress: 200,
	}
	encoded, err := flc.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	var packed [12]byte
	copy(packed[:], encoded)
	infoBits := buildInfoBits(packed)

	dataTypes := []elements.DataType{
		elements.DataTypeVoiceLCHeader,
		elements.DataTypeTerminatorWithLC,
	}
	for _, dt := range dataTypes {
		var decoded pdu.FullLinkControl
		ok := decoded.DecodeFromBits(infoBits, dt)
		if !ok {
			t.Errorf("DecodeFromBits failed for dataType %s", elements.DataTypeToName(dt))
		}
		if decoded.GetDataType() != dt {
			t.Errorf("GetDataType() = %v, want %v", decoded.GetDataType(), dt)
		}
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
				FLCO:          enums.FLCOGroupVoiceChannelUser,
				FeatureSetID:  enums.StandardizedFID,
				GroupAddress:  tt.groupAddr,
				SourceAddress: tt.sourceAddr,
			}
			encoded, err := flc.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			var packed [12]byte
			copy(packed[:], encoded)
			infoBits := buildInfoBits(packed)

			var decoded pdu.FullLinkControl
			ok := decoded.DecodeFromBits(infoBits, elements.DataTypeVoiceLCHeader)
			if !ok {
				t.Fatal("DecodeFromBits failed")
			}
			if decoded.GroupAddress != tt.groupAddr {
				t.Errorf("GroupAddress = %d, want %d", decoded.GroupAddress, tt.groupAddr)
			}
			if decoded.SourceAddress != tt.sourceAddr {
				t.Errorf("SourceAddress = %d, want %d", decoded.SourceAddress, tt.sourceAddr)
			}
		})
	}
}
