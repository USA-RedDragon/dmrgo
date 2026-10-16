package crc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func TestPackedCRCsMatchReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	for range 20000 {
		n := r.Intn(200)
		bits := make([]unpacked.Bit, n)
		for i := range bits {
			bits[i] = unpacked.Bit(r.Intn(2))
		}
		data := unpacked.Pack(bits)
		if got, want := crc.CRC7(data, n), ref.CRC7(bits); got != want {
			t.Fatalf("CRC7 of %d bits = %#x, want %#x", n, got, want)
		}
		if got, want := crc.CRC8(data, n), ref.CRC8(bits); got != want {
			t.Fatalf("CRC8 of %d bits = %#x, want %#x", n, got, want)
		}
		if got, want := crc.CRC9(data, n), ref.CRC9(bits); got != want {
			t.Fatalf("CRC9 of %d bits = %#x, want %#x", n, got, want)
		}
		w7, w8, w9 := uint8(r.Intn(256)), uint8(r.Intn(256)), uint16(r.Intn(1<<16))
		if got, want := crc.CheckCRC7(data, n, w7), ref.CheckCRC7(bits, w7); got != want {
			t.Fatalf("CheckCRC7 of %d bits = %v, want %v", n, got, want)
		}
		if got, want := crc.CheckCRC8(data, n, w8), ref.CheckCRC8(bits, w8); got != want {
			t.Fatalf("CheckCRC8 of %d bits = %v, want %v", n, got, want)
		}
		if got, want := crc.CheckCRC9(data, n, w9), ref.CheckCRC9(bits, w9); got != want {
			t.Fatalf("CheckCRC9 of %d bits = %v, want %v", n, got, want)
		}
	}
}

func TestPackedCRCsIgnoreTrailingBits(t *testing.T) {
	t.Parallel()

	data := []byte{0xA5, 0xFF, 0xFF}
	clean := []byte{0xA5, 0xC0, 0x00}
	if crc.CRC9(data, 10) != crc.CRC9(clean, 10) || crc.CRC8(data, 10) != crc.CRC8(clean, 10) || crc.CRC7(data, 10) != crc.CRC7(clean, 10) {
		t.Error("a CRC reads bits past n")
	}
}

func BenchmarkCRC(b *testing.B) {
	var d [23]byte
	b.Run("CRC7/4bits", func(b *testing.B) {
		for b.Loop() {
			d[0]++
			_ = crc.CRC7(d[:], 4)
		}
	})
	b.Run("CRC8/28bits", func(b *testing.B) {
		for b.Loop() {
			d[0]++
			_ = crc.CRC8(d[:], 28)
		}
	})
	b.Run("CRC9/87bits", func(b *testing.B) {
		for b.Loop() {
			d[0]++
			_ = crc.CRC9(d[:], 87)
		}
	})
	b.Run("CRC9/183bits", func(b *testing.B) {
		for b.Loop() {
			d[0]++
			_ = crc.CRC9(d[:], 183)
		}
	})
}
