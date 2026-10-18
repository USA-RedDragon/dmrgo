//go:build !tinygo

package rf

// discDefaultTier is the tier of a DiscConfig with TierDefault for NewDisc.
const discDefaultTier = Tier1

// addSoft adds to r the stages of tier t, Tier1 or Tier2.
func (r *DiscReceiver) addSoft(t Tier) error {
	opt := discTierOptions(t)
	r.addExt(&opt)
	return nil
}

// addExt adds to r the stages of opt after the core.
func (r *DiscReceiver) addExt(opt *options) {
	var ph *discPhase
	if opt.CohExt {
		// The core runs at discRate on the discriminator of the ring.
		hz, out := r.hz(), r.out
		_ = r.Init(DiscConfig{Hz: discRate, OnBurst: r.onBurst, opt: opt})
		r.out = out
		ph = newDiscPhase(r, hz)
	}
	x := softExtOf(opt, 2*r.tm.step)
	if ph != nil {
		x.ph, x.coh = ph, newCohStageOf(opt)
		x.coh.ph, x.coh.f, ph.coh = ph, &r.f, x.coh
		x.coh.easySNR = discEasySNR
		r.signMiss = leanExtSignMiss
	}
	r.ext = x
}

// write runs the samples x through the core, or through the phase of
// Tier2, part by part, which gives the core the samples to run. The stage
// is a softExt, whose methods the escape analysis follows, so that the
// idle samples of flush stay on the stack.
func (r *DiscReceiver) write(x []int16) {
	e, _ := r.ext.(*softExt)
	if e == nil || e.ph == nil {
		r.run(x)
		return
	}
	for len(x) > 0 {
		n, y := e.ph.input(x)
		r.run(y)
		x = x[n:]
	}
}

// inputHz returns the rate of the input of r: the core of Tier2 runs at
// discRate on input at any rate.
func (r *DiscReceiver) inputHz() int {
	if e, _ := r.ext.(*softExt); e != nil && e.ph != nil {
		return e.ph.inputHz()
	}
	return r.hz()
}
