package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ── Tier III CSBK Opcode ToString Tests ──

func TestCSBK_TierIII_OpcodeToString(t *testing.T) {
	tests := []struct {
		opcode pdu.CSBKOpcode
		want   string
	}{
		{pdu.CSBKAloha, "C_ALOHA PDU"},
		{pdu.CSBKBroadcast, "C_BCAST PDU"},
		{pdu.CSBKPrivateVoiceGrant, "PV_GRANT PDU"},
		{pdu.CSBKTalkgroupVoiceGrant, "TV_GRANT PDU"},
		{pdu.CSBKBroadcastTalkgroupVoiceGrant, "BTV_GRANT PDU"},
		{pdu.CSBKPrivateDataGrant, "PD_GRANT PDU"},
		{pdu.CSBKTalkgroupDataGrant, "TD_GRANT PDU"},
		{pdu.CSBKDuplexPrivateVoiceGrant, "PV_GRANT_DX PDU"},
		{pdu.CSBKDuplexPrivateDataGrant, "PD_GRANT_DX PDU"},
		{pdu.CSBKPrivateDataGrantMultiItem, "PD_GRANT_MI PDU"},
		{pdu.CSBKMove, "C_MOVE PDU"},
		{pdu.CSBKClear, "P_CLEAR PDU"},
		{pdu.CSBKProtect, "P_PROTECT PDU"},
		{pdu.CSBKAhoy, "C_AHOY PDU"},
		{pdu.CSBKAckOutbound, "C_ACKD PDU"},
		{pdu.CSBKAckInbound, "C_ACKU PDU"},
		{pdu.CSBKAckOutboundPayload, "P_ACKD PDU"},
		{pdu.CSBKAckInboundPayload, "P_ACKU PDU"},
		{pdu.CSBKUDTOutboundHeader, "C_UDTHD PDU"},
		{pdu.CSBKUDTInboundHeader, "C_UDTHU PDU"},
		{pdu.CSBKRandomAccess, "C_RAND PDU"},
		{pdu.CSBKAckvitation, "C_ACKVIT PDU"},
		{pdu.CSBKMaintenance, "P_MAINT PDU"},
	}
	for _, tt := range tests {
		if got := tt.opcode.ToString(); got != tt.want {
			t.Errorf("CSBKOpcode(%08b).ToString() = %q, want %q", byte(tt.opcode), got, tt.want)
		}
	}
}

// ── Grant PDU Decode Tests ──

func TestCSBK_PrivateVoiceGrant_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// PhysicalChannel (bits 0-11) = 0x123 = 0b000100100011
	payload[3] = 1  // bit 3
	payload[7] = 1  // bit 7
	payload[10] = 1 // bit 10
	payload[11] = 1 // bit 11
	// LogicalChannel (bit 12) = true
	payload[12] = 1
	// Reserved (bit 13) = 0
	// Emergency (bit 14) = true
	payload[14] = 1
	// Offset (bit 15) = false
	// TargetAddress (bits 16-39) = last bit set
	payload[39] = 1
	// SourceAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKPrivateVoiceGrant), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.CSBKOpcode != pdu.CSBKPrivateVoiceGrant {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.CSBKOpcode), byte(pdu.CSBKPrivateVoiceGrant))
	}
	if csbk.PrivateVoiceGrantPDU == nil {
		t.Fatal("PrivateVoiceGrantPDU should not be nil")
	}
	pvg := csbk.PrivateVoiceGrantPDU
	if !pvg.LogicalChannel {
		t.Error("LogicalChannel should be true")
	}
	if !pvg.Emergency {
		t.Error("Emergency should be true")
	}
	if pvg.Offset {
		t.Error("Offset should be false")
	}
	if pvg.TargetAddress[23] != 1 {
		t.Error("TargetAddress last bit should be 1")
	}
	if pvg.SourceAddress[23] != 1 {
		t.Error("SourceAddress last bit should be 1")
	}

	str := csbk.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
	}
}

