package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestFullLinkControl_GroupVoice_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
	}
	want := pdu.FLCGroupVoice{
		ServiceOptions: layer3.ServiceOptions{
			IsEmergency:         false,
			IsPrivacy:           false,
			IsBroadcast:         false,
			IsOpenVoiceCallMode: false,
			PriorityLevel:       0,
		},
		GroupAddress:  9990,
		SourceAddress: 3120101,
	}
	noErr(original.SetGroupVoice(want))

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
	gv, ok := decoded.AsGroupVoice()
	if !ok {
		t.Fatal("AsGroupVoice ok = false")
	}
	if gv.GroupAddress != want.GroupAddress {
		t.Errorf("GroupAddress = %d, want %d", gv.GroupAddress, want.GroupAddress)
	}
	if gv.SourceAddress != want.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", gv.SourceAddress, want.SourceAddress)
	}
}

func TestFullLinkControl_UnitToUnit_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   true,
		FLCO:          enums.FLCOUnitToUnitVoiceChannelUser,
		FID:           enums.FIDStandardized,
	}
	want := pdu.FLCUnitToUnit{
		ServiceOptions: layer3.ServiceOptions{
			IsEmergency:         true,
			IsPrivacy:           false,
			IsBroadcast:         false,
			IsOpenVoiceCallMode: false,
			PriorityLevel:       2,
		},
		TargetAddress: 1234567,
		SourceAddress: 7654321,
	}
	noErr(original.SetUnitToUnit(want))

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
	uu, ok := decoded.AsUnitToUnit()
	if !ok {
		t.Fatal("AsUnitToUnit ok = false")
	}
	if uu.TargetAddress != want.TargetAddress {
		t.Errorf("TargetAddress = %d, want %d", uu.TargetAddress, want.TargetAddress)
	}
	if uu.SourceAddress != want.SourceAddress {
		t.Errorf("SourceAddress = %d, want %d", uu.SourceAddress, want.SourceAddress)
	}
	if uu.ServiceOptions.IsEmergency != true {
		t.Error("ServiceOptions.IsEmergency should be true")
	}
	if uu.ServiceOptions.PriorityLevel != 2 {
		t.Errorf("ServiceOptions.PriorityLevel = %d, want 2", uu.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_EncodeStability(t *testing.T) {
	t.Parallel()

	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
	}
	noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{
		ServiceOptions: layer3.ServiceOptions{
			IsEmergency:   false,
			PriorityLevel: 1,
		},
		GroupAddress:  1,
		SourceAddress: 100,
	}))
	enc1 := must(flc.Encode())
	enc2 := must(flc.Encode())
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("byte %d: first=%#02x, second=%#02x", i, enc1[i], enc2[i])
		}
	}
}

func TestFullLinkControl_String(t *testing.T) {
	t.Parallel()

	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
		FEC:           fec.Result{BitsChecked: 96},
	}
	noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{
		GroupAddress:  9990,
		SourceAddress: 3120101,
	}))
	str := flc.String()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	if !strings.Contains(str, "Group Voice Channel User") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
	if !strings.HasPrefix(str, "FullLinkControl{ BurstDataType: Voice LC Header, ProtectFlag: ") {
		t.Errorf("header LC: %s", str)
	}
	lc, err := pdu.NewVoiceLCBytes(true, 9, 3120101, layer3.ServiceOptions{})
	noErr(err)
	emb := pdu.DecodeFullLinkControlBytes(lc)
	if s := emb.String(); strings.Contains(s, "BurstDataType") || !strings.HasPrefix(s, "FullLinkControl{ ProtectFlag: ") {
		t.Errorf("embedded LC without a burst data type: %s", s)
	}
}

func TestFullLinkControl_ServiceOptions_RoundTrip(t *testing.T) {
	t.Parallel()

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
	}
	noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{
		ServiceOptions: so,
		GroupAddress:   1,
		SourceAddress:  2,
	}))
	infoBits := must(flc.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, flc.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	gv, ok := decoded.AsGroupVoice()
	if !ok {
		t.Fatal("AsGroupVoice ok = false")
	}
	if gv.ServiceOptions.IsEmergency != true {
		t.Error("IsEmergency should survive round-trip")
	}
	if gv.ServiceOptions.IsPrivacy != true {
		t.Error("IsPrivacy should survive round-trip")
	}
	if gv.ServiceOptions.IsBroadcast != true {
		t.Error("IsBroadcast should survive round-trip")
	}
	if gv.ServiceOptions.IsOpenVoiceCallMode != true {
		t.Error("IsOpenVoiceCallMode should survive round-trip")
	}
	if gv.ServiceOptions.PriorityLevel != 3 {
		t.Errorf("PriorityLevel = %d, want 3", gv.ServiceOptions.PriorityLevel)
	}
}

func TestFullLinkControl_Decode_BothDataTypes(t *testing.T) {
	t.Parallel()

	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDStandardized,
	}
	noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{
		GroupAddress:  100,
		SourceAddress: 200,
	}))
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
	t.Parallel()

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
			t.Parallel()

			flc := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				FLCO:          enums.FLCOGroupVoiceChannelUser,
				FID:           enums.FIDStandardized,
			}
			noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{
				GroupAddress:  tt.groupAddr,
				SourceAddress: tt.sourceAddr,
			}))
			infoBits := must(flc.Encode())

			decoded := pdu.DecodeFullLinkControl(infoBits, flc.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			gv, ok := decoded.AsGroupVoice()
			if !ok {
				t.Fatal("AsGroupVoice ok = false")
			}
			if gv.GroupAddress != tt.groupAddr {
				t.Errorf("GroupAddress = %d, want %d", gv.GroupAddress, tt.groupAddr)
			}
			if gv.SourceAddress != tt.sourceAddr {
				t.Errorf("SourceAddress = %d, want %d", gv.SourceAddress, tt.sourceAddr)
			}
		})
	}
}

