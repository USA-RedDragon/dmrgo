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
