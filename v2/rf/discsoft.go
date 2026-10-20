//go:build !tinygo

package rf

const discDefaultTier = Tier1

func (r *DiscReceiver) addSoft(opt *options) error {
	r.addExt(opt)
	return nil
}

func (r *DiscReceiver) addExt(opt *options) {
	var ph *discPhase
	switch {
	case opt.CohExt && opt.CohLazy:
		ph = newDiscPhase(r, r.hz(), true, opt.cohRing())
	case opt.CohExt:
		hz, out := r.hz(), r.out
		_ = r.Init(DiscConfig{Hz: discRate, OnBurst: r.onBurst, opt: opt})
		r.out = out
		ph = newDiscPhase(r, hz, false, opt.cohRing())
	}
	x := softExtOf(opt, 2*r.tm.step)
	if ph != nil {
		x.ph, x.coh = ph, newCohStageOf(opt)
		x.coh.ph, x.coh.f, ph.coh = ph, &r.f, x.coh
		x.coh.easySNR = discEasySNR
		if ph.lazy {
			x.coh.dt = lazyDT(r.hz())
		}
		if !opt.CohLocked {
			r.signMiss = opt.signMiss()
		}
	}
	r.ext = x
}

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

func (r *DiscReceiver) inputHz() int {
	if e, _ := r.ext.(*softExt); e != nil && e.ph != nil && !e.ph.lazy {
		return e.ph.inputHz()
	}
	return r.hz()
}
