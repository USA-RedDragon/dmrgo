package rf

import "math"

const (
	discPhaseChunk = 256
	discPhaseRaw   = cohHist
	discPhaseDev   = 0.01
	discPhaseDev0  = 0.6
	discPhaseTaps  = iqTaps / 2
	discPhaseLag   = discPhaseTaps / 2
	discPhaseRing  = 16
	discEasySNR    = 20
	discAFCBlock   = iqAFCBlock / 2
)

const (
	discFitWide    = 0.16
	discFitNear    = 0.04
	discFitStep    = 0.02
	discFitGain    = 0.2
	discFitFar     = 3
	discFitFarStep = 0.05
	discSeedRho    = 0.4
	discFitRho     = 0.3
)

const (
	discClickHz     = 2.5 * 3 * deviationHz
	discClickSyms   = 1
	discClickWeight = 0.1
	discHeldFits    = 8
)

type discPhase struct {
	c        *leanCore
	sum      []int64
	size     int64
	sumN     int64
	last     int64
	raw      [discPhaseRing]complex64
	rawN     int64
	taps     [discPhaseTaps]float32
	hist     []complex64
	n        int64
	lo       int64
	num, den int64
	wait     int64
	a, b     float64
	o, sumO  int64
	unit     float64
	dcU      float64
	seen     bool
	cal      bool
	acc      complex64
	accN     int
	avg      complex64
	fitUnit  float64
	fitN     int
	coh      *cohStage
	clipAt   [discPhaseRaw / discClipBlock]int64
	clicks   []uint64
	held     float64
	heldN    int
	useHeld  bool
	out      [discPhaseChunk + discPhaseLag + 4]int16
}

func newDiscPhase(r *discRx, hz int, size int64) *discPhase {
	p := &discPhase{c: &r.leanCore, sum: make([]int64, size), size: size, clicks: make([]uint64, (size+63)/64)}
	for i := range p.clipAt {
		p.clipAt[i] = -1
	}
	p.hist = make([]complex64, size)
	fc := discCutHz / discRate
	var sum float64
	for i := range p.taps {
		k := float64(i - discPhaseLag)
		v := 2 * fc
		if k != 0 {
			v = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		v *= 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(discPhaseTaps-1))
		p.taps[i] = float32(v)
		sum += v
	}
	for i := range p.taps {
		p.taps[i] /= float32(sum)
	}
	switch hz {
	case discRate:
		p.num, p.den = 1, 1
	case discRate / 2:
		p.num, p.den, p.wait = 2, 1, 1
	default:
		p.num, p.den, p.wait = 5, 2, 1
	}
	return p
}

func (p *discPhase) inputHz() int {
	return int(discRate * p.den / p.num)
}

func (p *discPhase) input(x []int16) (int, []int16) {
	n := min(len(x), int(discPhaseChunk*p.den/p.num))
	p.update()
	lo, hi := p.clickBounds()
	k := 0
	scale := float32(float64(p.num) / (float64(p.den) * p.a))
	off := float32(p.b * float64(p.den) / float64(p.num))
	for _, u := range x[:n] {
		p.clip(u)
		p.click(u, lo, hi)
		i := p.sumN
		s := p.last + int64(u)
		p.sum[i&(p.size-1)] = s
		p.sumN++
		p.last = s
		for p.rawN*p.den <= (i-p.wait)*p.num {
			p.emit(p.rawN)
			p.rawN++
			if p.rawN < discPhaseTaps {
				continue
			}
			m := p.rawN - 1 - discPhaseLag
			p.filter(m)
			p.n = m + 1
			y, z := p.hist[m&(p.size-1)], p.hist[(m-1)&(p.size-1)]
			d := y * complex(real(z), -imag(z))
			v := p.rawAt(m)
			if p.cal && !p.clipped(float64(m-discPhaseLag), float64(m+discPhaseLag)) {
				v = int16(max(min(roundf((atan2f(imag(d), real(d))+off)*scale), math.MaxInt16), math.MinInt16))
			}
			p.acc += d
			if p.accN++; p.accN == discAFCBlock {
				p.follow()
				off = float32(p.b * float64(p.den) / float64(p.num))
			}
			for ; m == discPhaseLag && k < discPhaseLag; k++ {
				p.out[k] = v
			}
			p.out[k] = v
			k++
		}
	}
	return n, p.out[:k]
}