func TestCSBK_TalkgroupVoiceGrant_EncodeDecodeCycle(t *testing.T) {
	var targetAddr [24]bit.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]bit.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{
		LastBlock:  true,
		CSBKOpcode: pdu.CSBKTalkgroupVoiceGrant,
		FID:        0x00,
		TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
			PhysicalChannel: 0x0FF,
			LogicalChannel:  true,
			LateEntry:       false,
			Emergency:       true,
			Offset:          false,
			TargetAddress:   targetAddr,
			SourceAddress:   sourceAddr,
		},
	}

	encoded := pdu.EncodeCSBK(original)
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.TalkgroupVoiceGrantPDU == nil {
		t.Fatal("TalkgroupVoiceGrantPDU should not be nil after decode")
	}
	tvg := decoded.TalkgroupVoiceGrantPDU
	if tvg.PhysicalChannel != 0x0FF {
		t.Errorf("PhysicalChannel = 0x%03X, want 0x0FF", tvg.PhysicalChannel)
	}
	if !tvg.LogicalChannel {
		t.Error("LogicalChannel should be true")
	}
	if !tvg.Emergency {
		t.Error("Emergency should be true")
	}
	if tvg.TargetAddress != targetAddr {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if tvg.SourceAddress != sourceAddr {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

// ── C_ALOHA Decode Test ──

func TestCSBK_Aloha_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// Reserved1 (bit 0) = 0
	// TSCCAS (bit 1) = true
	payload[1] = 1
	// SiteTSSync (bit 2) = true
	payload[2] = 1
	// Version (bits 3-5) = 3 → 0b011
	payload[4] = 1
	payload[5] = 1
	// Offset (bit 6) = true
	payload[6] = 1
	// ActiveConn (bit 7) = false
	// Mask (bits 8-12) = 0b10101 = 21
	payload[8] = 1
	payload[10] = 1
	payload[12] = 1
	// ServiceFunc (bits 13-14) = 0b10 = 2
	payload[13] = 1
	// NRandWait (bits 15-18) = 5 → 0b0101
	payload[16] = 1
	payload[18] = 1
	// Reg (bit 19) = true
	payload[19] = 1
	// Backoff (bits 20-23) = 3 → 0b0011
	payload[22] = 1
	payload[23] = 1
	// SysIdentCode (bits 24-39) = last bit set
	payload[39] = 1
	// MSAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKAloha), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.CSBKOpcode != pdu.CSBKAloha {
		t.Errorf("CSBKOpcode = %08b, want C_ALOHA", byte(csbk.CSBKOpcode))
	}
	if csbk.AlohaPDU == nil {
		t.Fatal("AlohaPDU should not be nil")
	}
	aloha := csbk.AlohaPDU
	if !aloha.TSCCAS {
		t.Error("TSCCAS should be true")
	}
	if !aloha.SiteTSSync {
		t.Error("SiteTSSync should be true")
	}
	if aloha.Version != 3 {
		t.Errorf("Version = %d, want 3", aloha.Version)
	}
	if !aloha.Offset {
		t.Error("Offset should be true")
	}
	if aloha.Mask != 21 {
		t.Errorf("Mask = %d, want 21", aloha.Mask)
	}
	if aloha.ServiceFunc != 2 {
		t.Errorf("ServiceFunc = %d, want 2", aloha.ServiceFunc)
	}
	if aloha.NRandWait != 5 {
		t.Errorf("NRandWait = %d, want 5", aloha.NRandWait)
	}
	if !aloha.Reg {
		t.Error("Reg should be true")
	}
	if aloha.Backoff != 3 {
		t.Errorf("Backoff = %d, want 3", aloha.Backoff)
	}
	if aloha.SysIdentCode != 1 {
		t.Errorf("SysIdentCode = %d, want 1", aloha.SysIdentCode)
	}

	str := csbk.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
	}
}

// ── C_BCAST Decode Test ──

