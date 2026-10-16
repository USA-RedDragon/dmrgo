package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// buildCSBK returns the 12 octets of a standard FID CSBK with LB set, the
// given opcode and payload, and the CRC-CCITT with the CSBK mask.
func buildCSBK(opcode byte, payload [8]byte) [12]byte {
	var data [12]byte
	data[0] = 0x80 | (opcode & 0x3F)
	copy(data[2:10], payload[:])
	c := crc.CCITT(data[:10]) ^ 0xA5A5
	data[10] = byte(c >> 8)
	data[11] = byte(c)
	return data
}

func TestCSBK_OpcodeString(t *testing.T) {
	t.Parallel()

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

	unknown := pdu.CSBKOpcode(0xFF)
	if s := unknown.String(); s == "" {
		t.Error("unknown opcode String() should not be empty")
	}
}

func TestCSBK_PreamblePDU_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 0, 1)
	bits.SetBit(payload[:], 1, 0)
	bits.SetBit(payload[:], 8+5, 1)
	bits.SetBit(payload[:], 8+7, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 62, 1)

	infoBits := buildCSBK(0b00111101, payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_NegativeAck_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 0, 1)
	bits.SetBit(payload[:], 6, 1)
	bits.SetBit(payload[:], 7, 1)
	bits.SetBit(payload[:], 9, 1)
	bits.SetBit(payload[:], 14, 1)

	infoBits := buildCSBK(0b00100110, payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 0, 1)
	infoBits := buildCSBK(0b00111101, payload)

	fecResult := pdu.DecodeCSBK(infoBits).FEC
	if fecResult.Uncorrectable {
		t.Fatal("valid CSBK should decode ok")
	}

	corrupted := infoBits
	corrupted[5] ^= 0x80

	fecResult2 := pdu.DecodeCSBK(corrupted).FEC
	if !fecResult2.Uncorrectable {
		t.Error("corrupted CSBK should fail CRC check")
	}
}

func TestCSBK_BSOutboundActivation_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 62, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKBSOutboundActivation), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

	str := csbk.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCSBK_BSOutboundActivation_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

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
			BSAddress:     uint32(unpacked.Uint(bsAddr[:])),
			SourceAddress: uint32(unpacked.Uint(srcAddr[:])),
		}).Encode()),
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	decodedAct, ok := decoded.AsBSOutboundActivation()
	if !ok {
		t.Fatal("BSOutboundActivationPDU should not be nil")
	}
	if decodedAct.BSAddress != uint32(unpacked.Uint(bsAddr[:])) {
		t.Error("BSAddress mismatch after encode-decode cycle")
	}
	if decodedAct.SourceAddress != uint32(unpacked.Uint(srcAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_UnitToUnitVoiceServiceRequest_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 0, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKUnitToUnitVoiceServiceRequest), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
	t.Parallel()

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
			TargetAddress:  uint32(unpacked.Uint(targetAddr[:])),
			SourceAddress:  uint32(unpacked.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU == nil {
		t.Fatal("UnitToUnitVoiceServiceRequestPDU should not be nil")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.ServiceOptions != serviceOptions(0x42) {
		t.Errorf("ServiceOptions = %+v, want 0x42", decoded.UnitToUnitVoiceServiceRequestPDU.ServiceOptions)
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.TargetAddress != uint32(unpacked.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if decoded.UnitToUnitVoiceServiceRequestPDU.SourceAddress != uint32(unpacked.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_UnitToUnitVoiceServiceAnswerResponse_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 1, 1)
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 63, 1)

	infoBits := buildCSBK(byte(pdu.CSBKUnitToUnitVoiceServiceAnswerResponse), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
	t.Parallel()

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
			TargetAddress:  uint32(unpacked.Uint(targetAddr[:])),
			SourceAddress:  uint32(unpacked.Uint(sourceAddr[:])),
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
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
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.TargetAddress != uint32(unpacked.Uint(targetAddr[:])) {
		t.Error("TargetAddress mismatch after encode-decode cycle")
	}
	if decoded.UnitToUnitVoiceServiceAnswerResponsePDU.SourceAddress != uint32(unpacked.Uint(sourceAddr[:])) {
		t.Error("SourceAddress mismatch after encode-decode cycle")
	}
}

func TestCSBK_ChannelTiming_Decode(t *testing.T) {
	t.Parallel()

	var payload [8]byte
	bits.SetBit(payload[:], 10, 1)
	bits.SetBit(payload[:], 15, 1)
	bits.SetBit(payload[:], 35, 1)
	bits.SetBit(payload[:], 36, 1)
	bits.SetBit(payload[:], 37, 1)
	bits.SetBit(payload[:], 38, 1)
	bits.SetBit(payload[:], 39, 1)
	bits.SetBit(payload[:], 59, 1)
	bits.SetBit(payload[:], 61, 1)

	infoBits := buildCSBK(byte(pdu.CSBKChannelTiming), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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
	t.Parallel()

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
			SyncAge:                 uint16(unpacked.Uint(syncAge[:])),
			Generation:              uint8(unpacked.Uint(generation[:])),
			LeaderIdentifier:        uint32(unpacked.Uint(leaderID[:])),
			NewLeader:               true,
			LeaderDynamicIdentifier: uint8(unpacked.Uint(leaderDynID[:])),
			ChannelTimingOp0:        false,
			SourceIdentifier:        uint32(unpacked.Uint(sourceID[:])),
			Reserved:                false,
			SourceDynamicIdentifier: uint8(unpacked.Uint(sourceDynID[:])),
			ChannelTimingOp1:        true,
		},
	}

	encoded := must(original.Encode())
	if bits.Bit(encoded[:], 16+39) != 1 {
		t.Error("ChannelTimingOp1 should be encoded at payload bit 39")
	}
	if bits.Bit(encoded[:], 16+63) != 0 {
		t.Error("ChannelTimingOp0 should be encoded at payload bit 63")
	}
	decoded := pdu.DecodeCSBK(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if decoded.ChannelTimingPDU == nil {
		t.Fatal("ChannelTimingPDU should not be nil")
	}
	if decoded.ChannelTimingPDU.SyncAge != uint16(unpacked.Uint(syncAge[:])) {
		t.Error("SyncAge mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.Generation != uint8(unpacked.Uint(generation[:])) {
		t.Error("Generation mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.LeaderIdentifier != uint32(unpacked.Uint(leaderID[:])) {
		t.Error("LeaderIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.NewLeader != true {
		t.Error("NewLeader should be true after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.LeaderDynamicIdentifier != uint8(unpacked.Uint(leaderDynID[:])) {
		t.Error("LeaderDynamicIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.ChannelTimingOp1 != true {
		t.Error("ChannelTimingOp1 should be true after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.ChannelTimingOp0 != false {
		t.Error("ChannelTimingOp0 should be false after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.SourceIdentifier != uint32(unpacked.Uint(sourceID[:])) {
		t.Error("SourceIdentifier mismatch after encode-decode cycle")
	}
	if decoded.ChannelTimingPDU.SourceDynamicIdentifier != uint8(unpacked.Uint(sourceDynID[:])) {
		t.Error("SourceDynamicIdentifier mismatch after encode-decode cycle")
	}
}

func serviceOptions(b byte) layer3.ServiceOptions {
	return layer3.DecodeServiceOptions([1]byte{b})
}
