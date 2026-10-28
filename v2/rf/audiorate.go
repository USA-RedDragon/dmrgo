//go:build !tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/resample"

type audioRate struct {
	rs   resample.Real
	rsOn bool
}

func (a *audioRate) resampling() bool { return a.rsOn }

func (a *audioRate) resampleTaps() int { return a.rs.Taps() }

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
	if !r.rs0.on() || t <= Tier0 {
		return
	}
	step := float64(r.rate) / float64(audioNative(int(r.rate)))
	d := audioDesignOf(t)
	rows := d.phases
	if d.lerp {
		rows++
	}
	h := make([]float32, rows*d.taps)
	resample.Design(h, d.taps, d.phases, audioUpCut*min(1, 1/step), audioUpBeta)
	r.rsOn = r.rs.Init(h, d.taps, d.phases, d.lerp, step, make([]int16, d.taps))
	r.rs0 = rate0{}
}

func (r *AudioReceiver) resampled(x []int16) {
	var buf [32]int16
	m := 0
	most := r.rs.MaxOut()
	for _, v := range x {
		m += r.rs.Push(v, buf[m:])
		if m+most > len(buf) {
			r.input(buf[:m])
			m = 0
		}
	}
	if m > 0 {
		r.input(buf[:m])
	}
}
