package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ── Tier III CSBK Opcode ToString Tests ──

func TestCSBK_TierIII_OpcodeString(t *testing.T) {
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
		{pdu.CSBKAhoy, "C_AHOY/P_AHOY PDU"},
		{pdu.CSBKAckOutbound, "C_ACKD PDU"},
		{pdu.CSBKAckInbound, "C_ACKU PDU"},
		{pdu.CSBKAckOutboundPayload, "P_ACKD PDU"},
		{pdu.CSBKAckInboundPayload, "P_ACKU PDU"},
		{pdu.CSBKRandomAccess, "C_RAND PDU"},
		{pdu.CSBKAckvitation, "C_ACKVIT PDU"},
		{pdu.CSBKMaintenance, "P_MAINT PDU"},
	}
	for _, tt := range tests {
		if got := tt.opcode.String(); got != tt.want {
			t.Errorf("CSBKOpcode(%08b).String() = %q, want %q", byte(tt.opcode), got, tt.want)
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

	infoBits := buildCSBKBits(byte(pdu.CSBKPrivateVoiceGrant), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKPrivateVoiceGrant {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKPrivateVoiceGrant))
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
	if ((pvg.TargetAddress >> 0) & 1) != 1 {
		t.Error("TargetAddress last bit should be 1")
	}
	if ((pvg.SourceAddress >> 0) & 1) != 1 {
		t.Error("SourceAddress last bit should be 1")
	}

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_TalkgroupVoiceGrant_EncodeDecodeCycle(t *testing.T) {
	var targetAddr [24]bit.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]bit.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{
		LastBlock: true,
		Opcode:    pdu.CSBKTalkgroupVoiceGrant,
		FID:       0x00,
		TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
			PhysicalChannel: 0x0FF,
			LogicalChannel:  true,
			LateEntry:       false,
			Emergency:       true,
			Offset:          false,
			TargetAddress:   uint32(bit.Uint(targetAddr[:])),
			SourceAddress:   uint32(bit.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
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
	if tvg.TargetAddress != uint32(bit.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if tvg.SourceAddress != uint32(bit.Uint(sourceAddr[:])) {
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

	infoBits := buildCSBKBits(byte(pdu.CSBKAloha), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKAloha {
		t.Errorf("CSBKOpcode = %08b, want C_ALOHA", byte(csbk.Opcode))
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

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
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

	infoBits := buildCSBKBits(byte(pdu.CSBKBroadcast), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %08b, want C_BCAST", byte(csbk.Opcode))
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

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
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

	infoBits := buildCSBKBits(byte(pdu.CSBKClear), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	infoBits := buildCSBKBits(byte(pdu.CSBKAhoy), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	infoBits := buildCSBKBits(byte(pdu.CSBKRandomAccess), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

// ── Opcode 0x38 ──

func TestCSBK_Opcode0x38_BSOutbound(t *testing.T) {
	var payload [64]bit.Bit
	payload[39] = 1 // BSAddress last bit
	payload[63] = 1 // SourceAddress last bit

	infoBits := buildCSBKBits(0b00111000, payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.RawPayload != payload {
		t.Fatal("opcode 0x38 payload is not in RawPayload")
	}
	act, ok := csbk.AsBSOutboundActivation()
	if !ok {
		t.Fatal("AsBSOutboundActivation of opcode 0x38 is not ok")
	}
	if ((act.BSAddress>>0)&1) != 1 || ((act.SourceAddress>>0)&1) != 1 {
		t.Error("BSAddress/SourceAddress last bit should be set")
	}
	if must(csbk.Encode()) != infoBits {
		t.Error("DecodeCSBK(x).Encode() != x")
	}
	other := pdu.CSBK{Opcode: pdu.CSBKAloha}
	if _, ok := other.AsBSOutboundActivation(); ok {
		t.Error("AsBSOutboundActivation of C_ALOHA is ok")
	}
}

func TestCSBK_Opcode0x38_TDGrantMI(t *testing.T) {
	// TD_GRANT_MI (ETSI TS 102 361-4 Table B.1).
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

	infoBits := buildCSBKBits(0b00111000, payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	grant, ok := csbk.AsTalkgroupDataGrantMultiItem()
	if !ok {
		t.Fatal("AsTalkgroupDataGrantMultiItem of opcode 0x38 is not ok")
	}
	if grant.PhysicalChannel != 0x100 {
		t.Errorf("PhysicalChannel = %#x, want 0x100", grant.PhysicalChannel)
	}
	if !grant.LogicalChannel {
		t.Error("LogicalChannel should be true")
	}
	if grant.HiRate {
		t.Error("HiRate should be false")
	}
	if !grant.Emergency {
		t.Error("Emergency should be true")
	}
	if grant.Offset {
		t.Error("Offset should be false")
	}
	if ((grant.TargetAddress>>0)&1) != 1 || ((grant.SourceAddress>>0)&1) != 1 {
		t.Error("TargetAddress/SourceAddress last bit should be set")
	}
	csbk.RawPayload = must(grant.Encode())
	if must(csbk.Encode()) != infoBits {
		t.Error("RawPayload from the grant Encode does not round trip")
	}
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

	infoBits := buildCSBKBits(byte(pdu.CSBKAckOutbound), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	infoBits := buildCSBKBits(byte(pdu.CSBKMove), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	infoBits := buildCSBKBits(byte(pdu.CSBKMaintenance), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	infoBits := buildCSBKBits(byte(pdu.CSBKAckvitation), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
		LastBlock: true,
		Opcode:    pdu.CSBKPrivateDataGrant,
		FID:       0x00,
		PrivateDataGrantPDU: &pdu.PrivateDataGrantPDU{
			PhysicalChannel: 0x100,
			LogicalChannel:  true,
			HiRate:          true,
			Emergency:       false,
			Offset:          true,
			TargetAddress:   uint32(bit.Uint(targetAddr[:])),
			SourceAddress:   uint32(bit.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
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
	if pdg.TargetAddress != uint32(bit.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch")
	}
	if pdg.SourceAddress != uint32(bit.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch")
	}
}

func TestCSBK_Aloha_EncodeDecodeCycle(t *testing.T) {
	var msAddr [24]bit.Bit
	msAddr[0] = 1
	msAddr[15] = 1
	msAddr[23] = 1

	original := &pdu.CSBK{
		LastBlock: true,
		Opcode:    pdu.CSBKAloha,
		FID:       0x00,
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
			MSAddress:    uint32(bit.Uint(msAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
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
	if a.MSAddress != uint32(bit.Uint(msAddr[:])) {
		t.Error("MSAddress mismatch")
	}
}

// ── AnnouncementType Tests ──

func TestAnnouncementType_String(t *testing.T) {
	tests := []struct {
		at   enums.AnnouncementType
		want string
	}{
		{enums.AnnouncementAnnWDTSCC, "Ann_WD_TSCC"},
		{enums.AnnouncementCallTimer, "CallTimer_Parms"},
		{enums.AnnouncementVoteNow, "Vote_Now"},
		{enums.AnnouncementLocalTime, "Local_Time"},
		{enums.AnnouncementMassReg, "MassReg"},
		{enums.AnnouncementChanFreq, "Chan_Freq"},
		{enums.AnnouncementAdjacentSite, "Adjacent_Site"},
		{enums.AnnouncementGenSiteParams, "Gen_Site_Params"},
		{enums.AnnouncementManufacturerSpecific1, "Manufacturer Specific"},
		{enums.AnnouncementManufacturerSpecific2, "Manufacturer Specific"},
	}
	for _, tt := range tests {
		if got := tt.at.String(); got != tt.want {
			t.Errorf("(%d).String() = %q, want %q", int(tt.at), got, tt.want)
		}
	}

	// Reserved value
	reserved := (enums.AnnouncementType(15)).String()
	if reserved == "" {
		t.Error("reserved AnnouncementType ToName should not be empty")
	}
}

func TestAnnouncementType_Cast(t *testing.T) {
	for i := 0; i <= 7; i++ {
		at := enums.AnnouncementType(i)
		if int(at) != i {
			t.Errorf("AnnouncementType(%d) = %d", i, int(at))
		}
	}
}

// ── SLCO Tests ──

func TestSLCO_TierIII_Values(t *testing.T) {
	csys := enums.SLCO(0b0010)
	if csys != enums.SLCOCSysParms {
		t.Errorf("SLCO(0b0010) = %d, want SLCOCSysParms(%d)", int(csys), int(enums.SLCOCSysParms))
	}
	psys := enums.SLCO(0b0011)
	if psys != enums.SLCOPSysParms {
		t.Errorf("SLCO(0b0011) = %d, want SLCOPSysParms(%d)", int(psys), int(enums.SLCOPSysParms))
	}

	if name := enums.SLCOCSysParms.String(); name != "C_SYS_Parms" {
		t.Errorf("SLCOCSysParms.ToName() = %q, want %q", name, "C_SYS_Parms")
	}
	if name := enums.SLCOPSysParms.String(); name != "P_SYS_Parms" {
		t.Errorf("SLCOPSysParms.ToName() = %q, want %q", name, "P_SYS_Parms")
	}
}

// ── ShortLC CSysParms Tests ──

type netSiteModelCase struct {
	name      string
	model     uint8
	wantNetID uint16
	wantSite  uint16
}

// netSitePattern is 0b1100_1010_0101 in NetSiteRaw order.
func netSitePattern() uint16 {
	return 0b1100_1010_0101
}

// netSiteModelCases follows ETSI TS 102 361-4 Table 6.5.
func netSiteModelCases() []netSiteModelCase {
	return []netSiteModelCase{
		{"Tiny", 0, 0b110010100, 0b101},
		{"Small", 1, 0b1100101, 0b00101},
		{"Large", 2, 0b1100, 0b10100101},
		{"Huge", 3, 0b11, 0b0010100101},
	}
}

func TestShortLC_CSysParms_NetSite_Models(t *testing.T) {
	for _, tt := range netSiteModelCases() {
		t.Run(tt.name, func(t *testing.T) {
			cs := &pdu.ShortLCCSysParms{MODEL: tt.model, NetSiteRaw: netSitePattern()}
			if got := cs.NetID(); got != tt.wantNetID {
				t.Errorf("NetID() = %b, want %b", got, tt.wantNetID)
			}
			if got := cs.SiteID(); got != tt.wantSite {
				t.Errorf("SiteID() = %b, want %b", got, tt.wantSite)
			}
		})
	}
}

func TestShortLC_PSysParms_NetSite_Models(t *testing.T) {
	for _, tt := range netSiteModelCases() {
		t.Run(tt.name, func(t *testing.T) {
			ps := &pdu.ShortLCPSysParms{MODEL: tt.model, NetSiteRaw: netSitePattern()}
			if got := ps.NetID(); got != tt.wantNetID {
				t.Errorf("NetID() = %b, want %b", got, tt.wantNetID)
			}
			if got := ps.SiteID(); got != tt.wantSite {
				t.Errorf("SiteID() = %b, want %b", got, tt.wantSite)
			}
		})
	}
}

// ── UDT Header opcodes are not CSBK opcodes ──

// TS 102 361-4 Table 7.2 carries C_UDTHD/C_UDTHU in a Data Header burst,
// so opcodes 0x1A/0x1B must not decode as a CSBK variant.
func TestCSBK_UDTHeaderOpcodes_NotCSBK(t *testing.T) {
	for _, op := range []byte{0b011010, 0b011011} {
		var payload [64]bit.Bit
		payload[0] = 1
		csbk := pdu.DecodeCSBK(buildCSBKBits(op, payload))
		fecResult := csbk.FEC
		if fecResult.Uncorrectable {
			t.Fatal("DecodeCSBK returned uncorrectable FEC")
		}
		if got := csbk.Opcode.String(); !strings.HasPrefix(got, "Unknown CSBKOpcode") {
			t.Errorf("opcode %06b String() = %q, want Unknown CSBKOpcode", op, got)
		}
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

	infoBits := buildCSBKBits(byte(pdu.CSBKProtect), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
