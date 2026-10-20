package crc_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func crc7Of(bits []unpacked.Bit) uint8  { return crc.CRC7(unpacked.Pack(bits), len(bits)) }
func crc8Of(bits []unpacked.Bit) uint8  { return crc.CRC8(unpacked.Pack(bits), len(bits)) }
func crc9Of(bits []unpacked.Bit) uint16 { return crc.CRC9(unpacked.Pack(bits), len(bits)) }
func checkCRC7Of(bits []unpacked.Bit, want uint8) bool {
	return crc.CheckCRC7(unpacked.Pack(bits), len(bits), want)
}

func checkCRC8Of(bits []unpacked.Bit, want uint8) bool {
	return crc.CheckCRC8(unpacked.Pack(bits), len(bits), want)
}

func checkCRC9Of(bits []unpacked.Bit, want uint16) bool {
	return crc.CheckCRC9(unpacked.Pack(bits), len(bits), want)
}

func checkCRC7Codeword(bits []unpacked.Bit, mask uint8) bool {
	n := len(bits)
	if n < 8 {
		return false
	}
	return checkCRC7Of(bits[:n-7], uint8(unpacked.Uint(bits[n-7:]))^mask)
}

func checkCRC8Codeword(bits []unpacked.Bit) bool {
	n := len(bits)
	if n < 9 {
		return false
	}
	return checkCRC8Of(bits[:n-8], uint8(unpacked.Uint(bits[n-8:])))
}

func checkCRC9Codeword(bits []unpacked.Bit, mask uint16) bool {
	n := len(bits)
	if n < 10 {
		return false
	}
	return checkCRC9Of(bits[:n-9], uint16(unpacked.Uint(bits[n-9:]))^mask)
}

func checkCCITTCodeword(data []byte) bool {
	n := len(data)
	if n < 3 {
		return false
	}
	return crc.CheckCCITT(data[:n-2], uint16(data[n-2])<<8|uint16(data[n-1]))
}
