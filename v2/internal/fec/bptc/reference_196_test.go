package bptc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
)

func TestBPTC19696MatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(4))
	n := 1_000_000
	if testing.Short() {
		n = 50_000
	}
	for i := range n {
		var data [12]byte
		r.Read(data[:])
		reserved := uint8(r.Intn(8))
		refRes := [3]bit.Bit{bit.Bit(reserved & 1), bit.Bit(reserved >> 1 & 1), bit.Bit(reserved >> 2 & 1)}
		info := bptc.Encode196(data, reserved)
		refInfo := ref.Encode196WithReserved(bit.Unpack96(data), refRes)
		var want [25]byte
		packed.Pack(want[:], 0, refInfo[:])
		if info != want {
			t.Fatalf("Encode196(%x, %d) = %x, want %x", data, reserved, info, want)
		}
		switch i % 4 {
		case 0:
			r.Read(info[:])
		case 1:
			for range r.Intn(4) {
				p := r.Intn(196)
				info[p/8] ^= 0x80 >> (p % 8)
			}
		default:
			for range r.Intn(16) {
				p := r.Intn(196)
				info[p/8] ^= 0x80 >> (p % 8)
			}
		}
		packed.Unpack(refInfo[:], info[:], 0)
		got, gotRes, gotResult := bptc.Decode196(info)
		wantData, wantRes, wantResult := ref.Decode196WithReserved(refInfo)
		wantReserved := uint8(wantRes[0] | wantRes[1]<<1 | wantRes[2]<<2)
		if got != bit.Pack96(wantData) || gotRes != wantReserved || gotResult != wantResult {
			t.Fatalf("Decode196(%x) = %x %d %+v, want %x %d %+v", info, got, gotRes, gotResult, bit.Pack96(wantData), wantReserved, wantResult)
		}
	}
}

func BenchmarkBPTC19696(b *testing.B) {
	data := [12]byte{0x00, 0x2e, 0x44, 0x4d, 0x52, 0x47, 0x4f, 0x20, 0x54, 0x45, 0x53, 0x54}
	clean := bptc.Encode196(data, 0)
	bad := clean
	bad[5] ^= 0x10
	refClean := ref.Encode196(bit.Unpack96(data))
	var refBad [196]bit.Bit
	packed.Unpack(refBad[:], bad[:], 0)
	b.Run("Decode/clean", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _, _ = bptc.Decode196(clean)
		}
	})
	b.Run("Decode/1error", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _, _ = bptc.Decode196(bad)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			data[0]++
			_ = bptc.Encode196(data, 0)
		}
	})
	b.Run("RefDecode/clean", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.Decode196(refClean)
		}
	})
	b.Run("RefDecode/1error", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.Decode196(refBad)
		}
	})
	refData := bit.Unpack96(data)
	b.Run("RefEncode", func(b *testing.B) {
		for b.Loop() {
			refData[0] ^= 1
			_ = ref.Encode196(refData)
		}
	})
}