const discClipBlock = 32

func (p *discPhase) clip(u int16) {
	if u == math.MaxInt16 || u == math.MinInt16 {
		b := p.sumN / discClipBlock
		p.clipAt[b%int64(len(p.clipAt))] = b
	}
}

func (p *discPhase) clickBounds() (int32, int32) {
	unit := p.held
	if unit <= 0 {
		unit = p.unit
	}
	if unit <= 0 {
		return math.MinInt32, math.MaxInt32
	}
	d := discClickHz / unit
	return int32(max(math.Floor(p.dcU-d), math.MinInt32)), int32(min(math.Ceil(p.dcU+d), math.MaxInt32))
}

func (p *discPhase) click(u int16, lo, hi int32) {
	i := p.sumN & (p.size - 1)
	w, b := i/64, uint64(1)<<(i%64)
	if v := int32(u); v < lo || v > hi {
		p.clicks[w] |= b
	} else {
		p.clicks[w] &^= b
	}
}

func (p *discPhase) clickSpan() int64 {
	return (discClickSyms*discRate*p.den + symbolRate*p.num - 1) / (symbolRate * p.num)
}

func (p *discPhase) clicked(t0, t1 float64) bool {
	span := p.clickSpan()
	i0 := int64(math.Floor(max(t0, 0)*float64(p.den)/float64(p.num))) - span
	i1 := int64(math.Ceil(max(t1, 0)*float64(p.den)/float64(p.num))) + span
	i0, i1 = max(i0, p.sumN-p.size, 0), min(i1, p.sumN-1)
	for i := i0; i <= i1; i++ {
		k := i & (p.size - 1)
		if p.clicks[k/64]&(1<<(k%64)) != 0 {
			return true
		}
	}
	return false
}

func (p *discPhase) clipped(t0, t1 float64) bool {
	b0 := int64(max(t0, 0)*float64(p.den)/float64(p.num)) / discClipBlock
	b1 := int64(max(t1, 0)*float64(p.den)/float64(p.num)) / discClipBlock
	for b := b0; b <= b1; b++ {
		if p.clipAt[b%int64(len(p.clipAt))] == b {
			return true
		}
	}
	return false
}

func (p *discPhase) rawAt(m int64) int16 {
	r := float64(p.den) / float64(p.num)
	at := func(x float64) float64 {
		i := int64(math.Floor(x))
		mu := x - float64(i)
		s0 := float64(p.sum[i&(p.size-1)] - p.sumO)
		s1 := float64(p.sum[(i+1)&(p.size-1)] - p.sumO)
		return s0 + mu*(s1-s0)
	}
	x := float64(m) * r
	if x-r < 0 {
		return 0
	}
	v := (at(x) - at(x-r)) / r
	return int16(max(min(math.Round(v), math.MaxInt16), math.MinInt16))
}

func (p *discPhase) emit(m int64) {
	sn, cs := sincosf(wrapPi(p.phase(m)))
	p.raw[m&(discPhaseRing-1)] = complex(cs, sn)
}

func (p *discPhase) filter(m int64) {
	var re, im float32
	for k, t := range &p.taps {
		v := p.raw[(m-discPhaseLag+int64(k))&(discPhaseRing-1)]
		re += t * real(v)
		im += t * imag(v)
	}
	p.hist[m&(p.size-1)] = complex(re, im)
}

func (p *discPhase) phase(m int64) float64 {
	q := m * p.den
	i, r := q/p.num, q%p.num
	if r == 0 {
		return p.a*float64(p.sum[i&(p.size-1)]-p.sumO) - p.b*float64(i-p.o)
	}
	return p.between(i, float64(r)/float64(p.num))
}

func (p *discPhase) between(i int64, mu float64) float64 {
	at := func(k int64) float64 { return float64(p.sum[k&(p.size-1)] - p.sumO) }
	s0, s1, s2, s3 := at(i-1), at(i), at(i+1), at(i+2)
	s := s1 + 0.5*mu*(s2-s0+mu*(2*s0-5*s1+4*s2-s3+mu*(3*(s1-s2)+s3-s0)))
	return p.a*s - p.b*(float64(i-p.o)+mu)
}

func (p *discPhase) phaseAt(t float64) (float64, bool) {
	x := t * float64(p.den) / float64(p.num)
	i := int64(math.Floor(x))
	if i < 1 || i-1 < p.sumN-p.size || i+2 >= p.sumN {
		return 0, false
	}
	return p.between(i, x-float64(i)), true
}

