package rf

import (
	"math"
	"testing"
)

func TestCohExtVoice(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(t, slots, 1400, 4, 21)
	ambe := func(coh bool) float64 {
		o := discOptions()
		o.IQDisc, o.IQHz, o.SoftExt, o.ExtAMBE, o.ExtOSD, o.ExtELC = true, discRate, true, true, true, true
		o.CohExt, o.PSPGain, o.LLRScale = coh, 0.2, 0.5
		var out []Burst
		r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCU8}, o)
		for k := 0; k < len(x); k += 4096 {
			must(r.writeBB(x[k:min(k+4096, len(x))]))
		}
		r.flushAll()
		s := scoreBench(truth, out)["ambe"]
		return float64(s.Exact) / float64(max(s.Truth, 1))
	}
	soft, coh := ambe(false), ambe(true)
	t.Logf("whole voice frames: %.2f soft, %.2f coherent", soft, coh)
	if coh < soft+0.3 {
		t.Errorf("whole voice frames: %.2f coherent, %.2f soft", coh, soft)
	}
}

func TestCohTier2VoiceAboveEasySNR(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 30)
	sig := cleanBaseband(slots, 1400)
	lost := func(cn float64) int {
		n := 0
		for seed := range uint64(8) {
			s := scoreBench(truth, receiveBaseband(sig.noisy(t, cn, seed+1), Tier2))["ambe"]
			n += s.Truth - s.Exact
		}
		return n
	}
	n5, n6 := lost(5), lost(6)
	t.Logf("Tier2 lost %d voice frames at C/N 5 dB, %d at 6 dB", n5, n6)
	if n6 >= n5 {
		t.Errorf("Tier2 lost %d voice frames at C/N 6 dB, %d at 5 dB", n6, n5)
	}
}

func TestCohPilotFromInterior(t *testing.T) {
	t.Parallel()
	o := tierOptions(Tier2)
	o.CohExact, o.PhaseStates, o.LogMAP = true, 52, true
	exact := pilotRuns(newCohStageOf(&o))
	o = tierOptions(Tier2)
	near := pilotRuns(newCohStageOf(&o))
	lo, hi := syncFirstSym, syncFirstSym+syncSymbols-1
	fwd, bwd := [4]float32{1, 0, 0, 0}, [4]float32{0, 0, 0, 1}
	for _, c := range []struct {
		j           int
		exact, near [4]float32
	}{
		{lo + 2, bwd, fwd}, {lo + 3, bwd, fwd}, {hi - 3, fwd, bwd}, {hi - 2, fwd, bwd},
		{lo + 5, fwd, fwd}, {hi - 5, bwd, bwd}, {lo - 1, bwd, bwd}, {hi + 1, fwd, fwd},
	} {
		if exact.app[c.j] != c.exact {
			t.Errorf("exact stage, symbol %d: APP %v, want %v", c.j, exact.app[c.j], c.exact)
		}
		if near.app[c.j] != c.near {
			t.Errorf("per-survivor stage, symbol %d: APP %v, want %v", c.j, near.app[c.j], c.near)
		}
	}
}

func pilotRuns(s *cohStage) *cohStage {
	lo, hi := syncFirstSym, syncFirstSym+syncSymbols-1
	s.nseg = 2
	s.segs = [2][2]int{{lo, lo + 3}, {hi - 3, hi}}
	for _, sg := range s.segs {
		for j := sg[0]; j <= sg[1]; j++ {
			s.isK[j] = true
		}
	}
	for i := range s.dist {
		s.dist[i] = math.MaxInt
	}
	d := s.model.d
	fwd, bwd := []float32{1, 0, 0, 0}, []float32{0, 0, 0, 1}
	s.g0, s.dir, s.steps = lo, 1, slotSymbols
	for k := range slotSymbols - lo - 2*d {
		s.runKeep(k, fwd)
	}
	s.g0, s.dir = hi, -1
	for k := range hi - cachSymbols - 2*d {
		s.runKeep(k, bwd)
	}
	return s
}
