package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCSBK_TierIII_OpcodeString(t *testing.T) {
	t.Parallel()

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

func TestCSBK_PrivateVoiceGrant_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 3, 1)
	bits.SetBit(payload[:], 7, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 11, 1)
	bits.SetBit(payload[:], 12, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKPrivateVoiceGrant), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKPrivateVoiceGrant {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKPrivateVoiceGrant))
	}
	pvg, ok := csbk.AsPrivateVoiceGrant()
	if !ok {
		t.Fatal("AsPrivateVoiceGrant is not ok")
	}
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
	t.Parallel()

	var targetAddr [24]unpacked.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]unpacked.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{LastBlock: true}
	if err := original.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 0x0FF,
		LogicalChannel:  true,
		LateEntry:       false,
		Emergency:       true,
		Offset:          false,
		TargetAddress:   uint32(unpacked.Uint(targetAddr[:])),
		SourceAddress:   uint32(unpacked.Uint(sourceAddr[:])),
	}); err != nil {
		t.Fatal(err)
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	tvg, ok := decoded.AsTalkgroupVoiceGrant()
	if !ok {
		t.Fatal("AsTalkgroupVoiceGrant is not ok")
	}
	if tvg.PhysicalChannel != 0x0FF {
		t.Errorf("PhysicalChannel = 0x%03X, want 0x0FF", tvg.PhysicalChannel)
	}
	if !tvg.LogicalChannel {
		t.Error("LogicalChannel should be true")
	}
	if !tvg.Emergency {
		t.Error("Emergency should be true")
	}
	if tvg.TargetAddress != uint32(unpacked.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if tvg.SourceAddress != uint32(unpacked.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_Aloha_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 1, 1)
	bits.SetBit(payload[:], 2, 1)
	bits.SetBit(payload[:], 4, 1)
	bits.SetBit(payload[:], 5, 1)
	bits.SetBit(payload[:], 6, 1)
	bits.SetBit(payload[:], 8, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 12, 1)
	bits.SetBit(payload[:], 13, 1)
	bits.SetBit(payload[:], 16, 1)
	bits.SetBit(payload[:], 18, 1)
	bits.SetBit(payload[:], 19, 1)
	bits.SetBit(payload[:], 22, 1)
	bits.SetBit(payload[:], 23, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKAloha), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKAloha {
		t.Errorf("CSBKOpcode = %08b, want C_ALOHA", byte(csbk.Opcode))
	}
	aloha, ok := csbk.AsAloha()
	if !ok {
		t.Fatal("AsAloha is not ok")
	}
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

func TestCSBK_CBroadcast_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 2, 1)
	bits.SetBit(payload[:], 4, 1)
	bits.SetBit(payload[:], 5, 1)
	bits.SetBit(payload[:], 19, 1)
	bits.SetBit(payload[:], 21, 1)
	bits.SetBit(payload[:], 22, 1)
	bits.SetBit(payload[:], 23, 1)
	bits.SetBit(payload[:], 27, 1)
	bits.SetBit(payload[:], 30, 1)
	bits.SetBit(payload[:], 33, 1)
	bits.SetBit(payload[:], 34, 1)
	bits.SetBit(payload[:], 37, 1)
	bits.SetBit(payload[:], 40, 1)

	infoBits := buildCSBK(byte(pdu.CSBKBroadcast), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %08b, want C_BCAST", byte(csbk.Opcode))
	}
	bc, ok := csbk.AsBroadcast()
	if !ok {
		t.Fatal("AsBroadcast is not ok")
	}
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

func TestCSBK_Clear_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 3, 1)
	bits.SetBit(payload[:], 15, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKClear), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	clr, ok := csbk.AsClear()
	if !ok {
		t.Fatal("AsClear is not ok")
	}
	if clr.PhysicalChannel != 0x100 {
		t.Errorf("PhysicalChannel = 0x%03X, want 0x100", clr.PhysicalChannel)
	}
	if !clr.Group {
		t.Error("Group should be true")
	}
}