func (p *discPhase) trialAt(t, t0, e, hz float64) complex64 {
	a, ok := p.phaseAt(t)
	b, ok0 := p.phaseAt(t0)
	if !ok || !ok0 {
		return 0
	}
	c := 2 * math.Pi * hz * (t - t0) / discRate
	sn, cs := sincosf(wrapPi((1+e)*(a-b-c) + c))
	return complex(cs, sn)
}

func wrapPi(x float64) float32 {
	return float32(x - 2*math.Pi*math.Round(x/(2*math.Pi)))
}

func (p *discPhase) update() {
	c := p.c
	f := c.audio
	if f.gain <= 0 {
		return
	}
	g := float64(f.gain)
	unit := p.unit
	switch {
	case c.locked && p.useHeld && p.heldN >= discHeldFits:
		unit = p.held
	case c.locked && p.fitN > 0:
		unit = p.fitUnit
	case p.cal:
	case c.locked:
		unit = g / float64(max(c.devScale, 0.1))
	case !p.seen:
		unit = g / discPhaseDev0
	}
	if p.unit > 0 && math.Abs(unit/p.unit-1) < discPhaseDev {
		return
	}
	p.build(unit, p.dcU)
}

func (p *discPhase) follow() {
	acc := p.acc
	p.acc, p.accN = 0, 0
	if p.avg == 0 {
		p.avg = acc
	} else {
		p.avg += complex(iqAFCAvgGain, 0) * (acc - p.avg)
	}
	est, avg := ringHz(acc), ringHz(p.avg)
	c := p.c
	switch {
	case c.locked:
		hz := p.coreHz()
		switch {
		case abs32(avg) >= iqAFCPullHz:
			p.move(avg)
			p.avg = 0
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < iqAFCPullHz:
			p.move(hz)
		}
	case abs32(est) >= iqAFCPullHz:
		p.avg = 0
		p.move(est)
	case abs32(avg) >= iqAFCFineHz:
		p.move(avg)
	}
}

func ringHz(acc complex64) float32 {
	return atan2f(imag(acc), real(acc)) * (discRate / (2 * math.Pi))
}

func (p *discPhase) move(hz float32) {
	p.build(p.unit, p.dcU+float64(hz)/p.unit)
}

func (p *discPhase) lock() {
	p.fitN, p.seen, p.cal = 0, true, false
	c := p.c
	g := float64(c.audio.gain)
	unit := p.unit
	if !p.cal {
		unit = g / float64(max(c.devScale, 0.1))
	}
	p.held, p.heldN = unit, 0
	p.build(unit, float64(c.audio.dc+c.dcHz)/g)
	p.acc, p.accN, p.avg = 0, 0, 0
}

func (p *discPhase) build(unit, dc float64) {
	if p.unit > 0 {
		if q := p.coh; q != nil {
			q.freq = (q.freq/p.unit + p.dcU - dc) * unit
			for i := range q.radios {
				r := &q.radios[i]
				r.freq = (r.freq/p.unit + p.dcU - dc) * unit
				r.totalHz = (r.totalHz/p.unit + p.dcU - dc) * unit
			}
		}
		k := unit / p.unit
		d := -2 * math.Pi * (dc - p.dcU) * unit / discRate
		p.acc, p.avg = rescaleTurn(p.acc, k, d), rescaleTurn(p.avg, k, d)
	}
	p.unit, p.dcU = unit, dc
	p.a = 2 * math.Pi * unit / (float64(discRate) * float64(p.den) / float64(p.num))
	p.b = p.a * dc
	p.o = max(p.sumN-p.size, 0)
	p.sumO = 0
	if p.sumN > 0 {
		p.sumO = p.sum[p.o&(p.size-1)]
	}
	lo := max(((p.o+p.wait)*p.num+p.den-1)/p.den, p.rawN-p.size)
	p.lo = lo + discPhaseLag
	for m := lo; m < p.rawN; m++ {
		p.emit(m)
		if k := m - discPhaseLag; k >= p.lo {
			p.filter(k)
		}
	}
}

func rescaleTurn(v complex64, k, d float64) complex64 {
	if v == 0 {
		return 0
	}
	ph := k*float64(atan2f(imag(v), real(v))) + d
	m := math.Hypot(float64(real(v)), float64(imag(v)))
	return complex(float32(m*math.Cos(ph)), float32(m*math.Sin(ph)))
}

