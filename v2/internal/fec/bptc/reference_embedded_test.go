package bptc_test

import (
	"fmt"
	"math/bits"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func evenInfo(info [10]byte) bool {
	w := 0
	for _, b := range info[:9] {
		w += bits.OnesCount8(b)
	}
	return w%2 == 0
}

func refEvenInfo(info [77]unpacked.Bit) bool {
	var p [10]byte
	unpacked.PackAt(p[:], 0, info[:])
	return evenInfo(p)
}

func TestEmbeddedMatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	n := testmode.N(200_000, 200_000, 10_000)
	if testing.Short() {
		n = 20_000
	}
	type cEmb struct {
		info   [10]byte
		random bool
		noise  [4]uint32
		nflips int
		flips  [12]int
	}
	checkCases(t, n, func(i int) cEmb {
		var c cEmb
		r.Read(c.info[:])
		c.info[9] &= 0xF8
		if i%5 == 4 {
			c.random = true
			for k := range c.noise {
				c.noise[k] = r.Uint32()
			}
		} else {
			c.nflips = r.Intn(12)
			for k := range c.nflips {
				c.flips[k] = r.Intn(128)
			}
		}
		return c
	}, func(c *cEmb) string {
		info := c.info
		var refInfo [77]unpacked.Bit
		unpacked.UnpackAt(refInfo[:], info[:], 0)
		frags := bptc.EncodeEmbedded(info)
		refFrags := ref.EncodeEmbedded(refInfo)
		for k := range frags {
			if uint64(frags[k]) != unpacked.Uint(refFrags[k][:]) {
				return fmt.Sprintf("EncodeEmbedded(%x) = %08x, want %v", info, frags, refFrags)
			}
		}
		if c.random {
			frags = c.noise
		}
		for _, p := range c.flips[:c.nflips] {
			frags[p/32] ^= 1 << (31 - p%32)
		}
		for k := range frags {
			unpacked.PutUint(refFrags[k][:], uint64(frags[k]))
		}
		cmp := func(name string, got [10]byte, gotRes, wantRes any, want [77]unpacked.Bit) string {
			var w [10]byte
			unpacked.PackAt(w[:], 0, want[:])
			if got != w || gotRes != wantRes {
				return fmt.Sprintf("%s(%08x) = %x %+v, want %x %+v", name, frags, got, gotRes, w, wantRes)
			}
			return ""
		}
		got, gotRes := bptc.DecodeEmbedded(frags)
		want, wantRes := ref.DecodeEmbedded(refFrags)
		if msg := cmp("DecodeEmbedded", got, gotRes, wantRes, want); msg != "" {
			return msg
		}
		got, gotRes = bptc.DecodeEmbeddedWithCheck(frags, evenInfo)
		want, wantRes = ref.DecodeEmbeddedWithCheck(refFrags, refEvenInfo)
		return cmp("DecodeEmbeddedWithCheck", got, gotRes, wantRes, want)
	})
}

func BenchmarkEmbedded(b *testing.B) {
	info := [10]byte{0, 0, 0, 0x26, 0x0f, 0x30, 0xb3, 0x3c, 0x55, 0xa0}
	clean := bptc.EncodeEmbedded(info)
	bad := clean
	bad[2] ^= 1 << 5
	var refInfo [77]unpacked.Bit
	unpacked.UnpackAt(refInfo[:], info[:], 0)
	refClean := ref.EncodeEmbedded(refInfo)
	refBad := refClean
	refBad[2][26] ^= 1
	b.Run("Decode/clean", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeEmbedded(clean)
		}
	})
	b.Run("Decode/1error", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeEmbedded(bad)
		}
	})
	noise := [4]uint32{0x9e3779b9, 0x7f4a7c15, 0xf39cc060, 0x5ced2a4b}
	b.Run("Decode/noise", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeEmbedded(noise)
		}
	})
	r := rand.New(rand.NewSource(7))
	var hard [64][4]uint32
	for i := range hard {
		var info [10]byte
		r.Read(info[:])
		info[9] &= 0xF8
		hard[i] = bptc.EncodeEmbedded(info)
		for _, p := range r.Perm(128)[:4+i%5] {
			hard[i][p/32] ^= 1 << (31 - p%32)
		}
	}
	b.Run("Decode/4to8errors", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			_, _ = bptc.DecodeEmbeddedWithCheck(hard[i&63], evenInfo)
			i++
		}
	})
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			info[0]++
			_ = bptc.EncodeEmbedded(info)
		}
	})
	b.Run("RefDecode/clean", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeEmbedded(refClean)
		}
	})
	b.Run("RefDecode/1error", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeEmbedded(refBad)
		}
	})
	b.Run("RefEncode", func(b *testing.B) {
		for b.Loop() {
			refInfo[0] ^= 1
			_ = ref.EncodeEmbedded(refInfo)
		}
	})
}
