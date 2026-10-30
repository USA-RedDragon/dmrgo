//go:build !tinygo

package rf

import "testing"

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