func (p *discPhase) offsetHz() float64 {
	if c := p.c; c.locked && !c.bs {
		return float64(p.coreHz())
	}
	return float64(ringHz(p.avg))
}

func (p *discPhase) coreHz() float32 {
	c := p.c
	return float32((float64(c.audio.dc+c.dcHz)/float64(c.audio.gain) - p.dcU) * p.unit)
}

func (s *cohStage) fitDeviation(k0, k1 int, fit float64) {
	if math.Abs(fit) >= cohFreqFirst {
		return
	}
	p := s.ph
	if p == nil {
		s.fitIndex(k0, k1, fit)
		return
	}
	span, step := discFitNear, discFitStep
	switch {
	case !p.cal:
		span, step = math.Log(discFitFar), discFitFarStep
	case p.fitN == 0:
		span = discFitWide
	}
	s.totalHz += fit
	e, rho := s.scaleFit(k0, k1, span, step)
	s.totalHz = s.freq
	if rho < discFitRho {
		return
	}
	unit := p.unit * math.Exp(e)
	if p.fitN == 0 {
		p.fitUnit = unit
	} else {
		p.fitUnit += discFitGain * (unit - p.fitUnit)
	}
	if p.heldN < discHeldFits {
		p.heldN++
		p.held += (unit - p.held) / float64(p.heldN)
	}
	p.fitN++
	p.cal = true
	u := p.fitUnit
	if p.useHeld && p.heldN >= discHeldFits {
		u = p.held
	}
	if math.Abs(u/p.unit-1) >= discPhaseDev {
		p.build(u, p.dcU)
		s.totalHz = s.freq
	}
}

func (s *cohStage) seedDeviation(c *leanCore, k0, k1 int) bool {
	p := s.ph
	if p == nil || p.cal {
		return false
	}
	if rho2, _, _ := c.syncAt(c.ns - 1); rho2 < leanResample*leanResample {
		return false
	}
	e, rho := s.scaleFit(k0, k1, math.Log(discFitFar), discFitFarStep)
	if rho < discSeedRho || math.Abs(e) < discPhaseDev {
		return false
	}
	p.build(p.unit*math.Exp(e), p.dcU)
	return true
}

const cohFitScales = 48

func (s *cohStage) scaleFit(k0, k1 int, span, step float64) (float64, float64) {
	n := min(int(math.Round(2*span/step))+1, cohFitScales)
	amp, noise := s.amp[:n], s.noise[:n]
	best := 0
	s.trialing = s.ph != nil
	for i := range amp {
		if s.ph == nil {
			s.model.setOuterH(s.hFit * math.Exp(-span+float64(i)*step))
		} else {
			s.trial = math.Exp(-span+float64(i)*step) - 1
		}
		amp[i], noise[i] = s.levels(k0, k1)
		if amp[i] > amp[best] {
			best = i
		}
	}
	s.trialing, s.trial = false, 0
	a2 := amp[best] * amp[best]
	if amp[best] <= 0 {
		return 0, 0
	}
	e := -span + float64(best)*step
	if best > 0 && best+1 < len(amp) {
		a, b, c := amp[best-1], amp[best], amp[best+1]
		if d := a - 2*b + c; d < 0 {
			e += 0.5 * (a - c) / d * step
		}
	}
	return e, a2 / (a2 + max(noise[best], 0))
}

func (s *cohStage) fitIndex(k0, k1 int, fit float64) {
	span := discFitNear
	if s.hNew {
		span = discFitWide
	}
	h := s.model.h
	s.totalHz += fit
	e, rho := s.scaleFit(k0, k1, span, discFitStep)
	s.totalHz = s.freq
	if rho >= discFitRho && rho < 1 {
		v := cohIndexVar0 + cohIndexVar*(1-rho)/rho
		g := s.hVar / (s.hVar + v)
		s.hVar *= 1 - g
		s.hFit *= math.Exp(g * e)
		s.hNew = false
		f := s.hFit / cohSyncIndex
		if math.Abs(f/cohIndex-1) < cohIndexNear {
			f = cohIndex
		}
		if math.Abs(f/h-1) >= cohIndexMove {
			s.model.setH(f)
			return
		}
	}
	s.model.setOuterH(h)
}
