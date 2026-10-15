package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// buildCSBKBits constructs a [96]bit.Bit array for a CSBK PDU.
// opcode is 6 bits, FID is 0, payload is 64 bits.
// lb and pf flags default to true/false.
// The CRC-CCITT is computed and XOR-masked per spec (§9.1.7).
func buildCSBKBits(opcode byte, payload [64]bit.Bit) [96]bit.Bit {
	var dataBytes [12]byte

	// Byte 0: lb=1, pf=0, opcode(6 bits)
	dataBytes[0] = 0x80 | (opcode & 0x3F) // lb=1, pf=0, 6-bit opcode

	// Pack 64 payload bits into bytes 2..9
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			dataBytes[2+i] <<= 1
			dataBytes[2+i] |= byte(payload[i*8+j])
		}
	}

	// Compute CRC over bytes 0..9
	crc := crc.CCITT(dataBytes[:10])
	dataBytes[10] = byte(crc >> 8)
	dataBytes[11] = byte(crc)

	// Apply XOR mask
	dataBytes[10] ^= 0xA5
	dataBytes[11] ^= 0xA5

	// Unpack into 96 bits
	var bits [96]bit.Bit
	for i := 0; i < 12; i++ {
		for j := 0; j < 8; j++ {
			if (dataBytes[i]>>(7-j))&1 == 1 {
				bits[i*8+j] = 1
			}
		}
	}

	return bits
}

func TestCSBK_OpcodeString(t *testing.T) {
	tests := []struct {
		opcode pdu.CSBKOpcode
		want   string
	}{
		{pdu.CSBKBSOutboundActivation, "BS Outbound Activation PDU or TD_GRANT_MI PDU"},
		{pdu.CSBKUnitToUnitVoiceServiceRequest, "Unit-to-Unit Voice Service Request PDU"},
		{pdu.CSBKUnitToUnitVoiceServiceAnswerResponse, "Unit-to-Unit Voice Service Answer/Response PDU"},
		{pdu.CSBKNegativeAcknowledgement, "Negative Acknowledgement PDU"},
		{pdu.CSBKPreamble, "Preamble PDU"},
		{pdu.CSBKChannelTiming, "Channel Timing PDU"},
	}
	for _, tt := range tests {
		if got := tt.opcode.String(); got != tt.want {
			t.Errorf("CSBKOpcode(%08b).String() = %q, want %q", byte(tt.opcode), got, tt.want)
		}
	}

	// Unknown opcode should contain "Unknown"
	unknown := pdu.CSBKOpcode(0xFF)
	if s := unknown.String(); s == "" {
		t.Error("unknown opcode String() should not be empty")
	}
}

func TestCSBK_PreamblePDU_Decode(t *testing.T) {
	// Preamble opcode = 0b00111101
	var payload [64]bit.Bit
	// Data=1, Group=0 (bits 0,1)
	payload[0] = 1
	payload[1] = 0
	// Reserved (bits 2-7) = 0
	// CSBKBlocksToFollow = 5 (bits 8-15)
	payload[8+5] = 1 // bit 13 → value 4
	payload[8+7] = 1 // bit 15 → value 1 → total = 5
	// TargetAddress (bits 16-39) = 0x000001 → last bit set
	payload[39] = 1
	// SourceAddress (bits 40-63) = 0x000002 → second to last bit set
	payload[62] = 1

	infoBits := buildCSBKBits(0b00111101, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKPreamble {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKPreamble))
	}
	if csbk.PreamblePDU == nil {
		t.Fatal("PreamblePDU should not be nil")
	}
	if !csbk.PreamblePDU.Data {
		t.Error("PreamblePDU.Data should be true")
	}
	if csbk.PreamblePDU.Group {
		t.Error("PreamblePDU.Group should be false")
	}
	if csbk.PreamblePDU.CSBKBlocksToFollow != 5 {
		t.Errorf("CSBKBlocksToFollow = %d, want 5", csbk.PreamblePDU.CSBKBlocksToFollow)
	}
	if !csbk.LastBlock {
		t.Error("LastBlock should be true")
	}

	// Verify String doesn't panic
	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_NegativeAck_Decode(t *testing.T) {
	// NegativeAck opcode = 0b00100110
	var payload [64]bit.Bit
	// AdditionalInfo=1 (bit 0)
	payload[0] = 1
	// SourceType=0 (bit 1)
	// ServiceType (bits 2-7) = 0b000011 = 3
	payload[6] = 1
	payload[7] = 1
	// ReasonCode (bits 8-15) = 0x42
	payload[9] = 1 // 0b01000010
	payload[14] = 1
	// SourceAddress (bits 16-39) — leave zero
	// TargetAddress (bits 40-63) — leave zero

	infoBits := buildCSBKBits(0b00100110, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKNegativeAcknowledgement {
		t.Errorf("CSBKOpcode = %08b, want NegativeAck", byte(csbk.Opcode))
	}
	if csbk.NegativeAcknowledgementPDU == nil {
		t.Fatal("NegativeAcknowledgementPDU should not be nil")
	}
	if csbk.NegativeAcknowledgementPDU.AdditionalInfo != layer3.AdditionalInfoValid {
		t.Error("AdditionalInfo should be Valid")
	}
	if csbk.NegativeAcknowledgementPDU.ServiceType != pdu.CSBKOpcode(3) {
		t.Errorf("ServiceType = %d, want 3", csbk.NegativeAcknowledgementPDU.ServiceType)
	}
	if csbk.NegativeAcknowledgementPDU.ReasonCode != 0x42 {
		t.Errorf("ReasonCode = 0x%02X, want 0x42", csbk.NegativeAcknowledgementPDU.ReasonCode)
	}
}

func TestCSBK_CRCValidation(t *testing.T) {
	// Build valid CSBK bits, then corrupt them to verify CRC catches it
	var payload [64]bit.Bit
	payload[0] = 1
	infoBits := buildCSBKBits(0b00111101, payload)

	// First verify it decodes ok
	_, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("valid CSBK should decode ok")
	}

	// Now corrupt a bit in the middle and verify CRC fails
	var corrupted [96]bit.Bit
	copy(corrupted[:], infoBits[:])
	corrupted[40] ^= 1 // flip a payload bit

	_, fecResult2 := pdu.DecodeCSBK(corrupted)
	if !fecResult2.Uncorrectable {
		t.Error("corrupted CSBK should fail CRC check")
	}
}

