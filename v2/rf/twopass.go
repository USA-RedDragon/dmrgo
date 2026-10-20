package rf

import "math"

const twoPassSlots = 1024

type passRec struct {
	at                   float64
	amp, noise           float64
	anchor               bool
	hz                   float64
	hzN                  int
	freq, a1, n1         float64
	smHz, smAmp, smNoise float64
}

func (o *options) passSlots() int {
	switch {
	case !o.TwoPass:
		return 0
	case o.TwoPassSlots > 0:
		return o.TwoPassSlots
	}
	return twoPassSlots
}

func (r *engine) pass(j int64) *passRec {
	return &r.passRecs[r.slotPos(j)]
}

func (r *engine) emits() bool {
	return !r.warm && !r.scout
}

func (r *engine) passAnchor(s *slotState, tau float64) {
	if !r.scout {
		return
	}
	p := r.pass(s.index)
	p.at, p.amp, p.noise, p.anchor = s.start+tau, s.a.amp, s.a.noise, true
}

func (r *engine) passCarrier(lo, hi symRef, hz float64) {
	if !r.scout {
		return
	}
	j := lo.slot + (hi.slot-lo.slot)/2
	if r.slot(j).index != j {
		return
	}
	p := r.pass(j)
	p.hz += hz
	p.hzN++
}

func (r *engine) passKeep(j int64) {
	if !r.scout {
		return
	}
	p := r.pass(j)
	p.freq, p.a1, p.n1 = r.freq, r.amp, r.noise
}

func (r *engine) passAt(j int64) {
	if !r.frozen {
		return
	}
	p := r.pass(j)
	r.freq = p.smHz
	if p.smAmp > 0 {
		r.amp, r.noise = p.smAmp, p.smNoise
	}
}

func (r *engine) startPass(seg bool) {
	r.scout, r.frozen = false, true
	r.passEnd, r.passSeg = r.next, seg
	lo, hi := r.passStart, r.next
	var f [5]float64
	for j := lo; j < hi; j++ {
		if p := r.pass(j); p.anchor {
			k := float64(j)
			f[0]++
			f[1] += k
			f[2] += k * k
			f[3] += p.at
			f[4] += k * p.at
		}
	}
	s2, s4 := f[2]+timingPrior, f[4]+timingPrior*slotSamplesN
	if det := f[0]*s2 - f[1]*f[1]; f[0] > 1.5 && det > 1e-9 {
		if b := (f[0]*s4 - f[1]*f[3]) / det; math.Abs(b-slotSamplesN) < 2 && !r.opt.TwoPassKeepTiming {
			r.period = b
			r.t0 = (f[3]*s2 - f[1]*s4) / det
		}
	}
	for j := lo; j < hi; j++ {
		p := r.pass(j)
		p.smHz, p.smAmp, p.smNoise = r.carrierAt(j, lo, hi), p.a1, p.n1
		switch r.opt.TwoPassLevels {
		case 0:
			p.smAmp, p.smNoise = r.levelsAt(j, lo, hi)
		case 2:
			p.smAmp, p.smNoise = r.levelsAround(j, lo, hi)
		}
	}
	if r.hg != nil {
		r.gateH(lo, hi)
	}
	r.next, r.first = lo, lo
	r.lastAnch, r.lost, r.quiet, r.offEnergy = math.MinInt64, 0, 0, 0
	r.passAt(lo)
}

func (r *engine) window(j, lo, hi int64) (int64, int64) {
	if w := int64(r.opt.TwoPassSmooth); w > 0 {
		return max(lo, j-w), min(hi, j+w+1)
	}
	return lo, hi
}

func (r *engine) carrierAt(j, lo, hi int64) float64 {
	p := r.pass(j)
	if r.opt.TwoPassKeepCarrier {
		return p.freq
	}
	a, b := r.window(j, lo, hi)
	var f [5]float64
	var slots int
	for k := a; k < b; k++ {
		q := r.pass(k)
		if q.hzN == 0 {
			continue
		}
		n, x := float64(q.hzN), float64(k-j)
		f[0] += n
		f[1] += n * x
		f[2] += n * x * x
		f[3] += q.hz
		f[4] += x * q.hz
		slots++
	}
	if slots < 2 {
		return p.freq
	}
	if det := f[0]*f[2] - f[1]*f[1]; det > 1e-9 {
		return (f[3]*f[2] - f[1]*f[4]) / det
	}
	return f[3] / f[0]
}

func (r *engine) levelsAt(j, lo, hi int64) (float64, float64) {
	a, b := r.window(j, lo, hi)
	var amp, noise, n float64
	for k := a; k < b; k++ {
		if q := r.pass(k); q.anchor {
			amp, noise, n = amp+q.amp, noise+q.noise, n+1
		}
	}
	if n == 0 {
		p := r.pass(j)
		return p.a1, p.n1
	}
	return amp / n, noise / n
}

func (r *engine) levelsAround(j, lo, hi int64) (float64, float64) {
	a, b := j, j
	for a >= lo && !r.pass(a).anchor {
		a--
	}
	for b < hi && !r.pass(b).anchor {
		b++
	}
	switch {
	case a < lo && b >= hi:
		p := r.pass(j)
		return p.a1, p.n1
	case a < lo:
		a = b
	case b >= hi:
		b = a
	}
	pa, pb := r.pass(a), r.pass(b)
	u := 0.0
	if b > a {
		u = float64(j-a) / float64(b-a)
	}
	return pa.amp + u*(pb.amp-pa.amp), pa.noise + u*(pb.noise-pa.noise)
}

func (r *engine) endPass() {
	r.frozen = false
	if r.passSeg {
		r.scout, r.passStart = true, r.first
		return
	}
	r.finishStream()
	if r.pend {
		r.searchAt, r.doneAt, r.candAt, r.pend = r.pendAt, r.pendDone, -1, false
	}
}
