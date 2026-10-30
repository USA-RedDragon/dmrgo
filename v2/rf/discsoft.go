//go:build !tinygo

package rf

import "math"

const discDefaultTier = Tier1

func (r *discRx) addSoft(opt *options) error {
	r.addExt(opt)
	return nil
}

func (r *discRx) addExt(opt *options) {
	if opt.UnionTwins != 0 {
		r.addUnion(opt)
		return
	}
	var ph *discPhase
	if opt.CohExt {
		hz := r.hz()
		r.setup(discSetup{Hz: discRate, OnBurst: r.onBurst, opt: opt, pol: r.f.polBits&polOn != 0}, r.f.dec)
		r.lagTicks -= int32(math.Round(float64(discRate-hz) * 256 / float64(hz)))
		ph = newDiscPhase(r, hz, opt.cohRing())
	}
	x := softExtOf(opt, 2*r.tm.step)
	if ph != nil {
		x.ph = ph
		x.setCoh(newCohStageOf(opt))
		x.coh.src, x.coh.ph, ph.coh = ph, ph, x.coh
		ph.useHeld = opt.CohExact
		x.coh.easySNR = discEasySNR
		r.signMiss = opt.signMiss()
	}
	r.ext = x
}

func (r *discRx) attachFix(restore bool) {
	e, _ := r.ext.(*softExt)
	if !restore || e == nil {
		return
	}
	a := newACStage(r.inputHz())
	e.ac = a
	rxs := []*discRx{r}
	if e.union != nil {
		rxs = append(rxs, e.union.twins...)
	}
	for _, d := range rxs {
		on := d.onBurst
		if on == nil {
			on = r.sink.put
		}
		d.onBurst = func(b *Burst) {
			a.stamp(b)
			on(b)
		}
	}
}

func (r *discRx) fixLag() float64 {
	if e, _ := r.ext.(*softExt); e != nil && e.ac != nil {
		return e.ac.lag
	}
	return 0
}

func (r *discRx) write(x []int16) {
	if e, _ := r.ext.(*softExt); e != nil && e.ac != nil {
		e.ac.write(r, x)
		return
	}
	r.writeCore(x)
}

func (r *discRx) writeCore(x []int16) {
	e, _ := r.ext.(*softExt)
	if e != nil && e.union != nil {
		e.union.write(x)
		return
	}
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

func (r *discRx) inputHz() int {
	if e, _ := r.ext.(*softExt); e != nil && e.ph != nil {
		return e.ph.inputHz()
	}
	return r.hz()
}

func (r *leanCore) hasExt() bool { return r.ext != nil }

func (r *leanCore) isReplaying() bool { return r.replaying }

func (r *leanCore) isReplayDry() bool { return r.replayDry }
