package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildRCInfoBits constructs a valid 11-bit RC PDU with correct CRC-7.
func buildRCInfoBits(payload byte) [11]bit.Bit {
	var bits [11]bit.Bit
	// Payload: bits 0-3 (MSB-first)
	copy(bits[0:4], bit.BitsFromUint8(payload&0x0F, 4))
	// CRC-7 with mask 0x7A
	crcVal := crc.CalculateCRC7(bits[:4]) ^ pdu.CRC7MaskRC
	for i := 0; i < 7; i++ {
		bits[4+i] = bit.Bit((crcVal >> (6 - i)) & 1)
	}
	return bits
}

func TestReverseChannel_DecodeEncode_RoundTrip(t *testing.T) {
	t.Parallel()
	for payload := byte(0); payload < 16; payload++ {
		bits := buildRCInfoBits(payload)
		rc, fecResult := pdu.DecodeReverseChannel(bits)
		if fecResult.Uncorrectable {
			t.Errorf("payload=%d: unexpected uncorrectable error", payload)
			continue
		}
		if rc.RCPayload != payload {
			t.Errorf("payload=%d: got %d", payload, rc.RCPayload)
			continue
		}

		// Re-encode and verify round-trip
		encoded := pdu.EncodeReverseChannel(&rc)
		if encoded != bits {
			t.Errorf("payload=%d: round-trip failed: encoded %v != original %v", payload, encoded, bits)
		}
	}
}

func TestReverseChannel_CRCFailure(t *testing.T) {
	t.Parallel()
	bits := buildRCInfoBits(5)

	// Corrupt a CRC bit
	bits[7] ^= 1

	_, fecResult := pdu.DecodeReverseChannel(bits)
	if !fecResult.Uncorrectable {
		t.Error("expected uncorrectable after CRC corruption")
	}
}

func TestReverseChannel_BitsChecked(t *testing.T) {
	t.Parallel()
	bits := buildRCInfoBits(0)
	_, fecResult := pdu.DecodeReverseChannel(bits)
	if fecResult.BitsChecked != 11 {
		t.Errorf("expected BitsChecked=11, got %d", fecResult.BitsChecked)
	}
}

func TestReverseChannel_PayloadMasked(t *testing.T) {
	t.Parallel()
	// Verify only lower 4 bits are used
	rc := pdu.ReverseChannel{RCPayload: 0xFF}
	encoded := pdu.EncodeReverseChannel(&rc)
	decoded, fecResult := pdu.DecodeReverseChannel(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if decoded.RCPayload != 0x0F {
		t.Errorf("expected payload=0x0F (masked), got 0x%02X", decoded.RCPayload)
	}
}

func TestReverseChannel_ToString(t *testing.T) {
	t.Parallel()
	bits := buildRCInfoBits(7)
	rc, _ := pdu.DecodeReverseChannel(bits)
	s := rc.ToString()
	if s == "" {
		t.Error("ToString should not be empty")
	}
}

func TestReverseChannel_CRC7Mask(t *testing.T) {
	t.Parallel()
	if pdu.CRC7MaskRC != 0x7A {
		t.Errorf("CRC7MaskRC should be 0x7A, got 0x%02X", pdu.CRC7MaskRC)
	}
}
