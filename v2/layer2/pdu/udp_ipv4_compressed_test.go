package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestUDPIPv4CompressedHeader_RoundTrip(t *testing.T) {
	t.Parallel()
	original := pdu.UDPIPv4CompressedHeader{
		IPv4Identification:      0xABCD,
		SAID:                    0, // Radio Network
		DAID:                    2, // Group Network
		HeaderCompressionOpcode: 0, // UDP/IPv4
		SPID:                    1, // Text Message (5016)
		DPID:                    2, // Location Protocol (5017)
		ExtendedHeader1:         0x1234,
		ExtendedHeader2:         0x5678,
	}

	encoded := pdu.EncodeUDPIPv4CompressedHeader(&original)
	decoded, _ := pdu.DecodeUDPIPv4CompressedHeader(encoded)

	if decoded.IPv4Identification != original.IPv4Identification {
		t.Errorf("IPv4Identification = 0x%04X, want 0x%04X",
			decoded.IPv4Identification, original.IPv4Identification)
	}
	if decoded.SAID != original.SAID {
		t.Errorf("SAID = %d, want %d", decoded.SAID, original.SAID)
	}
	if decoded.DAID != original.DAID {
		t.Errorf("DAID = %d, want %d", decoded.DAID, original.DAID)
	}
	if decoded.HeaderCompressionOpcode != original.HeaderCompressionOpcode {
		t.Errorf("HeaderCompressionOpcode = %d, want %d",
			decoded.HeaderCompressionOpcode, original.HeaderCompressionOpcode)
	}
	if decoded.SPID != original.SPID {
		t.Errorf("SPID = %d, want %d", decoded.SPID, original.SPID)
	}
	if decoded.DPID != original.DPID {
		t.Errorf("DPID = %d, want %d", decoded.DPID, original.DPID)
	}
	if decoded.ExtendedHeader1 != original.ExtendedHeader1 {
		t.Errorf("ExtendedHeader1 = 0x%04X, want 0x%04X",
			decoded.ExtendedHeader1, original.ExtendedHeader1)
	}
	if decoded.ExtendedHeader2 != original.ExtendedHeader2 {
		t.Errorf("ExtendedHeader2 = 0x%04X, want 0x%04X",
			decoded.ExtendedHeader2, original.ExtendedHeader2)
	}
}

func TestUDPIPv4CompressedHeader_NonContiguousHCOpcode(t *testing.T) {
	t.Parallel()
	// Test that the 2-bit non-contiguous HeaderCompressionOpcode (bits 24 + 32)
	// survives round-trip for all 4 possible values
	for opcode := uint8(0); opcode <= 3; opcode++ {
		original := pdu.UDPIPv4CompressedHeader{
			HeaderCompressionOpcode: opcode,
			SPID:                    42,
			DPID:                    17,
		}
		encoded := pdu.EncodeUDPIPv4CompressedHeader(&original)
		decoded, _ := pdu.DecodeUDPIPv4CompressedHeader(encoded)
		if decoded.HeaderCompressionOpcode != opcode {
			t.Errorf("opcode %d: HeaderCompressionOpcode = %d", opcode, decoded.HeaderCompressionOpcode)
		}
	}
}

func TestUDPIPv4CompressedHeader_ToString(t *testing.T) {
	t.Parallel()
	h := pdu.UDPIPv4CompressedHeader{
		IPv4Identification: 0x1234,
		SAID:               0,
		DAID:               1,
		SPID:               1,
		DPID:               2,
	}
	s := h.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestUDPIPv4CompressedHeader_EncodeStability(t *testing.T) {
	t.Parallel()
	h := &pdu.UDPIPv4CompressedHeader{
		IPv4Identification:      0xFFFF,
		SAID:                    15,
		DAID:                    15,
		HeaderCompressionOpcode: 3,
		SPID:                    127,
		DPID:                    127,
		ExtendedHeader1:         0xFFFF,
		ExtendedHeader2:         0xFFFF,
	}
	enc1 := pdu.EncodeUDPIPv4CompressedHeader(h)
	enc2 := pdu.EncodeUDPIPv4CompressedHeader(h)
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}

	// Verify bits are valid (all 0 or 1)
	for i, b := range enc1 {
		if b != bit.Bit(0) && b != bit.Bit(1) {
			t.Errorf("bit %d: invalid value %d", i, b)
		}
	}
}
