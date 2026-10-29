package reedsolomon

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"

	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

func TestDecodeMatchesReferenceForEverySyndrome(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("exhaustive")
	}
	workers := runtime.GOMAXPROCS(0)
	step := workers * testmode.N(1, 1, 97)
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := w; p < 1<<24; p += step {
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
	type rsCase struct {
		data   [9]byte
		random bool
		noise  [12]byte
		nops   int
		pos    [8]int
		xor    [8]byte
	}
	checkCases(t, testmode.N(1_000_000, 1_000_000, 20_000), func(n int) rsCase {
		var c rsCase
		r.Read(c.data[:])
		switch n % 4 {
		case 0:
		case 1:
			c.nops = 1
			c.pos[0] = r.Intn(12)
			c.xor[0] = byte(1 + r.Intn(255))
		case 2:
			c.nops = 1 + r.Intn(8)
			for k := range c.nops {
				c.pos[k] = r.Intn(12)
				c.xor[k] = 1 << r.Intn(8)
			}
		default:
			c.random = true
			r.Read(c.noise[:])
		}
		return c
	}, func(c *rsCase) string {
		data := c.data
		cw := Encode(data)
		if cw != ref.Encode(data) {
			return fmt.Sprintf("Encode(%x) = %x, want %x", data, cw, ref.Encode(data))
		}
		for k := range c.nops {
			cw[c.pos[k]] ^= c.xor[k]
		}
		if c.random {
			cw = c.noise
		}
		gd, gr := Decode(cw)
		wd, wr := ref.Decode(cw)
		if gd != wd || gr != wr {
			return fmt.Sprintf("Decode(%x) = %x %+v, want %x %+v", cw, gd, gr, wd, wr)
		}
		return ""
	})
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