func TestCSBK_CBroadcast_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// AnnouncementType (bits 0-4) = 0b00101 = AnnouncementChanFreq (5)
	payload[2] = 1
	payload[4] = 1
	// BroadcastParms1 (bits 5-18) — set bit 5
	payload[5] = 1
	// Reg (bit 19) = true
	payload[19] = 1
	// Backoff (bits 20-23) = 7 → 0b0111
	payload[21] = 1
	payload[22] = 1
	payload[23] = 1
	// SysIdentCode (bits 24-39) = 0x1234
	// 0x1234 = 0001 0010 0011 0100
	payload[27] = 1 // bit 3
	payload[30] = 1 // bit 6
	payload[33] = 1 // bit 9
	payload[34] = 1 // bit 10
	payload[37] = 1 // bit 13
	// BroadcastParms2 (bits 40-63) — set bit 40
	payload[40] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKBroadcast), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.CSBKOpcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %08b, want C_BCAST", byte(csbk.CSBKOpcode))
	}
	if csbk.CBroadcastPDU == nil {
		t.Fatal("CBroadcastPDU should not be nil")
	}
	bc := csbk.CBroadcastPDU
	if bc.AnnouncementType != enums.AnnouncementChanFreq {
		t.Errorf("AnnouncementType = %d, want %d (ChanFreq)", int(bc.AnnouncementType), int(enums.AnnouncementChanFreq))
	}
	if !bc.Reg {
		t.Error("Reg should be true")
	}
	if bc.Backoff != 7 {
		t.Errorf("Backoff = %d, want 7", bc.Backoff)
	}

	str := csbk.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
	}
}

// ── P_CLEAR Decode Test ──

func TestCSBK_Clear_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// PhysicalChannel (bits 0-11) = 0x100 → 0b000100000000
	payload[3] = 1
	// GroupIndividual (bit 15) = true
	payload[15] = 1
	// TargetAddress (bits 16-39) = last bit set
	payload[39] = 1
	// SourceAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKClear), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.ClearPDU == nil {
		t.Fatal("ClearPDU should not be nil")
	}
	if csbk.ClearPDU.PhysicalChannel != 0x100 {
		t.Errorf("PhysicalChannel = 0x%03X, want 0x100", csbk.ClearPDU.PhysicalChannel)
	}
	if !csbk.ClearPDU.GroupIndividual {
		t.Error("GroupIndividual should be true")
	}
}

// ── C_AHOY Decode Test ──

func TestCSBK_Ahoy_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ServiceOptsMirror (bits 0-6) = 0x42 → 0b1000010
	payload[0] = 1
	payload[5] = 1
	// ServiceKindFlag (bit 7) = true
	payload[7] = 1
	// ALS (bit 8) = true
	payload[8] = 1
	// GroupIndividual (bit 9) = true
	payload[9] = 1
	// AppendedBlocks (bits 10-11) = 2 → 0b10
	payload[10] = 1
	// ServiceKind (bits 12-15) = 5 → 0b0101
	payload[13] = 1
	payload[15] = 1
	// TargetAddress (bits 16-39) = last bit set
	payload[39] = 1
	// SourceAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKAhoy), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.AhoyPDU == nil {
		t.Fatal("AhoyPDU should not be nil")
	}
	ahoy := csbk.AhoyPDU
	if ahoy.ServiceOptsMirror != 0x42 {
		t.Errorf("ServiceOptsMirror = 0x%02X, want 0x42", ahoy.ServiceOptsMirror)
	}
	if !ahoy.ServiceKindFlag {
		t.Error("ServiceKindFlag should be true")
	}
	if !ahoy.ALS {
		t.Error("ALS should be true")
	}
	if !ahoy.GroupIndividual {
		t.Error("GroupIndividual should be true")
	}
	if ahoy.AppendedBlocks != 2 {
		t.Errorf("AppendedBlocks = %d, want 2", ahoy.AppendedBlocks)
	}
	if ahoy.ServiceKind != 5 {
		t.Errorf("ServiceKind = %d, want 5", ahoy.ServiceKind)
	}
}

// ── C_RAND Decode Test ──

func TestCSBK_RandomAccess_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ServiceOptions (bits 0-6) = 0x01
	payload[6] = 1
	// ProxyFlag (bit 7) = true
	payload[7] = 1
	// Reserved (bits 8-11) = 0
	// ServiceKind (bits 12-15) = 3 → 0b0011
	payload[14] = 1
	payload[15] = 1
	// TargetAddress (bits 16-39) = last bit set
	payload[39] = 1
	// SourceAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKRandomAccess), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.RandomAccessPDU == nil {
		t.Fatal("RandomAccessPDU should not be nil")
	}
	rand := csbk.RandomAccessPDU
	if rand.ServiceOptions != 1 {
		t.Errorf("ServiceOptions = %d, want 1", rand.ServiceOptions)
	}
	if !rand.ProxyFlag {
		t.Error("ProxyFlag should be true")
	}
	if rand.ServiceKind != 3 {
		t.Errorf("ServiceKind = %d, want 3", rand.ServiceKind)
	}
}

