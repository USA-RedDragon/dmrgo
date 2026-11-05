package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

// TestSoftExtEmbeddedLC checks at 5 dB that softExt recovers at least 80% of
// the embedded LCs and more than the Tier0 receiver without it, as rates at
// level testseed.Alpha.
func TestSoftExtEmbeddedLC(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	elc := func(a []int16, soft bool) (int, int) {
		var out []Burst
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: benchAudioHz, Tier: Tier0, OnBursts: each(func(b *Burst) { out = append(out, *b) })})
		if err != nil {
			t.Fatal(err)
		}
		if soft {
			r.ext = newSoftExt(0, 2)
		}
		must(r.writeAll(a))
		r.flushAll()
		s := scoreBench(truth, out)["elc"]
		return s.Exact, s.Truth
	}
	var hard, soft, n int
	for seed := uint64(41); seed < 49; seed++ {
		a := discAudio(noisyBaseband(t, slots, 1400, 5, seed), benchAudioHz)
		h, _ := elc(a, false)
		s, m := elc(a, true)
		hard, soft, n = hard+h, soft+s, n+m
	}
	t.Logf("embedded LCs recovered: %d hard, %d with softExt, of %d", hard, soft, n)
	if testseed.RateBelow(soft, n, 0.8) || !testseed.RateHigher(soft, n, hard, n) {
		t.Errorf("embedded LCs recovered: %d with softExt, %d without, of %d", soft, hard, n)
	}
}

func TestSoftExtLookbackClicks(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 30)
	a := discAudio(noisyBaseband(t, slots, 1400, 9, 604), benchAudioHz)
	t0 := scoreBench(truth, receiveAudioTier(t, a, Tier0))["rate12"]
	t1 := scoreBench(truth, receiveAudioTier(t, a, Tier1))["rate12"]
	if t1.Exact < t0.Exact {
		t.Errorf("rate 1/2 blocks recovered: %d of %d with Tier1, %d with Tier0", t1.Exact, t1.Truth, t0.Exact)
	}
}

func TestSoftExtLookback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	start := -1
	for i, ts := range truth {
		if i > 20 && ts.kind == "voice" && ts.pos == enums.VoiceBurstB {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("no voice burst B in the stream")
	}
	a := discAudio(noisyBaseband(t, slots[start:], 1400, 20, 7), benchAudioHz)
	first := func(lookback int) (start, b float64) {
		start, b = -1, -1
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: benchAudioHz, Tier: Tier0, OnBursts: each(func(x *Burst) {
			if start < 0 {
				start = x.Seconds
			}
			if b < 0 && x.Voice == enums.VoiceBurstB {
				b = x.Seconds
			}
		})})
		if err != nil {
			t.Fatal(err)
		}
		r.ext = newSoftExt(lookback, 2)
		must(r.writeAll(a))
		r.flushAll()
		return start, b
	}
	plain, _ := first(0)
	_, b := first(4)
	if b < 0 || b >= plain {
		t.Errorf("with lookback, the first voice burst B is at %.3f s, the first burst without at %.3f s", b, plain)
	}
}

func newSoftExt(lookback int, sps float32) *softExt {
	return newSoftExtStages(extAll, lookback, sps)
}
