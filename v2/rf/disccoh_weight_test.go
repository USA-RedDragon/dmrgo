//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func discPhaseOf(t *testing.T, r *AudioReceiver) *discPhase {
	t.Helper()
	e, _ := r.ext.(*softExt)
	if e == nil || e.ph == nil {
		t.Fatal("no coherent audio stage")
	}
	return e.ph
}

func TestDiscPhaseHeldUnit(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	genie := 1 / audioGain
	for _, cn := range []float64{40, 12, 7, 5} {
		for _, hz := range []int{9600, 24000, 48000} {
			for seed := uint64(1); seed <= 4; seed++ {
				a := discAudio(noisyBaseband(slots, 1400, cn, seed), hz)
				r := mustAudio(AudioRxConfig{Rate: hz, Tier: Tier2})
				r.AppendFlush(r.AppendBursts(nil, a))
				p := discPhaseOf(t, r)
				if e := p.cohUnit()/genie - 1; math.Abs(e) > 0.03 {
					t.Errorf("%g dB at %d Hz, seed %d: held %.4f Hz per unit, genie %.4f", cn, hz, seed, p.cohUnit(), genie)
				}
			}
		}
	}
}

func TestDiscPhaseClickWeights(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	const hz = 9600
	for _, cn := range []float64{40, 7} {
		a := discAudio(noisyBaseband(slots, 1400, cn, 5), hz)
		spikes := map[int]bool{}
		for i := len(a) - 3000; i < len(a)-100; i += 487 {
			a[i] = int16(audioOffset + (1400+4500)*audioGain)
			spikes[i] = true
		}
		r := mustAudio(AudioRxConfig{Rate: hz, Tier: Tier2})
		r.AppendFlush(r.AppendBursts(nil, a))
		p := discPhaseOf(t, r)
		if p.sumN < int64(len(a)) {
			t.Fatalf("%g dB: %d input samples, want at least %d", cn, p.sumN, len(a))
		}
		mean := audioOffset + 1400*audioGain
		near := func(i int, f func(int) bool) bool {
			for k := i - discClickSpan; k <= i+discClickSpan; k++ {
				if k >= 0 && k < len(a) && f(k) {
					return true
				}
			}
			return false
		}
		genie := func(k int) bool { return math.Abs(float64(a[k])-mean)/audioGain > discClickHz }
		var miss, bad, n int
		for i := int(p.sumN-p.size) + 64; i < len(a); i++ {
			tr := float64(int64(i)*p.num) / float64(p.den)
			w := p.cohWeight(tr, tr)
			if spikes[i] && w != discClickWeight {
				t.Errorf("%g dB: sample %d of a spike weighs %g", cn, i, w)
			}
			if w == 0 {
				continue
			}
			n++
			switch g := near(i, genie); {
			case g && w == 1:
				miss++
			case !g && w != 1:
				bad++
			}
		}
		if miss+bad > n/100 {
			t.Errorf("%g dB: %d of %d samples weigh 1 next to a genie click, %d weigh less with none", cn, miss, n, bad)
		}
		t.Logf("%g dB: %d samples, %d missed, %d extra, held %.4f", cn, n, miss, bad, p.held)
	}
}