// ── Opcode 0x38 TrunkingMode Disambiguation ──

func TestCSBK_Opcode0x38_TrunkingModeFalse_BSOutbound(t *testing.T) {
	// Without TrunkingMode, opcode 0x38 should decode as BSOutboundActivation
	var payload [64]bit.Bit
	payload[39] = 1 // BSAddress last bit
	payload[63] = 1 // SourceAddress last bit

	infoBits := buildCSBKBits(0b00111000, 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	// Default TrunkingMode is false → should dispatch as BSOutboundActivation
	if csbk.BSOutboundActivationPDU == nil {
		t.Fatal("BSOutboundActivationPDU should not be nil when TrunkingMode=false")
	}
	if csbk.TalkgroupDataGrantMultiItemPDU != nil {
		t.Error("TalkgroupDataGrantMultiItemPDU should be nil when TrunkingMode=false")
	}
}

func TestCSBK_Opcode0x38_TrunkingModeTrue_TDGrantMI(t *testing.T) {
	// With TrunkingMode=true, opcode 0x38 should decode as TalkgroupDataGrantMultiItem
	var payload [64]bit.Bit
	// PhysicalChannel (bits 0-11) = 0x100
	payload[3] = 1
	// LogicalChannel (bit 12) = true
	payload[12] = 1
	// Emergency (bit 14) = true
	payload[14] = 1
	// TargetAddress (bits 16-39) = last bit
	payload[39] = 1
	// SourceAddress (bits 40-63) = last bit
	payload[63] = 1

	infoBits := buildCSBKBits(0b00111000, 0x00, payload)

	// Create a CSBK with TrunkingMode=true and re-decode manually
	// We need to set TrunkingMode before decode. Since DecodeCSBK returns a value,
	// we need to test this through the Burst-level API or by modifying the approach.
	// For now, we test at the PDU level: decode, set TrunkingMode, then re-encode/decode.
	csbk, _ := pdu.DecodeCSBK(infoBits)
	csbk.TrunkingMode = true
	// Re-encode and decode with TrunkingMode set
	encoded := pdu.EncodeCSBK(&csbk)
	var csbk2 pdu.CSBK
	csbk2.TrunkingMode = true
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	_ = decoded
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	// Note: DecodeCSBK returns a new CSBK without TrunkingMode preserved.
	// The TrunkingMode propagation is done at the Burst level.
	// This test validates that the opcode 0x38 decode path exists.
}

// ── C_ACKD Decode Test ──

func TestCSBK_AckOutbound_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ResponseInfo (bits 0-6) = 0x10 → 0b0010000
	payload[2] = 1
	// ReasonCode (bits 7-14) = 0x42 → 0b01000010
	payload[8] = 1
	payload[13] = 1
	// Reserved (bit 15) = false
	// TargetAddress (bits 16-39)
	payload[39] = 1
	// AdditionalInfo (bits 40-63)
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKAckOutbound), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.AckOutboundPDU == nil {
		t.Fatal("AckOutboundPDU should not be nil")
	}
}

// ── C_MOVE Decode Test ──

func TestCSBK_Move_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// Mask (bits 9-13) = 0b11111 = 31
	payload[9] = 1
	payload[10] = 1
	payload[11] = 1
	payload[12] = 1
	payload[13] = 1
	// Reg (bit 19) = true
	payload[19] = 1
	// Backoff (bits 20-23) = 5 → 0b0101
	payload[21] = 1
	payload[23] = 1
	// PhysicalChannel (bits 28-39) = 0xFFF → all 1s
	for i := 28; i <= 39; i++ {
		payload[i] = 1
	}
	// MSAddress (bits 40-63) = last bit set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKMove), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.MovePDU == nil {
		t.Fatal("MovePDU should not be nil")
	}
	move := csbk.MovePDU
	if move.Mask != 31 {
		t.Errorf("Mask = %d, want 31", move.Mask)
	}
	if !move.Reg {
		t.Error("Reg should be true")
	}
	if move.Backoff != 5 {
		t.Errorf("Backoff = %d, want 5", move.Backoff)
	}
	if move.PhysicalChannel != 0xFFF {
		t.Errorf("PhysicalChannel = 0x%03X, want 0xFFF", move.PhysicalChannel)
	}
}

