package ambe

import (
	"math/bits"
	"math/rand"
	"testing"
)

func TestRegenerate(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	for n := range 1 << 20 {
		var frame [7]byte
		r.Read(frame[:])
		frame[6] &= 0x80
		clean := Encode(frame)
		var mask [9]byte
		for range n % 9 {
			i := r.Intn(72)
			mask[i/8] ^= 0x80 >> (i % 8)
		}
		m0, m1, m2, m3 := columns(&mask)
		nA := bits.OnesCount32(m0<<6 | m1>>12)
		nB := bits.OnesCount32((m1&0xFFF)<<11 | m2>>7)
		var c [9]byte
		mC2 := m2 & 0x7F
		for i := range c {
			sh := 16 - 2*i
			c[i] = byte((mC2>>sh&2<<4 | m3>>sh&2<<3 | mC2>>sh&1<<1 | m3>>sh&1) & 0xFF)
		}
		var in, want [9]byte
		for i := range in {
			in[i], want[i] = clean[i]^mask[i], clean[i]^c[i]
		}
		got, ok := Regenerate(in)
		switch {
		case !ok && got != in:
			t.Fatalf("Regenerate(%x) = %x, false: changed the slot", in, got)
		case nA == 4 && ok:
			t.Fatalf("4 errors in section A %x: Regenerate accepted %x", mask, got)
		case nA > 3 || nB > 3:
		case ok != (nA < 2 || nA+nB < 6):
			t.Fatalf("%d+%d errors %x: Regenerate ok %v", nA, nB, mask, ok)
		case ok && got != want:
			t.Fatalf("%d+%d errors %x: Regenerate(%x) = %x, want %x", nA, nB, mask, in, got, want)
		}
		if ok {
			if f, res := Decode(got); f != frameOf(in) || res.ErrorsCorrected != 0 || res.Uncorrectable {
				t.Fatalf("Decode(Regenerate(%x)) = %x %+v, want %x", in, f, res, frameOf(in))
			}
		}
	}
}

func frameOf(slot [9]byte) [7]byte {
	f, _ := Decode(slot)
	return f
}

func BenchmarkRegenerate(b *testing.B) {
	slot := Encode([7]byte{1, 2, 3, 4, 5, 6, 0x80})
	slot[1] ^= 0x08
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Regenerate(slot)
	}
}
