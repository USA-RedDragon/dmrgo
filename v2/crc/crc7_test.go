package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

func TestCRC7_ZeroInput(t *testing.T) {
	bits := make([]bit.Bit, 4)
	result := crc.CalculateCRC7(bits)
	if result != 0x00 {
		t.Errorf("CRC7 of 4 zero bits = 0x%02X, want 0x00", result)
	}
}

func TestCRC7_SingleBit(t *testing.T) {
	bits := []bit.Bit{1}
	result := crc.CalculateCRC7(bits)
	if result == 0 {
		t.Error("CRC7 of single 1-bit should be non-zero")
	}
}

func TestCRC7_CheckValid_NoMask(t *testing.T) {
	data := []bit.Bit{1, 0, 1, 1}
	c := crc.CalculateCRC7(data)

	// Append CRC as 7 bits MSB-first
	withCRC := make([]bit.Bit, 11)
	copy(withCRC, data)
	for i := 0; i < 7; i++ {
		withCRC[4+i] = bit.Bit((c >> (6 - i)) & 1)
	}

	if !crc.CheckCRC7(withCRC, 0x00) {
		t.Error("CheckCRC7 should return true for valid data+CRC (no mask)")
	}
}

func TestCRC7_CheckValid_WithMask(t *testing.T) {
	// Simulate Reverse Channel CRC mask 0x7A (per B.3.12)
	data := []bit.Bit{1, 0, 1, 1}
	c := crc.CalculateCRC7(data) ^ 0x7A

	withCRC := make([]bit.Bit, 11)
	copy(withCRC, data)
	for i := 0; i < 7; i++ {
		withCRC[4+i] = bit.Bit((c >> (6 - i)) & 1)
	}

	if !crc.CheckCRC7(withCRC, 0x7A) {
		t.Error("CheckCRC7 should return true for valid data+CRC with mask 0x7A")
	}
}

func TestCRC7_CheckInvalid(t *testing.T) {
	data := []bit.Bit{1, 0, 1, 1}
	c := crc.CalculateCRC7(data)

	withCRC := make([]bit.Bit, 11)
	copy(withCRC, data)
	for i := 0; i < 7; i++ {
		withCRC[4+i] = bit.Bit((c >> (6 - i)) & 1)
	}

	// Corrupt a data bit
	withCRC[0] ^= 1
	if crc.CheckCRC7(withCRC, 0x00) {
		t.Error("CheckCRC7 should return false for corrupted data")
	}
}

func TestCRC7_CheckTooShort(t *testing.T) {
	if crc.CheckCRC7(make([]bit.Bit, 7), 0x00) {
		t.Error("CheckCRC7 should return false for data shorter than 8 bits")
	}
}

func TestCRC7_ValueRange(t *testing.T) {
	// CRC-7 should always be in range [0, 127]
	for v := 0; v < 16; v++ {
		data := []bit.Bit{
			bit.Bit((v >> 3) & 1),
			bit.Bit((v >> 2) & 1),
			bit.Bit((v >> 1) & 1),
			bit.Bit(v & 1),
		}
		c := crc.CalculateCRC7(data)
		if c > 0x7F {
			t.Errorf("CRC7(%v) = 0x%02X, exceeds 7-bit range", data, c)
		}
	}
}
