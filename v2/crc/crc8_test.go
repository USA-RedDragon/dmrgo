package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

func TestCRC8_ZeroInput(t *testing.T) {
	// All-zero input should produce zero CRC (no feedback ever triggers)
	bits := make([]bit.Bit, 28)
	result := crc.CalculateCRC8(bits)
	if result != 0x00 {
		t.Errorf("CRC8 of 28 zero bits = 0x%02X, want 0x00", result)
	}
}

func TestCRC8_SingleBit(t *testing.T) {
	// A single 1-bit should produce a non-zero CRC
	bits := []bit.Bit{1}
	result := crc.CalculateCRC8(bits)
	if result == 0x00 {
		t.Error("CRC8 of single 1-bit should be non-zero")
	}
}

func TestCRC8_CheckValid(t *testing.T) {
	// Build data + CRC, then verify CheckCRC8 succeeds
	data := make([]bit.Bit, 28)
	// Set some bits to create a non-trivial pattern
	data[0] = 1
	data[3] = 1
	data[7] = 1
	data[15] = 1
	data[27] = 1

	c := crc.CalculateCRC8(data)

	// Append CRC as 8 bits MSB-first
	withCRC := make([]bit.Bit, 36)
	copy(withCRC, data)
	for i := 0; i < 8; i++ {
		withCRC[28+i] = bit.Bit((c >> (7 - i)) & 1)
	}

	if !crc.CheckCRC8(withCRC) {
		t.Error("CheckCRC8 should return true for valid data+CRC")
	}
}

func TestCRC8_CheckInvalid(t *testing.T) {
	data := make([]bit.Bit, 28)
	data[0] = 1
	data[5] = 1

	c := crc.CalculateCRC8(data)

	withCRC := make([]bit.Bit, 36)
	copy(withCRC, data)
	for i := 0; i < 8; i++ {
		withCRC[28+i] = bit.Bit((c >> (7 - i)) & 1)
	}

	// Corrupt one data bit
	withCRC[5] ^= 1
	if crc.CheckCRC8(withCRC) {
		t.Error("CheckCRC8 should return false for corrupted data")
	}
}

func TestCRC8_CheckTooShort(t *testing.T) {
	// Fewer than 9 bits should return false
	if crc.CheckCRC8(make([]bit.Bit, 8)) {
		t.Error("CheckCRC8 should return false for data shorter than 9 bits")
	}
}

func TestCRC8_Deterministic(t *testing.T) {
	// Same input produces same CRC
	data := []bit.Bit{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 1, 0}
	crc1 := crc.CalculateCRC8(data)
	crc2 := crc.CalculateCRC8(data)
	if crc1 != crc2 {
		t.Errorf("CRC8 not deterministic: 0x%02X vs 0x%02X", crc1, crc2)
	}
}

func TestCRC8_DifferentDataDifferentCRC(t *testing.T) {
	data1 := []bit.Bit{1, 0, 0, 0, 0, 0, 0, 0}
	data2 := []bit.Bit{0, 1, 0, 0, 0, 0, 0, 0}
	crc1 := crc.CalculateCRC8(data1)
	crc2 := crc.CalculateCRC8(data2)
	if crc1 == crc2 {
		t.Error("Different data should produce different CRCs")
	}
}