func TestCSBK_Ahoy_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 0, 1)
	bits.SetBit(payload[:], 5, 1)
	bits.SetBit(payload[:], 7, 1)
	bits.SetBit(payload[:], 8, 1)
	bits.SetBit(payload[:], 9, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 13, 1)
	bits.SetBit(payload[:], 15, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKAhoy), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	ahoy, ok := csbk.AsAhoy()
	if !ok {
		t.Fatal("AsAhoy is not ok")
	}
	if ahoy.ServiceOptsMirror != 0x42 {
		t.Errorf("ServiceOptsMirror = 0x%02X, want 0x42", ahoy.ServiceOptsMirror)
	}
	if !ahoy.ServiceKindFlag {
		t.Error("ServiceKindFlag should be true")
	}
	if !ahoy.ALS {
		t.Error("ALS should be true")
	}
	if !ahoy.Group {
		t.Error("Group should be true")
	}
	if ahoy.UAB != 2 {
		t.Errorf("AppendedBlocks = %d, want 2", ahoy.UAB)
	}
	if ahoy.ServiceKind != 5 {
		t.Errorf("ServiceKind = %d, want 5", ahoy.ServiceKind)
	}
}

func TestCSBK_RandomAccess_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 6, 1)
	bits.SetBit(payload[:], 7, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 15, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKRandomAccess), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	rand, ok := csbk.AsRandomAccess()
	if !ok {
		t.Fatal("AsRandomAccess is not ok")
	}
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

func TestCSBK_Opcode0x38_BSOutbound(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(0b00111000, payload)

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
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 3, 1)
	bits.SetBit(payload[:], 12, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(0b00111000, payload)

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
	csbk.RawPayload = must(pdu.EncodePayload(&grant))
	if must(csbk.Encode()) != infoBits {
		t.Error("RawPayload from the grant Encode does not round trip")
	}
}

func TestCSBK_AckOutbound_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 2, 1)
	bits.SetBit(payload[:], 8, 1)
	bits.SetBit(payload[:], 13, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKAckOutbound), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	_, ok := csbk.AsAckOutbound()
	if !ok {
		t.Fatal("AsAckOutbound is not ok")
	}
}

func TestCSBK_Move_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 9, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 11, 1)
	bits.SetBit(payload[:], 12, 1)
	bits.SetBit(payload[:], 13, 1)
	bits.SetBit(payload[:], 19, 1)
	bits.SetBit(payload[:], 21, 1)
	bits.SetBit(payload[:], 23, 1)
	for i := 28; i <= 39; i++ {
		bits.SetBit(payload[:], i, 1)
	}
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKMove), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	move, ok := csbk.AsMove()
	if !ok {
		t.Fatal("AsMove is not ok")
	}
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

func TestCSBK_Maintenance_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 13, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKMaintenance), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	maintenance, ok := csbk.AsMaintenance()
	if !ok {
		t.Fatal("AsMaintenance is not ok")
	}
	if maintenance.MaintKind != 3 {
		t.Errorf("MaintKind = %d, want 3", maintenance.MaintKind)
	}
}

func TestCSBK_Ackvitation_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 6, 1)
	bits.SetBit(payload[:], 7, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 11, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKAckvitation), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	ackvit, ok := csbk.AsAckvitation()
	if !ok {
		t.Fatal("AsAckvitation is not ok")
	}
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

func TestCSBK_PrivateDataGrant_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	var targetAddr [24]unpacked.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]unpacked.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{LastBlock: true}
	if err := original.SetPrivateDataGrant(pdu.PrivateDataGrantPDU{
		PhysicalChannel: 0x100,
		LogicalChannel:  true,
		HiRate:          true,
		Emergency:       false,
		Offset:          true,
		TargetAddress:   uint32(unpacked.Uint(targetAddr[:])),
		SourceAddress:   uint32(unpacked.Uint(sourceAddr[:])),
	}); err != nil {
		t.Fatal(err)
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	pdg, ok := decoded.AsPrivateDataGrant()
	if !ok {
		t.Fatal("AsPrivateDataGrant is not ok")
	}
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
	if pdg.TargetAddress != uint32(unpacked.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch")
	}
	if pdg.SourceAddress != uint32(unpacked.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch")
	}
}

