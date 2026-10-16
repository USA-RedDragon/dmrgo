package trellis

import (
	"math/rand"
	"testing"
)

func BenchmarkDecode(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	_, clean := randomBlock(r)
	oneError := clean
	oneError[7] ^= 0x40
	for _, c := range []struct {
		name string
		in   [25]byte
	}{{"clean", clean}, {"one error", oneError}} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = Decode(c.in)
			}
		})
	}
}

func BenchmarkEncode(b *testing.B) {
	var data [18]byte
	b.ReportAllocs()
	for b.Loop() {
		data[0]++
		_ = Encode(data)
	}
}
