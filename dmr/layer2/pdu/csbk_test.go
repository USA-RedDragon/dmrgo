package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/crc"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

// buildCSBKBits constructs a [96]bit.Bit array for a CSBK PDU.
// opcode is 6 bits, fid is 8 bits, payload is 64 bits.
// lb and pf flags default to true/false.
// The CRC-CCITT is computed and XOR-masked per spec (§9.1.7).
func buildCSBKBits(opcode byte, fid byte, payload [64]bit.Bit) [96]bit.Bit {
	var dataBytes [12]byte

	// Byte 0: lb=1, pf=0, opcode(6 bits)
	dataBytes[0] = 0x80 | (opcode & 0x3F) // lb=1, pf=0, 6-bit opcode
	dataBytes[1] = fid

	// Pack 64 payload bits into bytes 2..9
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			dataBytes[2+i] <<= 1
			dataBytes[2+i] |= byte(payload[i*8+j])
		}
	}

	// Compute CRC over bytes 0..9
	crc := crc.CalculateCRCCCITT(dataBytes[:10])
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

func TestCSBK_OpcodeToString(t *testing.T) {
	tests := []struct {
		opcode pdu.CSBKOpcode
		want   string
	}{
		{pdu.CSBKBSOutboundActivationPDU, "BS Outbound Activation PDU"},
		{pdu.CSBKUnitToUnitVoiceServiceRequestPDU, "Unit-to-Unit Voice Service Request PDU"},
		{pdu.CSBKUnitToUnitVoiceServiceAnswerResponsePDU, "Unit-to-Unit Voice Service Answer/Response PDU"},
		{pdu.CSBKNegativeAcknowledgementPDU, "Negative Acknowledgement PDU"},
		{pdu.CSBKPreamblePDU, "Preamble PDU"},
		{pdu.CSBKChannelTimingPDU, "Channel Timing PDU"},
	}
	for _, tt := range tests {
		if got := tt.opcode.ToString(); got != tt.want {
			t.Errorf("CSBKOpcode(%08b).ToString() = %q, want %q", byte(tt.opcode), got, tt.want)
		}
	}

	// Unknown opcode should contain "Unknown"
	unknown := pdu.CSBKOpcode(0xFF)
	if s := unknown.ToString(); s == "" {
		t.Error("unknown opcode ToString() should not be empty")
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

	infoBits := buildCSBKBits(0b00111101, 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.CSBKOpcode != pdu.CSBKPreamblePDU {
		t.Errorf("CSBKOpcode = %08b, want %08b", byte(csbk.CSBKOpcode), byte(pdu.CSBKPreamblePDU))
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

	// Verify ToString doesn't panic
	str := csbk.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
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

	infoBits := buildCSBKBits(0b00100110, 0x00, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.CSBKOpcode != pdu.CSBKNegativeAcknowledgementPDU {
		t.Errorf("CSBKOpcode = %08b, want NegativeAck", byte(csbk.CSBKOpcode))
	}
	if csbk.NegativeAcknowledgementPDU == nil {
		t.Fatal("NegativeAcknowledgementPDU should not be nil")
	}
	if !csbk.NegativeAcknowledgementPDU.AdditionalInfo {
		t.Error("AdditionalInfo should be true")
	}
	if csbk.NegativeAcknowledgementPDU.ReasonCode != 0x42 {
		t.Errorf("ReasonCode = 0x%02X, want 0x42", csbk.NegativeAcknowledgementPDU.ReasonCode)
	}
}

func TestCSBK_CRCValidation(t *testing.T) {
	// Build valid CSBK bits, then corrupt them to verify CRC catches it
	var payload [64]bit.Bit
	payload[0] = 1
	infoBits := buildCSBKBits(0b00111101, 0x00, payload)

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