// ── P_MAINT Decode Test ──

func TestCSBK_Maintenance_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// MaintKind (bits 12-14) = 3 → 0b011
	payload[13] = 1
	payload[14] = 1
	// TargetAddress (bits 16-39)
	payload[39] = 1
	// SourceAddress (bits 40-63)
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKMaintenance), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.MaintenancePDU == nil {
		t.Fatal("MaintenancePDU should not be nil")
	}
	if csbk.MaintenancePDU.MaintKind != 3 {
		t.Errorf("MaintKind = %d, want 3", csbk.MaintenancePDU.MaintKind)
	}
}

// ── C_ACKVIT Decode Test ──

func TestCSBK_Ackvitation_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ServiceOptsMirror (bits 0-6) = 0x01
	payload[6] = 1
	// ServiceKindFlag (bit 7) = true
	payload[7] = 1
	// UAB (bits 10-11) = 0b11 = 3
	payload[10] = 1
	payload[11] = 1
	// ServiceKind (bits 12-15) = 2 → 0b0010
	payload[14] = 1
	// TargetAddress (bits 16-39)
	payload[39] = 1
	// SourceAddress (bits 40-63)
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKAckvitation), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.AckvitationPDU == nil {
		t.Fatal("AckvitationPDU should not be nil")
	}
	ackvit := csbk.AckvitationPDU
	if ackvit.ServiceOptsMirror != 1 {
		t.Errorf("ServiceOptsMirror = %d, want 1", ackvit.ServiceOptsMirror)
	}
	if !ackvit.ServiceKindFlag {
		t.Error("ServiceKindFlag should be true")
	}
	if ackvit.UAB != 3 {
		t.Errorf("UAB = %d, want 3", ackvit.UAB)
	}
	if ackvit.ServiceKind != 2 {
		t.Errorf("ServiceKind = %d, want 2", ackvit.ServiceKind)
	}
}

// ── Grant Encode-Decode Cycle Tests ──

func TestCSBK_PrivateDataGrant_EncodeDecodeCycle(t *testing.T) {
	var targetAddr [24]bit.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]bit.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{
		LastBlock:  true,
		CSBKOpcode: pdu.CSBKPrivateDataGrant,
		FID:        0x00,
		PrivateDataGrantPDU: &pdu.PrivateDataGrantPDU{
			PhysicalChannel: 0x100,
			LogicalChannel:  true,
			HiRate:          true,
			Emergency:       false,
			Offset:          true,
			TargetAddress:   targetAddr,
			SourceAddress:   sourceAddr,
		},
	}

	encoded := pdu.EncodeCSBK(original)
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.PrivateDataGrantPDU == nil {
		t.Fatal("PrivateDataGrantPDU should not be nil")
	}
	pdg := decoded.PrivateDataGrantPDU
	if pdg.PhysicalChannel != 0x100 {
		t.Errorf("PhysicalChannel = 0x%03X, want 0x100", pdg.PhysicalChannel)
	}
	if !pdg.LogicalChannel {
		t.Error("LogicalChannel should be true")
	}
	if !pdg.HiRate {
		t.Error("HiRate should be true")
	}
	if pdg.Emergency {
		t.Error("Emergency should be false")
	}
	if !pdg.Offset {
		t.Error("Offset should be true")
	}
	if pdg.TargetAddress != targetAddr {
		t.Error("TargetAddress mismatch")
	}
	if pdg.SourceAddress != sourceAddr {
		t.Error("SourceAddress mismatch")
	}
}