func TestCSBK_BSOutboundActivation_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// Reserved (bits 0-15) = 0
	// BSAddress (bits 16-39) = set last bit for address 1
	payload[39] = 1
	// SourceAddress (bits 40-63) = set last two bits for address 3
	payload[62] = 1
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKBSOutboundActivation), payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKBSOutboundActivation {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKBSOutboundActivation))
	}
	csbkAct, ok := csbk.AsBSOutboundActivation()
	if !ok {
		t.Fatal("BSOutboundActivationPDU should not be nil")
	}
	if ((csbkAct.BSAddress >> 0) & 1) != 1 {
		t.Error("BSAddress last bit should be 1")
	}
	if ((csbkAct.SourceAddress>>1)&1) != 1 || ((csbkAct.SourceAddress>>0)&1) != 1 {
		t.Error("SourceAddress last two bits should be 1")
	}

	// Verify String doesn't panic
	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_BSOutboundActivation_EncodeDecodeCycle(t *testing.T) {
	var bsAddr [24]bit.Bit
	bsAddr[0] = 1
	bsAddr[23] = 1
	var srcAddr [24]bit.Bit
	srcAddr[12] = 1

	original := &pdu.CSBK{
		LastBlock:   true,
		ProtectFlag: false,
		Opcode:      pdu.CSBKBSOutboundActivation,
		FID:         0x00,
		RawPayload: must((&pdu.BSOutboundActivationPDU{
			Reserved:      0,
			BSAddress:     uint32(bit.Uint(bsAddr[:])),
			SourceAddress: uint32(bit.Uint(srcAddr[:])),
		}).Encode()),
	}

	encoded := must(original.Encode())
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	decodedAct, ok := decoded.AsBSOutboundActivation()
	if !ok {
		t.Fatal("BSOutboundActivationPDU should not be nil")
	}
	if decodedAct.BSAddress != uint32(bit.Uint(bsAddr[:])) {
		t.Error("BSAddress mismatch after encode-decode cycle")
	}
	if decodedAct.SourceAddress != uint32(bit.Uint(srcAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_UnitToUnitVoiceServiceRequest_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ServiceOptions (bits 0-7) = 0x80 (emergency)
	payload[0] = 1
	// Reserved (bits 8-15) = 0
	// TargetAddress (bits 16-39) = address with bit 39 set
	payload[39] = 1
	// SourceAddress (bits 40-63) = address with bit 63 set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKUnitToUnitVoiceServiceRequest), payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKUnitToUnitVoiceServiceRequest {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKUnitToUnitVoiceServiceRequest))
	}
	if csbk.UnitToUnitVoiceServiceRequestPDU == nil {
		t.Fatal("UnitToUnitVoiceServiceRequestPDU should not be nil")
	}
	if csbk.UnitToUnitVoiceServiceRequestPDU.ServiceOptions != serviceOptions(0x80) {
		t.Errorf("ServiceOptions = %+v, want 0x80", csbk.UnitToUnitVoiceServiceRequestPDU.ServiceOptions)
	}

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_UnitToUnitVoiceServiceRequest_EncodeDecodeCycle(t *testing.T) {
	var targetAddr [24]bit.Bit
	targetAddr[0] = 1
	targetAddr[23] = 1
	var sourceAddr [24]bit.Bit
	sourceAddr[12] = 1

	original := &pdu.CSBK{
		LastBlock: true,
		Opcode:    pdu.CSBKUnitToUnitVoiceServiceRequest,
		FID:       0x00,
		UnitToUnitVoiceServiceRequestPDU: &pdu.UnitToUnitVoiceServiceRequestPDU{
			ServiceOptions: serviceOptions(0x42),
			Reserved:       0,
			TargetAddress:  uint32(bit.Uint(targetAddr[:])),
			SourceAddress:  uint32(bit.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU == nil {
		t.Fatal("UnitToUnitVoiceServiceRequestPDU should not be nil")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.ServiceOptions != serviceOptions(0x42) {
		t.Errorf("ServiceOptions = %+v, want 0x42", decoded.UnitToUnitVoiceServiceRequestPDU.ServiceOptions)
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.TargetAddress != uint32(bit.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.SourceAddress != uint32(bit.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_UnitToUnitVoiceServiceAnswerResponse_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// ServiceOptions (bits 0-7) = 0x40
	payload[1] = 1
	// AnswerResponse (bits 8-15) = 0x20
	payload[10] = 1
	// TargetAddress (bits 16-39) — bit 39 set
	payload[39] = 1
	// SourceAddress (bits 40-63) — bit 63 set
	payload[63] = 1

	infoBits := buildCSBKBits(byte(pdu.CSBKUnitToUnitVoiceServiceAnswerResponse), payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKUnitToUnitVoiceServiceAnswerResponse {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKUnitToUnitVoiceServiceAnswerResponse))
	}
	if csbk.UnitToUnitVoiceServiceAnswerResponsePDU == nil {
		t.Fatal("UnitToUnitVoiceServiceAnswerResponsePDU should not be nil")
	}
	if csbk.UnitToUnitVoiceServiceAnswerResponsePDU.ServiceOptions != serviceOptions(0x40) {
		t.Errorf("ServiceOptions = %+v, want 0x40", csbk.UnitToUnitVoiceServiceAnswerResponsePDU.ServiceOptions)
	}
	if csbk.UnitToUnitVoiceServiceAnswerResponsePDU.AnswerResponse != 0x20 {
		t.Errorf("AnswerResponse = 0x%02X, want 0x20", csbk.UnitToUnitVoiceServiceAnswerResponsePDU.AnswerResponse)
	}

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_UnitToUnitVoiceServiceAnswerResponse_EncodeDecodeCycle(t *testing.T) {
	var targetAddr [24]bit.Bit
	targetAddr[5] = 1
	targetAddr[10] = 1
	var sourceAddr [24]bit.Bit
	sourceAddr[0] = 1
	sourceAddr[23] = 1

	original := &pdu.CSBK{
		LastBlock: true,
		Opcode:    pdu.CSBKUnitToUnitVoiceServiceAnswerResponse,
		FID:       0x00,
		UnitToUnitVoiceServiceAnswerResponsePDU: &pdu.UnitToUnitVoiceServiceAnswerResponsePDU{
			ServiceOptions: serviceOptions(0x81),
			AnswerResponse: 0x03,
			TargetAddress:  uint32(bit.Uint(targetAddr[:])),
			SourceAddress:  uint32(bit.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU == nil {
		t.Fatal("UnitToUnitVoiceServiceAnswerResponsePDU should not be nil")
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.ServiceOptions != serviceOptions(0x81) {
		t.Errorf("ServiceOptions = %+v, want 0x81", decoded.UnitToUnitVoiceServiceAnswerResponsePDU.ServiceOptions)
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.AnswerResponse != 0x03 {
		t.Errorf("AnswerResponse = 0x%02X, want 0x03", decoded.UnitToUnitVoiceServiceAnswerResponsePDU.AnswerResponse)
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.TargetAddress != uint32(bit.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.SourceAddress != uint32(bit.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_ChannelTiming_Decode(t *testing.T) {
	var payload [64]bit.Bit
	// SyncAge (bits 0-10) = set bit 10 for value 1
	payload[10] = 1
	// Generation (bits 11-15) = set bit 15 for value 1
	payload[15] = 1
	// LeaderIdentifier (bits 16-35) = set bit 35
	payload[35] = 1
	// NewLeader (bit 36) = true
	payload[36] = 1
	// LeaderDynamicIdentifier (bits 37-38) = 0b11
	payload[37] = 1
	payload[38] = 1
	// ChannelTimingOp1 (bit 39, opcode MSB) = true
	payload[39] = 1
	// SourceIdentifier (bits 40-59) = set bit 59
	payload[59] = 1
	// Reserved (bit 60) = false
	// SourceDynamicIdentifier (bits 61-62) = 0b10
	payload[61] = 1
	// ChannelTimingOp0 (bit 63, opcode LSB) = false

	infoBits := buildCSBKBits(byte(pdu.CSBKChannelTiming), payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.Opcode != pdu.CSBKChannelTiming {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.Opcode), byte(pdu.CSBKChannelTiming))
	}
	if csbk.ChannelTimingPDU == nil {
		t.Fatal("ChannelTimingPDU should not be nil")
	}
	if !csbk.ChannelTimingPDU.NewLeader {
		t.Error("NewLeader should be true")
	}
	if !csbk.ChannelTimingPDU.ChannelTimingOp1 {
		t.Error("ChannelTimingOp1 (bit 39) should be true")
	}
	if csbk.ChannelTimingPDU.ChannelTimingOp0 {
		t.Error("ChannelTimingOp0 (bit 63) should be false")
	}
	if ((csbk.ChannelTimingPDU.LeaderDynamicIdentifier>>1)&1) != 1 || ((csbk.ChannelTimingPDU.LeaderDynamicIdentifier>>0)&1) != 1 {
		t.Error("LeaderDynamicIdentifier should be 0b11")
	}
	if ((csbk.ChannelTimingPDU.SourceDynamicIdentifier>>1)&1) != 1 || ((csbk.ChannelTimingPDU.SourceDynamicIdentifier>>0)&1) != 0 {
		t.Error("SourceDynamicIdentifier should be 0b10")
	}

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_ChannelTiming_EncodeDecodeCycle(t *testing.T) {
	var syncAge [11]bit.Bit
	syncAge[0] = 1
	syncAge[5] = 1
	var generation [5]bit.Bit
	generation[0] = 1
	generation[4] = 1
	var leaderID [20]bit.Bit
	leaderID[0] = 1
	leaderID[19] = 1
	var leaderDynID [2]bit.Bit
	leaderDynID[0] = 1
	var sourceID [20]bit.Bit
	sourceID[10] = 1
	var sourceDynID [2]bit.Bit
	sourceDynID[1] = 1

	original := &pdu.CSBK{
		LastBlock: true,
		Opcode:    pdu.CSBKChannelTiming,
		FID:       0x00,
		ChannelTimingPDU: &pdu.ChannelTimingPDU{
			SyncAge:                 uint16(bit.Uint(syncAge[:])),
			Generation:              uint8(bit.Uint(generation[:])),
			LeaderIdentifier:        uint32(bit.Uint(leaderID[:])),
			NewLeader:               true,
			LeaderDynamicIdentifier: uint8(bit.Uint(leaderDynID[:])),
			ChannelTimingOp0:        false,
			SourceIdentifier:        uint32(bit.Uint(sourceID[:])),
			Reserved:                false,
			SourceDynamicIdentifier: uint8(bit.Uint(sourceDynID[:])),
			ChannelTimingOp1:        true,
		},
	}

	encoded := must(original.Encode())
	if encoded[16+39] != 1 {
		t.Error("ChannelTimingOp1 should be encoded at payload bit 39")
	}
	if encoded[16+63] != 0 {
		t.Error("ChannelTimingOp0 should be encoded at payload bit 63")
	}
	decoded, fecResult := pdu.DecodeCSBK(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.ChannelTimingPDU == nil {
		t.Fatal("ChannelTimingPDU should not be nil")
	}
	if decoded.ChannelTimingPDU.SyncAge != uint16(bit.Uint(syncAge[:])) {
		t.Error("SyncAge mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.Generation != uint8(bit.Uint(generation[:])) {
		t.Error("Generation mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.LeaderIdentifier != uint32(bit.Uint(leaderID[:])) {
		t.Error("LeaderIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.NewLeader != true {
		t.Error("NewLeader should be true after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.LeaderDynamicIdentifier != uint8(bit.Uint(leaderDynID[:])) {
		t.Error("LeaderDynamicIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.ChannelTimingOp1 != true {
		t.Error("ChannelTimingOp1 should be true after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.ChannelTimingOp0 != false {
		t.Error("ChannelTimingOp0 should be false after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.SourceIdentifier != uint32(bit.Uint(sourceID[:])) {
		t.Error("SourceIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.SourceDynamicIdentifier != uint8(bit.Uint(sourceDynID[:])) {
		t.Error("SourceDynamicIdentifier mismatch after encode-decode cycle")
	}
}

func serviceOptions(b byte) layer3.ServiceOptions {
	var bits [8]bit.Bit
	bit.PutUint(bits[:], uint64(b))
	so, _ := layer3.DecodeServiceOptions(bits)
	return so
}
