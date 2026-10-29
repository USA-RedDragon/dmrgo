package bptc_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func TestBPTC19696MatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(4))
	n := testmode.N(1_000_000, 1_000_000, 20_000)
	if testing.Short() {
		n = 50_000
	}
	type c196 struct {
		data     [12]byte
		reserved uint8
		random   bool
		noise    [25]byte
		nflips   int
		flips    [16]int
	}
	testmode.Cases(t, n, func(i int) c196 {
		var c c196
		r.Read(c.data[:])
		c.reserved = uint8(r.Intn(8))
		switch i % 4 {
		case 0:
			c.random = true
			r.Read(c.noise[:])
		case 1:
			c.nflips = r.Intn(4)
		default:
			c.nflips = r.Intn(16)
		}
		for k := range c.nflips {
			c.flips[k] = r.Intn(196)
		}
		return c
	}, func(c *c196) string {
		data, reserved := c.data, c.reserved
		refRes := [3]unpacked.Bit{unpacked.Bit(reserved & 1), unpacked.Bit(reserved >> 1 & 1), unpacked.Bit(reserved >> 2 & 1)}
		info := bptc.Encode196(data, reserved)
		refInfo := ref.Encode196WithReserved(unpacked.Unpack96(data), refRes)
		var want [25]byte
		unpacked.PackAt(want[:], 0, refInfo[:])
		if info != want {
			return fmt.Sprintf("Encode196(%x, %d) = %x, want %x", data, reserved, info, want)
		}
		if c.random {
			info = c.noise
		}
		for _, p := range c.flips[:c.nflips] {
			info[p/8] ^= 0x80 >> (p % 8)
		}
		unpacked.UnpackAt(refInfo[:], info[:], 0)
		got, gotRes, gotResult := bptc.Decode196(info)
		wantData, wantRes, wantResult := ref.Decode196WithReserved(refInfo)
		wantReserved := uint8(wantRes[0] | wantRes[1]<<1 | wantRes[2]<<2)
		if got != unpacked.Pack96(wantData) || gotRes != wantReserved || gotResult != wantResult {
			return fmt.Sprintf("Decode196(%x) = %x %d %+v, want %x %d %+v", info, got, gotRes, gotResult, unpacked.Pack96(wantData), wantReserved, wantResult)
		}
		return ""
	})
}

func BenchmarkBPTC19696(b *testing.B) {
	data := [12]byte{0x00, 0x2e, 0x44, 0x4d, 0x52, 0x47, 0x4f, 0x20, 0x54, 0x45, 0x53, 0x54}
	clean := bptc.Encode196(data, 0)
	bad := clean
	bad[5] ^= 0x10
	refClean := ref.Encode196(unpacked.Unpack96(data))
	var refBad [196]unpacked.Bit
	unpacked.UnpackAt(refBad[:], bad[:], 0)
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
	refData := unpacked.Unpack96(data)
	b.Run("RefEncode", func(b *testing.B) {
		for b.Loop() {
			refData[0] ^= 1
			_ = ref.Encode196(refData)
		}
	})
}
