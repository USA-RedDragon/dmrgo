//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestCoreTier3(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 4)
	x := noisyBaseband(slots, 1400, 12, 1)
	iq := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8}, coreIQ3Options())
	var got []Burst
	for k := 0; k < len(x); k += 4096 {
		got = append(got, must(iq.writeBB(x[k:min(k+4096, len(x))]))...)
	}
	got = append(got, iq.flushAll()...)
	if exact, total := exactFrames(slots, got); exact != total || total == 0 {
		t.Errorf("I/Q: %d of %d voice frames exact", exact, total)
	}
	a := discAudio(x, 9600)
	ar := coreAudio3(t, AudioRxConfig{Rate: 9600, Tier: Tier3})
	got = ar.AppendFlush(ar.AppendBursts(nil, a))
	if exact, total := exactFrames(slots, got); exact != total || total == 0 {
		t.Errorf("audio: %d of %d voice frames exact", exact, total)
	}
	if e, _ := ar.ext.(*softExt); e == nil || e.coh == nil || e.coh.ex == nil || !e.ph.useHeld {
		t.Error("audio core Tier 3 runs without the exact stage and the held unit")
	}
}

func coreAudio3(t testing.TB, cfg AudioRxConfig) *AudioReceiver {
	t.Helper()
	opt := coreAudio3Options()
	r := &AudioReceiver{}
	r.configure(cfg, &opt)
	r.designResample(cfg.Tier)
	if err := r.addSoft(&opt); err != nil {
		t.Fatal(err)
	}
	r.attachFix(true)
	if n := streamBursts(opt.ExtLookback); cfg.OnBursts != nil && n > 0 {
		r.bursts.out = make([]Burst, 0, n)
	}
	return r
}

func TestCohSlotFit(t *testing.T) {
	t.Parallel()
	opt := coreIQ3Options()
	if !opt.CohSlotFit {
		t.Fatal("core Tier 3 I/Q runs without the slot timing fit")
	}
	s := newCohStageOf(&opt)
	rng := rand.New(rand.NewPCG(1, 2))
	period := float64(slotSymbols) * s.sps * (1 + 500e-6)
	var q float64
	n := 0
	for k := int64(0); k < 400; k += 2 {
		truth := 1e6 + period*float64(k)
		pt, ok := s.slotPredict(k)
		s.t0 = truth + 0.2*rng.NormFloat64()
		s.slotMeasure(k, pt, ok)
		if k >= 100 {
			q += (s.t0 - truth) * (s.t0 - truth)
			n++
		}
	}
	if rms := math.Sqrt(q / float64(n)); rms > 0.08 {
		t.Errorf("slot fit timing rms %.3f samples from measurements of rms 0.2", rms)
	}
	pt, ok := s.slotPredict(400)
	if !ok {
		t.Fatal("no slot timing prediction after 200 measurements")
	}
	s.t0 = pt + 3
	s.slotMeasure(400, pt, ok)
	if math.Abs(s.t0-pt-3) > 1e-9 {
		t.Errorf("a 3 sample timing jump moved the fit to %.3f instead of restarting it", s.t0-pt)
	}
}
