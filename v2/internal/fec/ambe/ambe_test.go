package ambe

import (
	"fmt"
	"math/rand"
	"testing"

	refambe "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func TestEncodeMatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	checkCases(t, 1<<20, func(n int) [7]byte {
		var frame [7]byte
		r.Read(frame[:])
		frame[6] &= 0x80
		if n < 4096 {
			frame[0], frame[1] = byte(n>>4), byte(n<<4)|frame[1]&0x0F
		}
		return frame
	}, func(frame *[7]byte) string {
		var info [49]unpacked.Bit
		copy(info[:], unpacked.Unpack(frame[:]))
		want := refambe.Encode(info)
		if got := Encode(*frame); got != [9]byte(unpacked.Pack(want[:])) {
			return fmt.Sprintf("Encode(%x) = %x, want %x", *frame, got, unpacked.Pack(want[:]))
		}
		return ""
	})
}

func TestDecodeMatchesReference(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	for n := range 1 << 21 {
		var slot [9]byte
		if n%2 == 0 {
			r.Read(slot[:])
		} else {
			var frame [7]byte
			r.Read(frame[:])
			frame[6] &= 0x80
			slot = Encode(frame)
			for range n % 9 {
				i := r.Intn(72)
				slot[i/8] ^= 0x80 >> (i % 8)
			}
		}
		var in [72]unpacked.Bit
		copy(in[:], unpacked.Unpack(slot[:]))
		wantInfo, wantRes := refambe.Decode(in)
		got, res := Decode(slot)
		if got != [7]byte(unpacked.Pack(wantInfo[:])) || res != wantRes {
			t.Fatalf("Decode(%x) = %x %+v, want %x %+v", slot, got, res, unpacked.Pack(wantInfo[:]), wantRes)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	slot := Encode([7]byte{1, 2, 3, 4, 5, 6, 0x80})
	slot[1] ^= 0x08
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Decode(slot)
	}
}

func BenchmarkEncode(b *testing.B) {
	frame := [7]byte{1, 2, 3, 4, 5, 6, 0x80}
	b.ReportAllocs()
	for b.Loop() {
		frame[0]++
		_ = Encode(frame)
	}
}

func TestDecodeMatchesReferenceForEverySeed(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(6))
	for a := range 4096 {
		for e := range 5 {
			var frame [7]byte
			r.Read(frame[:])
			frame[6] &= 0x80
			frame[0], frame[1] = byte(a>>4), byte(a<<4)|frame[1]&0x0F
			slot := Encode(frame)
			for _, p := range r.Perm(23)[:e] {
				c, j := 1, 6+p
				if p >= 12 {
					c, j = 2, p-12
				}
				i := j/2*8 + j%2*4 + c
				slot[i/8] ^= 0x80 >> (i % 8)
			}
			if na, nb := Errors(slot); e <= 3 && (na != 0 || nb != e) {
				t.Fatalf("seed %#x: Errors = %d, %d after %d errors in section B", a, na, nb, e)
			}
			var in [72]unpacked.Bit
			copy(in[:], unpacked.Unpack(slot[:]))
			wantInfo, wantRes := refambe.Decode(in)
			got, res := Decode(slot)
			if got != [7]byte(unpacked.Pack(wantInfo[:])) || res != wantRes {
				t.Fatalf("Decode(%x) = %x %+v, want %x %+v", slot, got, res, unpacked.Pack(wantInfo[:]), wantRes)
			}
			if e <= 3 && got != frame {
				t.Fatalf("seed %#x: Decode(%x) = %x, want %x", a, slot, got, frame)
			}
		}
	}
}
