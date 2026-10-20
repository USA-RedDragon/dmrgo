package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestUDPIPv4CompressedHeader_RoundTrip(t *testing.T) {
	t.Parallel()
	original := pdu.UDPIPv4CompressedHeader{
		IPv4Identification:      0xABCD,
		SAID:                    0,
		DAID:                    2,
		HeaderCompressionOpcode: 0,
		SPID:                    1,
		DPID:                    2,
		ExtendedHeader1:         0x1234,
		ExtendedHeader2:         0x5678,
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeUDPIPv4CompressedHeader(encoded)

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
	for opcode := uint8(0); opcode <= 3; opcode++ {
		original := pdu.UDPIPv4CompressedHeader{
			HeaderCompressionOpcode: opcode,
			SPID:                    42,
			DPID:                    17,
		}
		encoded := must(original.Encode())
		decoded := pdu.DecodeUDPIPv4CompressedHeader(encoded)
		if decoded.HeaderCompressionOpcode != opcode {
			t.Errorf("opcode %d: HeaderCompressionOpcode = %d", opcode, decoded.HeaderCompressionOpcode)
		}
	}
}

func TestUDPIPv4CompressedHeader_Ports(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spid     enums.SPID
		dpid     enums.DPID
		octets   int
		src, dst uint16
	}{
		{enums.SPIDExtendedHeader, enums.DPIDExtendedHeader, 9, 0x1111, 0x2222},
		{enums.SPIDExtendedHeader, enums.DPIDLocationProtocol, 7, 0x1111, 5017},
		{enums.SPIDTextMessage, enums.DPIDExtendedHeader, 7, 5016, 0x1111},
		{enums.SPIDLocationProtocol, enums.DPIDTextMessage, 5, 5017, 5016},
		{enums.SPIDManufacturerSpecific, enums.DPIDManufacturerSpecific, 5, 0, 0},
	}
	for _, tt := range tests {
		h := pdu.UDPIPv4CompressedHeader{SPID: tt.spid, DPID: tt.dpid, ExtendedHeader1: 0x1111, ExtendedHeader2: 0x2222}
		src, dst := h.Ports()
		if h.Octets() != tt.octets || src != tt.src || dst != tt.dst {
			t.Errorf("SPID %d DPID %d: %d octets, ports %d %d, want %d octets, ports %d %d",
				tt.spid, tt.dpid, h.Octets(), src, dst, tt.octets, tt.src, tt.dst)
		}
	}
}

func TestUDPIPv4CompressedHeader_String(t *testing.T) {
	t.Parallel()
	h := pdu.UDPIPv4CompressedHeader{
		IPv4Identification: 0x1234,
		SAID:               0,
		DAID:               1,
		SPID:               1,
		DPID:               2,
	}
	s := h.String()
	if s == "" {
		t.Error("String() should not be empty")
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
	enc1 := must(h.Encode())
	enc2 := must(h.Encode())
	for i := range enc1 {
		if enc1[i] != enc2[i] {
			t.Errorf("octet %d: first=%#02x, second=%#02x", i, enc1[i], enc2[i])
		}
	}
}
