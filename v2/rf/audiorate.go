package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/fmath"
	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

type audioRate struct {
	rs resample.Real
}

func (r *AudioReceiver) floatRate() *audioRate {
	if x, ok := r.ext.(*softExt); ok && r.hasExt() {
		return x.rs
	}
	return nil
}

func (r *AudioReceiver) resampling() bool { return r.floatRate() != nil }

func (r *AudioReceiver) resampleTaps() int {
	if a := r.floatRate(); a != nil {
		return a.rs.Taps()
	}
	return 0
}

type audioDesign struct {
	taps, phases int
	lerp         bool
}

func audioDesignOf(t Tier) audioDesign {
	switch t {
	case Tier1:
		return audioDesign{32, 64, true}
	case Tier2, Tier3:
		return audioDesign{resampleTaps, resamplePhases, false}
	case TierDefault, Tier0:
	}
	return audioDesign{audioUpTaps, audioUpPhases, true}
}

func audioRateOK(rate int, t Tier) bool {
	switch {
	case rate < audioMinRate || rate > audioMaxRate:
		return false
	case t <= Tier0:
		return rate <= 2*discRate
	}
	return true
}

func (r *AudioReceiver) designResample(t Tier) {
	x, ok := r.ext.(*softExt)
	if !ok || !r.hasExt() || !r.rs0.on() || t <= Tier0 {
		return
	}
	step := float64(r.rate) / float64(audioNative(int(r.rate)))
	d := audioDesignOf(t)
	rows := d.phases
	if d.lerp {
		rows++
	}
	h := make([]float32, rows*d.taps)
	resample.Design(h, d.taps, d.phases, audioUpCut*fmath.Min(1, 1/step), audioUpBeta)
	a := &audioRate{}
	if !a.rs.Init(h, d.taps, d.phases, d.lerp, step, make([]int16, d.taps)) {
		return
	}
	x.rs = a
	r.rs0 = rate0{}
}

func (r *AudioReceiver) resampled(x []int16) {
	var buf [32]int16
	m := 0
	a := r.floatRate()
	if a == nil {
		r.input(x)
		return
	}
	most := a.rs.MaxOut()
	for _, v := range x {
		m += a.rs.Push(v, buf[m:])
		if m+most > len(buf) {
			r.input(buf[:m])
			m = 0
		}
	}
	if m > 0 {
		r.input(buf[:m])
	}
}