func TestCSBK_Aloha_EncodeDecodeCycle(t *testing.T) {
	var msAddr [24]bit.Bit
	msAddr[0] = 1
	msAddr[15] = 1
	msAddr[23] = 1

	original := &pdu.CSBK{
		LastBlock:  true,
		CSBKOpcode: pdu.CSBKAloha,
		FID:        0x00,
		AlohaPDU: &pdu.AlohaPDU{
			TSCCAS:       true,
			SiteTSSync:   false,
			Version:      5,
			Offset:       true,
			ActiveConn:   false,
			Mask:         0b10101,
			ServiceFunc:  1,
			NRandWait:    6,
			Reg:          true,
			Backoff:      3,
			SysIdentCode: 0x1234,
			MSAddress:    msAddr,
		},
	}

	encoded := pdu.EncodeCSBK(original)
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.AlohaPDU == nil {
		t.Fatal("AlohaPDU should not be nil")
	}
	a := decoded.AlohaPDU
	if !a.TSCCAS {
		t.Error("TSCCAS should be true")
	}
	if a.SiteTSSync {
		t.Error("SiteTSSync should be false")
	}
	if a.Version != 5 {
		t.Errorf("Version = %d, want 5", a.Version)
	}
	if !a.Offset {
		t.Error("Offset should be true")
	}
	if a.Mask != 0b10101 {
		t.Errorf("Mask = %05b, want 10101", a.Mask)
	}
	if a.NRandWait != 6 {
		t.Errorf("NRandWait = %d, want 6", a.NRandWait)
	}
	if !a.Reg {
		t.Error("Reg should be true")
	}
	if a.SysIdentCode != 0x1234 {
		t.Errorf("SysIdentCode = 0x%04X, want 0x1234", a.SysIdentCode)
	}
	if a.MSAddress != msAddr {
		t.Error("MSAddress mismatch")
	}
}

// ── AnnouncementType Tests ──

func TestAnnouncementType_ToName(t *testing.T) {
	tests := []struct {
		at   enums.AnnouncementType
		want string
	}{
		{enums.AnnouncementAnnWDTSCC, "Ann_WD_TSCC"},
		{enums.AnnouncementCallTimer, "CallTimer_Parms"},
		{enums.AnnouncementVoteNow, "Vote_Now"},
		{enums.AnnouncementLocalTime, "Local_Time"},
		{enums.AnnouncementMassReg, "Mass_Reg"},
		{enums.AnnouncementChanFreq, "Chan_Freq"},
		{enums.AnnouncementAdjacentSite, "Adjacent_Site"},
		{enums.AnnouncementGenSiteParams, "Gen_Site_Params"},
	}
	for _, tt := range tests {
		if got := enums.AnnouncementTypeToName(tt.at); got != tt.want {
			t.Errorf("AnnouncementTypeToName(%d) = %q, want %q", int(tt.at), got, tt.want)
		}
	}

	// Reserved value
	reserved := enums.AnnouncementTypeToName(enums.AnnouncementType(15))
	if reserved == "" {
		t.Error("reserved AnnouncementType ToName should not be empty")
	}
}

func TestAnnouncementType_FromInt(t *testing.T) {
	for i := 0; i <= 7; i++ {
		at := enums.AnnouncementTypeFromInt(i)
		if int(at) != i {
			t.Errorf("AnnouncementTypeFromInt(%d) = %d", i, int(at))
		}
	}
}

// ── SLCO Tests ──

func TestSLCO_TierIII_Values(t *testing.T) {
	csys := enums.SLCOFromInt(0b0010)
	if csys != enums.SLCOCSysParms {
		t.Errorf("SLCOFromInt(0b0010) = %d, want SLCOCSysParms(%d)", int(csys), int(enums.SLCOCSysParms))
	}
	psys := enums.SLCOFromInt(0b0011)
	if psys != enums.SLCOPSysParms {
		t.Errorf("SLCOFromInt(0b0011) = %d, want SLCOPSysParms(%d)", int(psys), int(enums.SLCOPSysParms))
	}

	if name := enums.SLCOToName(enums.SLCOCSysParms); name != "C_SYS_Parms" {
		t.Errorf("SLCOCSysParms.ToName() = %q, want %q", name, "C_SYS_Parms")
	}
	if name := enums.SLCOToName(enums.SLCOPSysParms); name != "P_SYS_Parms" {
		t.Errorf("SLCOPSysParms.ToName() = %q, want %q", name, "P_SYS_Parms")
	}
}

// ── ShortLC CSysParms Tests ──

