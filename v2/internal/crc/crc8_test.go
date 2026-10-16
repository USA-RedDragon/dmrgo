package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func TestCRC8_ZeroInput(t *testing.T) {
	t.Parallel()

	bits := make([]bit.Bit, 28)
	result := crc8Of(bits)
	if result != 0x00 {
		t.Errorf("CRC8 of 28 zero bits = 0x%02X, want 0x00", result)
	}
}

func TestCRC8_SingleBit(t *testing.T) {
	t.Parallel()

	bits := []bit.Bit{1}
	result := crc8Of(bits)
	if result == 0x00 {
		t.Error("CRC8 of single 1-bit should be non-zero")
	}
}

func TestCRC8_CheckValid(t *testing.T) {
	t.Parallel()

	data := make([]bit.Bit, 28)
	data[0] = 1
	data[3] = 1
	data[7] = 1
	data[15] = 1
	data[27] = 1

	c := crc8Of(data)

	withCRC := make([]bit.Bit, 36)
	copy(withCRC, data)
	for i := 0; i < 8; i++ {
		withCRC[28+i] = bit.Bit((c >> (7 - i)) & 1)
	}

	if !checkCRC8Codeword(withCRC) {
		t.Error("CheckCRC8 should return true for valid data+CRC")
	}
}

func TestCRC8_CheckInvalid(t *testing.T) {
	t.Parallel()

	data := make([]bit.Bit, 28)
	data[0] = 1
	data[5] = 1

	c := crc8Of(data)

	withCRC := make([]bit.Bit, 36)
	copy(withCRC, data)
	for i := 0; i < 8; i++ {
		withCRC[28+i] = bit.Bit((c >> (7 - i)) & 1)
	}

	withCRC[5] ^= 1
	if checkCRC8Codeword(withCRC) {
		t.Error("CheckCRC8 should return false for corrupted data")
	}
}

func TestCRC8_CheckTooShort(t *testing.T) {
	t.Parallel()

	if checkCRC8Codeword(make([]bit.Bit, 8)) {
		t.Error("CheckCRC8 should return false for data shorter than 9 bits")
	}
}

func TestCRC8_Deterministic(t *testing.T) {
	t.Parallel()

	data := []bit.Bit{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 1, 0}
	crc1 := crc8Of(data)
	crc2 := crc8Of(data)
	if crc1 != crc2 {
		t.Errorf("CRC8 not deterministic: 0x%02X vs 0x%02X", crc1, crc2)
	}
}

func TestCRC8_DifferentDataDifferentCRC(t *testing.T) {
	t.Parallel()

	data1 := []bit.Bit{1, 0, 0, 0, 0, 0, 0, 0}
	data2 := []bit.Bit{0, 1, 0, 0, 0, 0, 0, 0}
	crc1 := crc8Of(data1)
	crc2 := crc8Of(data2)
	if crc1 == crc2 {
		t.Error("Different data should produce different CRCs")
	}
}