func TestCSBK_Aloha_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	var msAddr [24]unpacked.Bit
	msAddr[0] = 1
	msAddr[15] = 1
	msAddr[23] = 1

	original := &pdu.CSBK{LastBlock: true}
	if err := original.SetAloha(pdu.AlohaPDU{
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
		MSAddress:    uint32(unpacked.Uint(msAddr[:])),
	}); err != nil {
		t.Fatal(err)
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	a, ok := decoded.AsAloha()
	if !ok {
		t.Fatal("AsAloha is not ok")
	}
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
	if a.MSAddress != uint32(unpacked.Uint(msAddr[:])) {
		t.Error("MSAddress mismatch")
	}
}

func TestAnnouncementType_String(t *testing.T) {
	t.Parallel()

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

	reserved := (enums.AnnouncementType(15)).String()
	if reserved == "" {
		t.Error("reserved AnnouncementType ToName should not be empty")
	}
}

func TestAnnouncementType_Cast(t *testing.T) {
	t.Parallel()

	for i := 0; i <= 7; i++ {
		at := enums.AnnouncementType(i)
		if int(at) != i {
			t.Errorf("AnnouncementType(%d) = %d", i, int(at))
		}
	}
}

func TestSLCO_TierIII_Values(t *testing.T) {
	t.Parallel()

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

type netSiteModelCase struct {
	name      string
	model     uint8
	wantNetID uint16
	wantSite  uint16
}

func netSitePattern() uint16 {
	return 0b1100_1010_0101
}

// ETSI TS 102 361-4 Table 6.5
func netSiteModelCases() []netSiteModelCase {
	return []netSiteModelCase{
		{"Tiny", 0, 0b110010100, 0b101},
		{"Small", 1, 0b1100101, 0b00101},
		{"Large", 2, 0b1100, 0b10100101},
		{"Huge", 3, 0b11, 0b0010100101},
	}
}

func TestShortLC_CSysParms_NetSite_Models(t *testing.T) {
	t.Parallel()

	for _, tt := range netSiteModelCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cs := &pdu.ShortLCCSysParms{Model: pdu.NetworkModel(tt.model), NetSiteRaw: netSitePattern()}
			if got := must(cs.NetID()); got != tt.wantNetID {
				t.Errorf("NetID() = %b, want %b", got, tt.wantNetID)
			}
			if got := must(cs.SiteID()); got != tt.wantSite {
				t.Errorf("SiteID() = %b, want %b", got, tt.wantSite)
			}
		})
	}
}

func TestShortLC_PSysParms_NetSite_Models(t *testing.T) {
	t.Parallel()

	for _, tt := range netSiteModelCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ps := &pdu.ShortLCPSysParms{Model: pdu.NetworkModel(tt.model), NetSiteRaw: netSitePattern()}
			if got := must(ps.NetID()); got != tt.wantNetID {
				t.Errorf("NetID() = %b, want %b", got, tt.wantNetID)
			}
			if got := must(ps.SiteID()); got != tt.wantSite {
				t.Errorf("SiteID() = %b, want %b", got, tt.wantSite)
			}
		})
	}
}

// ETSI TS 102 361-4 Table 7.2
func TestCSBK_UDTHeaderOpcodes_NotCSBK(t *testing.T) {
	t.Parallel()

	for _, op := range []byte{0b011010, 0b011011} {
		var payload [8]byte
		bits.SetBit(payload[:], 0, 1)
		csbk := pdu.DecodeCSBK(buildCSBK(op, payload))
		fecResult := csbk.FEC
		if fecResult.Uncorrectable {
			t.Fatal("DecodeCSBK returned uncorrectable FEC")
		}
		if got := csbk.Opcode.String(); !strings.HasPrefix(got, "Unknown CSBKOpcode") {
			t.Errorf("opcode %06b String() = %q, want Unknown CSBKOpcode", op, got)
		}
	}
}

func TestCSBK_Protect_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 12, 1)
	bits.SetBit(payload[:], 14, 1)
	bits.SetBit(payload[:], 15, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKProtect), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	protect, ok := csbk.AsProtect()
	if !ok {
		t.Fatal("AsProtect is not ok")
	}
	if protect.ProtectKind != 5 {
		t.Errorf("ProtectKind = %d, want 5", protect.ProtectKind)
	}
	if !protect.Group {
		t.Error("Group should be true")
	}
}