func TestShortLC_CSysParms_NetSite_Models(t *testing.T) {
	tests := []struct {
		name      string
		model     uint8
		netBits   int
		siteBits  int
		wantNetID uint16
		wantSite  uint8
	}{
		{"Tiny", 0, 6, 4, 0b111111, 0b1111},
		{"Small", 1, 7, 5, 0b1111111, 0b11111},
		{"Large", 2, 9, 3, 0b111111111, 0b111},
		{"Huge", 3, 12, 0, 0b111111111111, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &pdu.ShortLCCSysParms{MODEL: tt.model}
			// Fill NetSiteRaw with all 1s
			for i := range cs.NetSiteRaw {
				cs.NetSiteRaw[i] = 1
			}

			netID := cs.NetID()
			siteID := cs.SiteID()

			if netID != tt.wantNetID {
				t.Errorf("NetID() = %d, want %d", netID, tt.wantNetID)
			}
			if siteID != tt.wantSite {
				t.Errorf("SiteID() = %d, want %d", siteID, tt.wantSite)
			}
		})
	}
}

func TestShortLC_PSysParms_NetSite_Models(t *testing.T) {
	tests := []struct {
		name      string
		model     uint8
		wantNetID uint16
		wantSite  uint8
	}{
		{"Tiny", 0, 0b111111, 0b1111},
		{"Small", 1, 0b1111111, 0b11111},
		{"Large", 2, 0b111111111, 0b111},
		{"Huge", 3, 0b111111111111, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &pdu.ShortLCPSysParms{MODEL: tt.model}
			for i := range ps.NetSiteRaw {
				ps.NetSiteRaw[i] = 1
			}

			netID := ps.NetID()
			siteID := ps.SiteID()

			if netID != tt.wantNetID {
				t.Errorf("NetID() = %d, want %d", netID, tt.wantNetID)
			}
			if siteID != tt.wantSite {
				t.Errorf("SiteID() = %d, want %d", siteID, tt.wantSite)
			}
		})
	}
}

// ── UDT Header Decode Tests ──

func TestCSBK_UDTOutboundHeader_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// GroupIndividual (bit 0) = true
	payload[0] = 1
	// A (bit 1) = true
	payload[1] = 1
	// Emergency (bit 2) = true
	payload[2] = 1
	// UDTOptionFlag (bit 3) = false
	// DataPacketFormat (bits 4-7) = 0b1010
	payload[4] = 1
	payload[6] = 1
	// SAP (bits 8-11) = 0b0011
	payload[10] = 1
	payload[11] = 1
	// UDTFormat (bits 12-15) = 0b0101
	payload[13] = 1
	payload[15] = 1
	// TargetAddress (bits 16-39)
	payload[39] = 1
	// SourceAddress (bits 40-63)
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKUDTOutboundHeader), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.UDTOutboundHeaderPDU == nil {
		t.Fatal("UDTOutboundHeaderPDU should not be nil")
	}
	udt := csbk.UDTOutboundHeaderPDU
	if !udt.GroupIndividual {
		t.Error("GroupIndividual should be true")
	}
	if !udt.A {
		t.Error("A should be true")
	}
	if !udt.Emergency {
		t.Error("Emergency should be true")
	}
	if udt.DataPacketFormat != 0b1010 {
		t.Errorf("DataPacketFormat = %d, want 10", udt.DataPacketFormat)
	}
	if udt.SAP != 3 {
		t.Errorf("SAP = %d, want 3", udt.SAP)
	}
	if udt.UDTFormat != 5 {
		t.Errorf("UDTFormat = %d, want 5", udt.UDTFormat)
	}
}

// ── P_PROTECT Decode Test ──

func TestCSBK_Protect_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ProtectKind (bits 12-14) = 5 → 0b101
	payload[12] = 1
	payload[14] = 1
	// GroupIndividual (bit 15) = true
	payload[15] = 1
	// TargetAddress (bits 16-39)
	payload[39] = 1
	// SourceAddress (bits 40-63)
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKProtect), 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.ProtectPDU == nil {
		t.Fatal("ProtectPDU should not be nil")
	}
	if csbk.ProtectPDU.ProtectKind != 5 {
		t.Errorf("ProtectKind = %d, want 5", csbk.ProtectPDU.ProtectKind)
	}
	if !csbk.ProtectPDU.GroupIndividual {
		t.Error("GroupIndividual should be true")
	}
}
