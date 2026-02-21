package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

func TestCRC32_ZeroInput(t *testing.T) {
	result := crc.CalculateCRC32([]byte{})
	if result != 0 {
		t.Errorf("CRC32 of empty input = 0x%08X, want 0x00000000", result)
	}
}

func TestCRC32_AllZeros(t *testing.T) {
	// All-zero input of even length should produce zero CRC
	result := crc.CalculateCRC32(make([]byte, 10))
	if result != 0 {
		t.Errorf("CRC32 of 10 zero bytes = 0x%08X, want 0x00000000", result)
	}
}

func TestCRC32_NonZero(t *testing.T) {
	// Non-zero input should produce non-zero CRC
	result := crc.CalculateCRC32([]byte{0x01, 0x02})
	if result == 0 {
		t.Error("CRC32 of [0x01, 0x02] should be non-zero")
	}
}

func TestCRC32_Deterministic(t *testing.T) {
	data := []byte{0xAA, 0x55, 0xDE, 0xAD, 0xBE, 0xEF}
	crc1 := crc.CalculateCRC32(data)
	crc2 := crc.CalculateCRC32(data)
	if crc1 != crc2 {
		t.Errorf("CRC32 not deterministic: 0x%08X vs 0x%08X", crc1, crc2)
	}
}

func TestCRC32_ByteSwap(t *testing.T) {
	// Verify byte-swapping within 16-bit words:
	// [0x01, 0x02] with swap → [0x02, 0x01]
	// [0x02, 0x01] without swap should equal [0x01, 0x02] with swap
	result1 := crc.CalculateCRC32([]byte{0x01, 0x02})

	// Manually pre-swap and compute with identity swap
	// This should differ from un-swapped since we're swapping twice
	result2 := crc.CalculateCRC32([]byte{0x02, 0x01})

	// They should be different because CalculateCRC32 always swaps internally
	if result1 == result2 {
		t.Error("CRC32 should produce different results for [0x01,0x02] vs [0x02,0x01] due to internal swap")
	}
}

func TestCRC32_OddLength(t *testing.T) {
	// Odd-length data: last byte is not swapped
	result := crc.CalculateCRC32([]byte{0x01, 0x02, 0x03})
	if result == 0 {
		t.Error("CRC32 of 3 non-zero bytes should be non-zero")
	}
}

func TestCRC32_CheckValid(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	expected := crc.CalculateCRC32(data)

	if !crc.CheckCRC32(data, expected) {
		t.Error("CheckCRC32 should return true for matching CRC")
	}
}

func TestCRC32_CheckInvalid(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	expected := crc.CalculateCRC32(data)

	if crc.CheckCRC32(data, expected^0x01) {
		t.Error("CheckCRC32 should return false for non-matching CRC")
	}
}

func TestCRC32_DifferentDataDifferentCRC(t *testing.T) {
	data1 := []byte{0x01, 0x02, 0x03, 0x04}
	data2 := []byte{0x01, 0x02, 0x03, 0x05}
	crc1 := crc.CalculateCRC32(data1)
	crc2 := crc.CalculateCRC32(data2)
	if crc1 == crc2 {
		t.Error("Different data should produce different CRC-32 values")
	}
}
