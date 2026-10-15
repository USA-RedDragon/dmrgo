package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
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
		if byte(rc.RCCommand) != payload {
			t.Errorf("payload=%d: got %d", payload, rc.RCCommand)
			continue
		}

		// Re-encode and verify round-trip
		encoded, err := pdu.EncodeReverseChannel(&rc)
		if err != nil {
			t.Fatalf("payload=%d: unexpected error: %v", payload, err)
		}
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
	if fecResult != (fec.Result{}) {
		t.Errorf("CRC-7 only PDU: got %+v, want a zero result", fecResult)
	}
}

func TestReverseChannel_OutOfRangeRejected(t *testing.T) {
	t.Parallel()
	for _, cmd := range []enums.RCCommand{-1, 0x10, 0x14, 0xFF} {
		rc := pdu.ReverseChannel{RCCommand: cmd}
		if _, err := pdu.EncodeReverseChannel(&rc); err == nil {
			t.Errorf("RCCommand 0x%X: expected error, got nil", int(cmd))
		}
	}
}

func TestReverseChannel_String(t *testing.T) {
	t.Parallel()
	bits := buildRCInfoBits(7)
	rc, _ := pdu.DecodeReverseChannel(bits)
	s := rc.String()
	if s == "" {
		t.Error("String should not be empty")
	}
}

func TestReverseChannel_CRC7Mask(t *testing.T) {
	t.Parallel()
	if pdu.CRC7MaskRC != 0x7A {
		t.Errorf("CRC7MaskRC should be 0x7A, got 0x%02X", pdu.CRC7MaskRC)
	}
}