func TestFullLinkControl_TalkerAliasBlock_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	flcoVariants := []enums.FLCO{
		enums.FLCOTalkerAliasBlock1,
		enums.FLCOTalkerAliasBlock2,
		enums.FLCOTalkerAliasBlock3,
	}

	for _, flco := range flcoVariants {
		t.Run(flco.String(), func(t *testing.T) {
			t.Parallel()

			var aliasData [7]byte
			for i := 0; i < 56; i++ {
				if i%3 == 0 {
					setRawBit(aliasData[:], i)
				}
			}

			original := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				ProtectFlag:   false,
				FID:           enums.FIDStandardized,
			}
			noErr(original.SetTalkerAliasBlock(flco, pdu.FLCTalkerAliasBlock{
				TalkerAliasData: aliasData,
			}))

			infoBits := must(original.Encode())
			decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			if decoded.FLCO != flco {
				t.Errorf("FLCO = %v, want %v", decoded.FLCO, flco)
			}
			blk, ok := decoded.AsTalkerAliasBlock()
			if !ok {
				t.Fatal("AsTalkerAliasBlock ok = false")
			}
			if blk.TalkerAliasData != aliasData {
				t.Error("TalkerAliasData mismatch after encode-decode cycle")
			}
		})
	}
}

func TestFullLinkControl_TalkerAliasBlock_String(t *testing.T) {
	t.Parallel()

	var aliasData [7]byte
	setRawBit(aliasData[:], 0)
	setRawBit(aliasData[:], 7)

	flc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FID:           enums.FIDStandardized,
		FEC:           fec.Result{BitsChecked: 96},
	}
	noErr(flc.SetTalkerAliasBlock(enums.FLCOTalkerAliasBlock1, pdu.FLCTalkerAliasBlock{
		TalkerAliasData: aliasData,
	}))
	str := flc.String()
	if len(str) == 0 {
		t.Error("ToString returned empty string")
	}
	if !strings.Contains(str, "Talker Alias Block") {
		t.Errorf("ToString missing FLCO name, got: %s", str)
	}
}

func TestFullLinkControl_TalkerAliasHeader_DataLength_RoundTrip(t *testing.T) {
	t.Parallel()

	// ETSI TS 102 361-2 Table 7.4
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
			t.Parallel()

			aliasData := uint64(1<<47 | 1)

			original := &pdu.FullLinkControl{
				BurstDataType: enums.DataTypeVoiceLCHeader,
				ProtectFlag:   false,
				FLCO:          enums.FLCOTalkerAliasHeader,
				FID:           enums.FIDStandardized,
			}
			noErr(original.SetTalkerAliasHeader(pdu.FLCTalkerAliasHeader{
				TalkerAliasDataLength: uint8(tt.dataLength),
				TalkerAliasData:       msbOf(tt.msb) | aliasData,
			}))

			infoBits := must(original.Encode())
			decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
			}
			hdr, ok := decoded.AsTalkerAliasHeader()
			if !ok {
				t.Fatal("AsTalkerAliasHeader ok = false")
			}
			if int(hdr.TalkerAliasDataLength) != tt.dataLength {
				t.Errorf("TalkerAliasDataLength = %d, want %d",
					hdr.TalkerAliasDataLength, tt.dataLength)
			}
			if hdr.TalkerAliasData != msbOf(tt.msb)|aliasData {
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
	t.Parallel()

	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		ProtectFlag:   false,
		FLCO:          enums.FLCOTerminatorDataLinkControl,
		FID:           enums.FIDStandardized,
	}
	noErr(original.SetTerminatorDataLinkControl(pdu.FLCTerminatorDataLinkControl{
		LLIDDestination:    1234567,
		LLIDSource:         7654321,
		Group:              true,
		ResponseRequested:  true,
		FullMessageFlag:    true,
		ReSynchronizeFlag:  false,
		SendSequenceNumber: 5,
	}))

	infoBits := must(original.Encode())
	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FLCO != enums.FLCOTerminatorDataLinkControl {
		t.Errorf("FLCO = %v, want FLCOTerminatorDataLinkControl", decoded.FLCO)
	}
	tdlc, ok := decoded.AsTerminatorDataLinkControl()
	if !ok {
		t.Fatal("AsTerminatorDataLinkControl ok = false")
	}
	if tdlc.LLIDDestination != 1234567 {
		t.Errorf("LLIDDestination = %d, want 1234567", tdlc.LLIDDestination)
	}
	if tdlc.LLIDSource != 7654321 {
		t.Errorf("LLIDSource = %d, want 7654321", tdlc.LLIDSource)
	}
	if !tdlc.Group {
		t.Error("Group should be true")
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
	t.Parallel()

	lc := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeTerminatorWithLC,
	}
	noErr(lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120101}))
	clean := must(lc.Encode())
	for _, flips := range []int{1, 3, 8} {
		rx := clean
		for i := range flips {
			rx[(40+i)/8] ^= 0x80 >> ((40 + i) % 8)
		}
		got := pdu.DecodeFullLinkControl(rx, lc.DataType())
		res := got.FEC
		want := fec.Result{BitsChecked: 96, ErrorsCorrected: flips}
		if res != want || got.FEC != want {
			t.Errorf("%d bit errors in one octet: result %+v, want %+v", flips, res, want)
		}
		if gv, ok := got.AsGroupVoice(); !ok || gv.GroupAddress != 91 {
			t.Errorf("%d bit errors in one octet: LC not corrected", flips)
		}
	}
}
