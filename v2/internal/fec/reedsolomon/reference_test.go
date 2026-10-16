package reedsolomon

import (
	"math/rand"
	"runtime"
	"sync"
	"testing"

	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/reedsolomon"
)

// The decode decision is a function of the syndromes only, and the syndromes
// of the words with zero data bytes cover all 2^24 values, so this test
// compares Decode with the reference decoder for every syndrome.
func TestDecodeMatchesReferenceForEverySyndrome(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("exhaustive")
	}
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := w; p < 1<<24; p += workers {
				var cw [12]byte
				cw[9], cw[10], cw[11] = byte(p>>16), byte(p>>8), byte(p)
				gd, gr := Decode(cw)
				wd, wr := ref.Decode(cw)
				if gd != wd || gr != wr || Check(cw) != ref.Check(cw) {
					errs <- "mismatch"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func TestDecodeMatchesReferenceRandom(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	for n := range 1_000_000 {
		var data [9]byte
		r.Read(data[:])
		cw := Encode(data)
		if cw != ref.Encode(data) {
			t.Fatalf("Encode(%x) = %x, want %x", data, cw, ref.Encode(data))
		}
		switch n % 4 {
		case 0:
		case 1:
			cw[r.Intn(12)] ^= byte(1 + r.Intn(255))
		case 2:
			for range 1 + r.Intn(8) {
				cw[r.Intn(12)] ^= 1 << r.Intn(8)
			}
		default:
			r.Read(cw[:])
		}
		gd, gr := Decode(cw)
		wd, wr := ref.Decode(cw)
		if gd != wd || gr != wr {
			t.Fatalf("Decode(%x) = %x %+v, want %x %+v", cw, gd, gr, wd, wr)
		}
	}
}

func BenchmarkCodec(b *testing.B) {
	data := [9]byte{0, 0, 0, 0x26, 0x0f, 0x30, 0xb3, 0x3c, 0x55}
	clean := Encode(data)
	bad := clean
	bad[4] ^= 0x10
	for _, c := range []struct {
		name string
		cw   [12]byte
	}{{"Decode/clean", clean}, {"Decode/1error", bad}} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = Decode(c.cw)
			}
		})
		b.Run("Ref"+c.name, func(b *testing.B) {
			for b.Loop() {
				_, _ = ref.Decode(c.cw)
			}
		})
	}
	b.Run("Encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			data[0]++
			_ = Encode(data)
		}
	})
}
