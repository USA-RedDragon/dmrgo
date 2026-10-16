package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func BenchmarkSingleBurst(b *testing.B) {
	const info = 0x5a3
	clean := bptc.EncodeSingleBurst(info, true)
	bad := clean ^ 1<<20
	var refInfo [11]unpacked.Bit
	unpacked.PutUint(refInfo[:], info)
	refClean := ref.EncodeSingleBurst(refInfo, true)
	refBad := refClean
	refBad[11] ^= 1
	b.Run("Decode/clean", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeSingleBurst(clean, true)
		}
	})
	b.Run("Decode/1error", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeSingleBurst(bad, true)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		v := uint16(0)
		for b.Loop() {
			v++
			_ = bptc.EncodeSingleBurst(v&0x7FF, true)
		}
	})
	b.Run("RefDecode/clean", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeSingleBurst(refClean, true)
		}
	})
	b.Run("RefDecode/1error", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeSingleBurst(refBad, true)
		}
	})
	b.Run("RefEncode", func(b *testing.B) {
		for b.Loop() {
			refInfo[0] ^= 1
			_ = ref.EncodeSingleBurst(refInfo, true)
		}
	})
}
