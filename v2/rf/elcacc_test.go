package rf

import (
	"math/bits"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
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

func elcRef(e *elcState, sigma float64, rng *rand.Rand) {
	e.binN, e.binE = [4][17]int32{}, [4][17]int32{}
	for f := range 4 {
		for range 157 {
			v := int16(max(min(64*(1+sigma*rng.NormFloat64()), 32767), -32767))
			b := bits.Len32(elcMag(v))
			e.binN[f][b]++
			if v < 0 {
				e.binE[f][b]++
			}
		}
	}
}

func elcCall(t *testing.T, src uint32) [4][4]byte {
	t.Helper()
	f := layer2.EncodeEmbeddedLC(pdu.LCBytes{0, 0, 0, 0, 0, 9, byte(src >> 16), byte(src >> 8), byte(src)})
	if lc, ok := strictEmbeddedLC(f); !ok || lc[8] != byte(src) {
		t.Fatal("embedded LC encode")
	}
	return f
}

func elcRun(t *testing.T, seed uint64, sigma float64, n int) (right, wrong []bool) {
	t.Helper()
	f, out := elcTruth(t)
	want, _ := strictEmbeddedLC(f)
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(seed, 7))
	var sum [4][embBits]int16
	var e elcState
	right, wrong = make([]bool, n), make([]bool, n)
	for k := range n {
		cur := elcNoisy(f, sigma, rng)
		elcRef(&e, sigma, rng)
		e.test = cur
		o := out
		lc, ok := d.decodeELC(&cur, &e, &sum, &o)
		right[k], wrong[k] = ok && lc == want, ok && lc != want
	}
	return right, wrong
}

// TestELCAccumulatesFirstWord checks that the embedded LC decoder sums the
// fragments of every superframe of a call until the first word decodes. At
// this noise one superframe alone does not decode.
// It fails when fewer than 28 of 32 calls decode the word by their sixth
// superframe.
func TestELCAccumulatesFirstWord(t *testing.T) {
	t.Parallel()
	n := 0
	for seed := range uint64(32) {
		right, _ := elcRun(t, seed, 0.8, 6)
		if slices.Contains(right, true) {
			n++
		}
	}
	if n < 28 {
		t.Errorf("%d of 32 calls decode the embedded LC by the sixth superframe", n)
	}
}

func TestELCRefusesAtHighNoise(t *testing.T) {
	t.Parallel()
	right, wrong := 0, 0
	for seed := range uint64(32) {
		r, w := elcRun(t, seed, 1.4, 6)
		for k := range r {
			if r[k] {
				right++
			}
			if w[k] {
				wrong++
			}
		}
	}
	if right > 0 || wrong > 0 {
		t.Errorf("at 24%% bit errors %d of 192 superframes give the right embedded LC and %d a wrong one, want none of either", right, wrong)
	}
}

func TestELCTalkerChangeNotStale(t *testing.T) {
	t.Parallel()
	const calls, talkers, per = 32, 3, 4
	ok, stale := 0, 0
	for seed := range uint64(calls) {
		rng := rand.New(rand.NewPCG(seed, 11))
		d := newSoftDecoder()
		var e elcState
		var sum [4][embBits]int16
		var o [4]layer2.RawBurst
		seen := make([][9]byte, 0, talkers)
		for range talkers {
			f := elcCall(t, 990000+uint32(rng.IntN(1000)))
			want, _ := strictEmbeddedLC(f)
			for range per {
				cur := elcNoisy(f, 0.7, rng)
				elcRef(&e, 0.7, rng)
				e.test = cur
				lc, good := d.decodeELC(&cur, &e, &sum, &o)
				if !good {
					continue
				}
				ok++
				if lc != want && slices.Contains(seen, lc) {
					stale++
				}
			}
			seen = append(seen, want)
		}
	}
	if n := calls * talkers * per; stale > 0 || ok < n*50/100 {
		t.Errorf("%d of %d superframes decode, %d of them to an earlier talker's LC", ok, n, stale)
	}
}

func TestELCNotWrong(t *testing.T) {
	t.Parallel()
	const calls, per = 64, 12
	ok, wrong := 0, 0
	for seed := range uint64(calls) {
		rng := rand.New(rand.NewPCG(seed, 13))
		d := newSoftDecoder()
		var e elcState
		var sum [4][embBits]int16
		var o [4]layer2.RawBurst
		f := elcCall(t, uint32(rng.IntN(1<<24)))
		want, _ := strictEmbeddedLC(f)
		for range per {
			cur := elcNoisy(f, 0.7, rng)
			elcRef(&e, 0.7, rng)
			e.test = cur
			if lc, good := d.decodeELC(&cur, &e, &sum, &o); good {
				ok++
				if lc != want {
					wrong++
				}
			}
		}
	}
	if n := calls * per; wrong > 0 || ok < n*70/100 {
		t.Errorf("%d of %d superframes decode, %d of them wrong", ok, n, wrong)
	}
}
