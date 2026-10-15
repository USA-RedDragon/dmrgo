package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
)

func TestCRCCCITT_KnownValues(t *testing.T) {
	t.Parallel()

	data := []byte("123456789")
	crc := crc.CCITT(data)
	if crc != 0xCE3C {
		t.Errorf("CRC of '123456789' = 0x%04X, want 0xCE3C", crc)
	}
}

func TestCRCCCITT_CheckValid(t *testing.T) {
	t.Parallel()

	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	cCRC := crc.CCITT(data)

	withCRC := append(append([]byte{}, data...), byte(cCRC>>8), byte(cCRC))
	if !checkCCITTCodeword(withCRC) {
		t.Error("CheckCRCCCITT should return true for valid data+CRC")
	}
}

func TestCRCCCITT_CheckInvalid(t *testing.T) {
	t.Parallel()

	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	cCRC := crc.CCITT(data)
	withCRC := append(append([]byte{}, data...), byte(cCRC>>8), byte(cCRC))

	withCRC[2] ^= 0xFF
	if checkCCITTCodeword(withCRC) {
		t.Error("CheckCRCCCITT should return false for corrupted data")
	}
}

func TestCRCCCITT_CheckTooShort(t *testing.T) {
	t.Parallel()

	if checkCCITTCodeword([]byte{0x01, 0x02}) {
		t.Error("CheckCRCCCITT should return false for data shorter than 3 bytes")
	}
}

func TestCRCCCITT_EmptyInput(t *testing.T) {
	t.Parallel()

	crc := crc.CCITT([]byte{})
	if crc != 0xFFFF {
		t.Errorf("CRC of empty = 0x%04X, want 0xFFFF", crc)
	}
}
