package rf

import "testing"

// TestCohExtVoice receives a BS stream at 5.2 dB Eb/N0 through iqRx with
// softExt, with and without the coherent stage, and checks that coherent
// detection again recovers far more whole voice frames: their C2 and C3
// bits have no FEC, so only the detection fixes them.
func TestCohExtVoice(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 4, 21)
	ambe := func(coh bool) float64 {
		o := tierOptions(TierDisc)
		o.IQDisc, o.IQHz, o.SoftExt, o.ExtAMBE, o.ExtOSD, o.ExtELC = true, discRate, true, true, true, true
		o.CohExt, o.PSPGain, o.LLRScale = coh, 0.3, 0.5
		var out []Burst
		r := mustNew(Config{Rate: basebandRate, Tier: TierDisc, Streaming: true, opt: &o, OnBurst: func(b *Burst) { out = append(out, *b) }})
		for k := 0; k < len(x); k += 4096 {
			r.WriteBaseband(x[k:min(k+4096, len(x))])
		}
		r.Flush()
		s := scoreBench(truth, out)["ambe"]
		return float64(s.Exact) / float64(max(s.Truth, 1))
	}
	soft, coh := ambe(false), ambe(true)
	t.Logf("whole voice frames: %.2f soft, %.2f coherent", soft, coh)
	if coh < soft+0.3 {
		t.Errorf("whole voice frames: %.2f coherent, %.2f soft", coh, soft)
	}
}
