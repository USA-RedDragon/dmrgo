package bptc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func TestCACHMatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	for n := range testmode.N(2_000_000, 2_000_000, 40_000) {
		info := r.Uint64() & (1<<36 - 1)
		var refInfo [36]unpacked.Bit
		unpacked.PutUint(refInfo[:], info)
		frags := bptc.EncodeCACH(info)
		refFrags := ref.EncodeCACH(refInfo)
		for i := range frags {
			if uint64(frags[i]) != unpacked.Uint(refFrags[i][:]) {
				t.Fatalf("EncodeCACH(%09x) = %05x, want %v", info, frags, refFrags)
			}
		}
		switch n % 3 {
		case 0:
			for range r.Intn(9) {
				p := r.Intn(68)
				frags[p/17] ^= 1 << (16 - p%17)
			}
		case 1:
			for i := range frags {
				frags[i] = r.Uint32() & 0x1FFFF
			}
		default:
		}
		for i := range frags {
			unpacked.PutUint(refFrags[i][:], uint64(frags[i]))
		}
		got, gotRes := bptc.DecodeCACH(frags)
		want, wantRes := ref.DecodeCACH(refFrags)
		if got != unpacked.Uint(want[:]) || gotRes != wantRes {
			t.Fatalf("DecodeCACH(%05x) = %09x %+v, want %v %+v", frags, got, gotRes, want, wantRes)
		}
	}
}

func BenchmarkCACH(b *testing.B) {
	const info = 0x9a5c3f1e2
	clean := bptc.EncodeCACH(info)
	bad := clean
	bad[1] ^= 1 << 3
	var refInfo [36]unpacked.Bit
	unpacked.PutUint(refInfo[:], info)
	refClean := ref.EncodeCACH(refInfo)
	refBad := refClean
	refBad[1][13] ^= 1
	b.Run("Decode/clean", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeCACH(clean)
		}
	})
	b.Run("Decode/1error", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = bptc.DecodeCACH(bad)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		v := uint64(info)
		for b.Loop() {
			v++
			_ = bptc.EncodeCACH(v & (1<<36 - 1))
		}
	})
	b.Run("RefDecode/clean", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeCACH(refClean)
		}
	})
	b.Run("RefDecode/1error", func(b *testing.B) {
		for b.Loop() {
			_, _ = ref.DecodeCACH(refBad)
		}
	})
	b.Run("RefEncode", func(b *testing.B) {
		for b.Loop() {
			refInfo[0] ^= 1
			_ = ref.EncodeCACH(refInfo)
		}
	})
}
