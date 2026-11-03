package rf

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func elcTruth(t *testing.T) ([4][4]byte, [4]layer2.RawBurst) {
	t.Helper()
	_, ft := benchStream(t, 1)
	var f [4][4]byte
	var out [4]layer2.RawBurst
	for i, ts := range ft {
		if ts.kind == "voice" && ts.pos == enums.VoiceBurstE {
			for j := range 4 {
				b := ft[i-2*(3-j)].burst
				f[j], out[j] = b.EmbeddedData(), b
			}
			return f, out
		}
	}
	t.Fatal("no superframe")
	return f, out
}

func elcNoisy(f [4][4]byte, sigma float64, rng *rand.Rand) [4][embBits]int16 {
	var l [4][embBits]int16
	for i := range l {
		for j := range l[i] {
			s := 1.0
			if f[i][j/8]>>(7-j%8)&1 != 0 {
				s = -1
			}
			l[i][j] = int16(max(min(64*(s+sigma*rng.NormFloat64()), 32767), -32767))
		}
	}
	return l
}

func elcRun(t *testing.T, seed uint64, frames func(k int, rng *rand.Rand) [4][embBits]int16, n int) []bool {
	t.Helper()
	f, out := elcTruth(t)
	want, _ := strictEmbeddedLC(f)
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(seed, 7))
	var acc, sum [4][embBits]int16
	var accN int
	var accLC [9]byte
	st := elcStat{rho: elcSpreadCoh}
	got := make([]bool, n)
	for k := range n {
		cur := frames(k, rng)
		o := out
		lc, ok := d.decodeELC(&cur, &acc, &sum, &accN, &accLC, &o, true, &st)
		got[k] = ok && lc == want
	}
	return got
}

// TestELCAccumulatesFirstWord checks that the embedded LC decoder sums the
// fragments of every superframe of a call until the first word decodes. At
// this noise one superframe or two rarely decode and four nearly always do.
// It fails when fewer than 28 of 32 calls decode the word by their sixth
// superframe.
func TestELCAccumulatesFirstWord(t *testing.T) {
	t.Parallel()
	f, _ := elcTruth(t)
	n := 0
	for seed := range uint64(32) {
		got := elcRun(t, seed, func(_ int, rng *rand.Rand) [4][embBits]int16 { return elcNoisy(f, 1.4, rng) }, 6)
		for _, ok := range got {
			if ok {
				n++
				break
			}
		}
	}
	if n < 28 {
		t.Errorf("%d of 32 calls decode the embedded LC by the sixth superframe", n)
	}
}
