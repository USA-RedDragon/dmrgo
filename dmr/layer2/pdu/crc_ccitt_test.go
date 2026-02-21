package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

func TestCRCCCITT_KnownValues(t *testing.T) {
	// Test with known input. "123456789" is the standard CRC test vector.
	// This implementation uses CRC-CCITT with init=0x0000, poly=0x1021, final inversion.
	data := []byte("123456789")
	crc := pdu.CalculateCRCCCITT(data)
	if crc != 0xCE3C {
		t.Errorf("CRC of '123456789' = 0x%04X, want 0xCE3C", crc)
	}
}

func TestCRCCCITT_CheckValid(t *testing.T) {
	// Build data + CRC, then verify CheckCRCCCITT succeeds
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	crc := pdu.CalculateCRCCCITT(data)

	// Append CRC in the order that CheckCRCCCITT expects (big-endian, swapped)
	// CheckCRCCCITT: crc8[0] == in[length-1] && crc8[1] == in[length-2]
	// So last byte = low byte of CRC, second-to-last = high byte of CRC
	withCRC := append(data, byte(crc>>8), byte(crc))
	if !pdu.CheckCRCCCITT(withCRC) {
		t.Error("CheckCRCCCITT should return true for valid data+CRC")
	}
}

func TestCRCCCITT_CheckInvalid(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	crc := pdu.CalculateCRCCCITT(data)
	withCRC := append(data, byte(crc>>8), byte(crc))

	// Corrupt one byte
	withCRC[2] ^= 0xFF
	if pdu.CheckCRCCCITT(withCRC) {
		t.Error("CheckCRCCCITT should return false for corrupted data")
	}
}

func TestCRCCCITT_CheckTooShort(t *testing.T) {
	if pdu.CheckCRCCCITT([]byte{0x01, 0x02}) {
		t.Error("CheckCRCCCITT should return false for data shorter than 3 bytes")
	}
}

func TestCRCCCITT_EmptyInput(t *testing.T) {
	// Empty input should produce a deterministic CRC (complement of 0)
	crc := pdu.CalculateCRCCCITT([]byte{})
	if crc != 0xFFFF {
		t.Errorf("CRC of empty = 0x%04X, want 0xFFFF", crc)
	}
}
