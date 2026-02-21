package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

func TestCRC9_ZeroInput(t *testing.T) {
	// All-zero input: polynomial division yields 0, then XOR with 0x1FF
	bits := make([]bit.Bit, 87)
	result := crc.CalculateCRC9(bits)
	if result != 0x1FF {
		t.Errorf("CRC9 of 87 zero bits = 0x%03X, want 0x1FF", result)
	}
}

func TestCRC9_SingleBit(t *testing.T) {
	bits := []bit.Bit{1}
	result := crc.CalculateCRC9(bits)
	// With inversion, should not be 0x1FF
	if result == 0x1FF {
		t.Error("CRC9 of single 1-bit should differ from all-zero CRC")
	}
	// Must be in 9-bit range
	if result > 0x1FF {
		t.Errorf("CRC9 = 0x%03X, exceeds 9-bit range", result)
	}
}

func TestCRC9_CheckValid_NoMask(t *testing.T) {
	data := make([]bit.Bit, 87)
	data[0] = 1
	data[10] = 1
	data[40] = 1
	data[86] = 1

	c := crc.CalculateCRC9(data)

	// Append CRC as 9 bits MSB-first
	withCRC := make([]bit.Bit, 96)
	copy(withCRC, data)
	for i := 0; i < 9; i++ {
		withCRC[87+i] = bit.Bit((c >> (8 - i)) & 1)
	}

	if !crc.CheckCRC9(withCRC, 0x000) {
		t.Error("CheckCRC9 should return true for valid data+CRC (no mask)")
	}
}

func TestCRC9_CheckValid_WithMask(t *testing.T) {
	// Rate 3/4 data continuation mask: 0x1FF (per B.3.12)
	data := make([]bit.Bit, 135)
	data[0] = 1
	data[50] = 1
	data[100] = 1

	c := crc.CalculateCRC9(data) ^ 0x1FF // apply mask

	withCRC := make([]bit.Bit, 144)
	copy(withCRC, data)
	for i := 0; i < 9; i++ {
		withCRC[135+i] = bit.Bit((c >> (8 - i)) & 1)
	}

	if !crc.CheckCRC9(withCRC, 0x1FF) {
		t.Error("CheckCRC9 should return true for valid data+CRC with mask 0x1FF")
	}
}

func TestCRC9_CheckInvalid(t *testing.T) {
	data := make([]bit.Bit, 87)
	data[0] = 1

	c := crc.CalculateCRC9(data)

	withCRC := make([]bit.Bit, 96)
	copy(withCRC, data)
	for i := 0; i < 9; i++ {
		withCRC[87+i] = bit.Bit((c >> (8 - i)) & 1)
	}

	// Corrupt data
	withCRC[0] ^= 1
	if crc.CheckCRC9(withCRC, 0x000) {
		t.Error("CheckCRC9 should return false for corrupted data")
	}
}

func TestCRC9_CheckTooShort(t *testing.T) {
	if crc.CheckCRC9(make([]bit.Bit, 9), 0) {
		t.Error("CheckCRC9 should return false for data shorter than 10 bits")
	}
}

func TestCRC9_Deterministic(t *testing.T) {
	data := make([]bit.Bit, 135)
	for i := range data {
		data[i] = bit.Bit(i % 2)
	}
	crc1 := crc.CalculateCRC9(data)
	crc2 := crc.CalculateCRC9(data)
	if crc1 != crc2 {
		t.Errorf("CRC9 not deterministic: 0x%03X vs 0x%03X", crc1, crc2)
	}
}

func TestCRC9_ValueRange(t *testing.T) {
	// CRC-9 should always be in range [0, 0x1FF]
	data := make([]bit.Bit, 183)
	for i := range data {
		data[i] = bit.Bit(i % 3)
	}
	c := crc.CalculateCRC9(data)
	if c > 0x1FF {
		t.Errorf("CRC9 = 0x%03X, exceeds 9-bit range", c)
	}
}

func TestCRC9_Rate12_DataSize(t *testing.T) {
	// Rate 1/2 confirmed: 80 data + 7 serial = 87 bits
	data := make([]bit.Bit, 87)
	data[0] = 1
	data[86] = 1
	c := crc.CalculateCRC9(data)
	if c > 0x1FF {
		t.Errorf("CRC9 for Rate 1/2 size = 0x%03X, exceeds range", c)
	}
}
