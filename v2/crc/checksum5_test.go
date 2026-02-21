package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/crc"
)

func TestChecksum5_AllZeros(t *testing.T) {
	data := [9]byte{}
	result := crc.CalculateChecksum5(data)
	if result != 0 {
		t.Errorf("Checksum5 of all zeros = %d, want 0", result)
	}
}

func TestChecksum5_SingleByte(t *testing.T) {
	data := [9]byte{31}
	result := crc.CalculateChecksum5(data)
	if result != 0 {
		t.Errorf("Checksum5 of {31,0,...} = %d, want 0 (31 mod 31)", result)
	}
}

func TestChecksum5_SimpleSum(t *testing.T) {
	// 1+2+3+4+5+6+7+8+9 = 45; 45 mod 31 = 14
	data := [9]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
	result := crc.CalculateChecksum5(data)
	if result != 14 {
		t.Errorf("Checksum5 of {1..9} = %d, want 14", result)
	}
}

func TestChecksum5_MaxValues(t *testing.T) {
	// 9 * 255 = 2295; 2295 mod 31 = 2295 - 74*31 = 2295 - 2294 = 1
	data := [9]byte{255, 255, 255, 255, 255, 255, 255, 255, 255}
	result := crc.CalculateChecksum5(data)
	if result != 1 {
		t.Errorf("Checksum5 of all 0xFF = %d, want 1", result)
	}
}

func TestChecksum5_Range(t *testing.T) {
	// Result must always be in [0, 30]
	for i := 0; i < 256; i++ {
		data := [9]byte{byte(i), byte(i >> 1), byte(i >> 2), 0, 0, 0, 0, 0, 0}
		result := crc.CalculateChecksum5(data)
		if result > 30 {
			t.Errorf("Checksum5 returned %d for data starting with %d, want ≤30", result, i)
		}
	}
}

func TestChecksum5_CheckValid(t *testing.T) {
	data := [9]byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x90}
	cs := crc.CalculateChecksum5(data)
	if !crc.CheckChecksum5(data, cs) {
		t.Error("CheckChecksum5 should return true for matching checksum")
	}
}

func TestChecksum5_CheckInvalid(t *testing.T) {
	data := [9]byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x90}
	cs := crc.CalculateChecksum5(data)
	if crc.CheckChecksum5(data, cs+1) {
		t.Error("CheckChecksum5 should return false for non-matching checksum")
	}
}

func TestChecksum5_Deterministic(t *testing.T) {
	data := [9]byte{0xAA, 0x55, 0xFF, 0x00, 0x12, 0x34, 0x56, 0x78, 0x9A}
	cs1 := crc.CalculateChecksum5(data)
	cs2 := crc.CalculateChecksum5(data)
	if cs1 != cs2 {
		t.Errorf("Checksum5 not deterministic: %d vs %d", cs1, cs2)
	}
}
