package rf

import "testing"

func TestCohExtVoice(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 4, 21)
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
			s := scoreBench(truth, receiveBaseband(sig.noisy(cn, seed+1), Tier2))["ambe"]
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
