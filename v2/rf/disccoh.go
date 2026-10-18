package rf

import "math"

// Layout of discPhase.
const (
	// discPhaseChunk is the most ring samples by which discPhase runs
	// ahead of the core.
	discPhaseChunk = 256
	// discPhaseRaw is the length of the ring of input sums, a power of 2
	// that holds the input of cohHist ring samples at every rate.
	discPhaseRaw = cohHist
	// discPhaseDev is the change of the deviation scale, as a share, from
	// which discPhase builds its ring again.
	discPhaseDev = 0.01
	// discPhaseDev0 is the deviation scale before the first stream: near
	// the FM threshold the noise sets the input RMS, which a DiscReceiver
	// scales to discInputRMSHz, and the fits of the sweep measure 0.60
	// from 0 to 5 dB C/N.
	discPhaseDev0 = 0.6
	// discPhaseTaps is the length of the channel filter of the phasors, to
	// discCutHz at discRate, as iqDisc filters the I/Q, and discPhaseLag
	// its delay.
	discPhaseTaps = iqTaps / 2
	discPhaseLag  = discPhaseTaps / 2
	// discPhaseRing is the length of the ring of phasors that the channel
	// filter reads, a power of 2.
	discPhaseRing = 16
	// discEasySNR is the SNR of the levels of cohStage from which a burst
	// on an audio ring is easy. The ring holds unit phasors, whose noise is
	// all in their phase, so its levels measure twice the SNR that those of
	// the I/Q measure at the same C/N, and the 90% points of the audio lie
	// higher.
	discEasySNR = 20
	// discAFCBlock is the number of ring samples of a block of follow, as
	// iqAFCBlock is of iqRx.
	discAFCBlock = iqAFCBlock / 2
)

// Settings of cohStage.fitDeviation, in the log of the scale: the span of
// the first fit of a stream around the scale of the last, the span of the
// later fits, the step between the scales, and the weight of each later
// fit; and the factor that the fits try either way until one sets the
// scale, with its step.
const (
	discFitWide    = 0.16
	discFitNear    = 0.04
	discFitStep    = 0.02
	discFitGain    = 0.2
	discFitFar     = 3
	discFitFarStep = 0.05
	// discSeedRho is the least share of the power of the samples that the
	// waveform of the SYNC that the core tests must explain for
	// seedDeviation.
	discSeedRho = 0.4
	// discFitRho is the least share of the power of the samples that the
	// waveform of the known symbols must explain at the best scale for a
	// fit to count.
	discFitRho = 0.3
)

// discPhase rebuilds the carrier phase of the signal from the samples of a
// DiscReceiver, as the ring of cohStage at discRate. The phase is the sum
// of the discriminator frequencies: the noise of a discriminator rises
// with the square of the frequency, and the sum makes it white, as the
// noise of the phase of the I/Q is, and a click, which adds 2π to the
// phase, drops out of its unit phasor. Its scale is the deviation that
// cohStage fits to the SYNC of the bursts, and its DC offset that of the
// core.
type discPhase struct {
	c *leanCore
	// sum holds the sums of the input samples, input sample i at
	// i%discPhaseRaw, sumN of them so far, and last the last.
	sum  []int64
	sumN int64
	last int64
	// raw holds the last unit phasors of the phase at discRate, rawN of
	// them so far, and hist them after the channel filter of taps, n of
	// them; those from lo on have the scale of the last build. Ring sample
	// m is at input sample m*den/num.
	raw      [discPhaseRing]complex64
	rawN     int64
	taps     [discPhaseTaps]float32
	hist     []complex64
	n        int64
	lo       int64
	num, den int64
	// wait is the number of input samples after the last that a ring
	// sample between input samples waits for: 1, or 0 at discRate.
	wait int64
	// The phase at input sample x is a*(sum(x)-sumO) - b*(x-o), with
	// sum linear between the input samples, for the scale unit, in Hz per
	// input unit, less the DC offset dcU, in input units. seen marks a
	// phase that a stream has set them.
	a, b    float64
	o, sumO int64
	unit    float64
	dcU     float64
	seen    bool
	// cal marks a phase whose scale a fit set: the core then runs on the
	// discriminator of the ring.
	cal bool
	// acc sums the products of the ring samples with the conjugate of the
	// ones before over accN samples of the block of follow, and avg is
	// its mean over the blocks.
	acc  complex64
	accN int
	avg  complex64
	// fitUnit is the scale that the fits of coh measured over the stream,
	// fitN of them.
	fitUnit float64
	fitN    int
	coh     *cohStage
	// clipAt holds, for each block of discClipBlock input samples that the
	// sums hold, its number when an input sample in it lies at a limit of
	// int16.
	clipAt [discPhaseRaw / discClipBlock]int64
	// lazy marks a phase that keeps only the sums: the core runs on the
	// input, and the coherent stage builds each phasor from the sums as it
	// reads it.
	lazy bool
	// out holds the discriminator samples of the ring for the core, in
	// the units of the input.
	out [discPhaseChunk + discPhaseLag + 4]int16
}

// lazyDT returns the offset of the symbol timing of the core of a
// DiscReceiver at hz that a lazy phase starts from, in samples at
// discRate: the delays of the front end of the core differ by its input
// rate, and the fits of the bursts measure these.
func lazyDT(hz int) float64 {
	switch hz {
	case discRate:
		return 0.7
	case discRate / 2:
		return -0.8
	}
	return -0.9
}

// newDiscPhase returns the phase of the samples at hz for DiscReceiver r,
// whose core runs at discRate on the discriminator of the ring, or on its
// input when lazy.
func newDiscPhase(r *DiscReceiver, hz int, lazy bool) *discPhase {
	p := &discPhase{c: &r.leanCore, sum: make([]int64, discPhaseRaw), lazy: lazy}
	for i := range p.clipAt {
		p.clipAt[i] = -1
	}
	if !lazy {
		p.hist = make([]complex64, cohHist)
	}
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

// inputHz returns the rate of the input of the receiver whose core runs
// on the ring.
func (p *discPhase) inputHz() int {
	return int(discRate * p.den / p.num)
}

// input takes the first samples of x and returns their number, and the
// discriminator samples of the ring that they complete, for the core: once
// a fit set the scale of the phase, the frequency of each ring sample after
// the channel filter, from the phase of its product with the conjugate of
// the one before, and before that the input samples.
func (p *discPhase) input(x []int16) (int, []int16) {
	n := min(len(x), int(discPhaseChunk*p.den/p.num))
	p.update()
	if p.lazy {
		for _, u := range x[:n] {
			p.clip(u)
			p.last += int64(u)
			p.sum[p.sumN&(discPhaseRaw-1)] = p.last
			p.sumN++
		}
		// The ring samples that the sums hold with the two after them of
		// the interpolation.
		p.n = max((p.sumN-3)*p.num/p.den, 0)
		p.lo = (max(p.sumN-discPhaseRaw, p.o)+2)*p.num/p.den + 1
		return n, x[:n]
	}
	k := 0
	// A ring sample moves the phase by a times the input sum over den/num
	// input samples, less b for each input sample.
	scale := float32(float64(p.num) / (float64(p.den) * p.a))
	off := float32(p.b * float64(p.den) / float64(p.num))
	for _, u := range x[:n] {
		p.clip(u)
		i := p.sumN
		s := p.last + int64(u)
		p.sum[i&(discPhaseRaw-1)] = s
		p.sumN++
		p.last = s
		// Ring samples between input samples wait for the one after.
		for p.rawN*p.den <= (i-p.wait)*p.num {
			p.emit(p.rawN)
			p.rawN++
			if p.rawN < discPhaseTaps {
				continue
			}
			m := p.rawN - 1 - discPhaseLag
			p.filter(m)
			p.n = m + 1
			y, z := p.hist[m&(cohHist-1)], p.hist[(m-1)&(cohHist-1)]
			d := y * complex(real(z), -imag(z))
			v := p.rawAt(m)
			// Where the input clipped, its phase is lost, and the core
			// takes the input.
			if p.cal && !p.clipped(float64(m-discPhaseLag), float64(m+discPhaseLag)) {
				v = int16(max(min(roundf((atan2f(imag(d), real(d))+off)*scale), math.MaxInt16), math.MinInt16))
			}
			p.acc += d
			if p.accN++; p.accN == discAFCBlock {
				p.follow()
				off = float32(p.b * float64(p.den) / float64(p.num))
			}
			// Core sample m is ring sample m: the first filtered sample
			// stands in for those before it.
			for ; m == discPhaseLag && k < discPhaseLag; k++ {
				p.out[k] = v
			}
			p.out[k] = v
			k++
		}
	}
	return n, p.out[:k]
}

// discClipBlock is the number of input samples of a block of clipAt.
const discClipBlock = 32

// clip notes input sample u, the next of the sums, when it lies at a limit
// of int16.
func (p *discPhase) clip(u int16) {
	if u == math.MaxInt16 || u == math.MinInt16 {
		b := p.sumN / discClipBlock
		p.clipAt[b%int64(len(p.clipAt))] = b
	}
}

// clipped reports whether an input sample from ring time t0 to t1 lies at
// a limit of int16: the input clipped there, and its phase is lost.
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

// rawAt returns the input at ring sample m, in input units: the mean of
// the input over the ring sample, from the sums linear between the input
// samples. The input of an input sample over a ring sample would hold each
// input sample over the ring samples that it spans, whose steps the
// matched filter of the core at discRate takes for the signal.
func (p *discPhase) rawAt(m int64) int16 {
	r := float64(p.den) / float64(p.num)
	at := func(x float64) float64 {
		i := int64(math.Floor(x))
		mu := x - float64(i)
		s0 := float64(p.sum[i&(discPhaseRaw-1)] - p.sumO)
		s1 := float64(p.sum[(i+1)&(discPhaseRaw-1)] - p.sumO)
		return s0 + mu*(s1-s0)
	}
	x := float64(m) * r
	if x-r < 0 {
		return 0
	}
	v := (at(x) - at(x-r)) / r
	return int16(max(min(math.Round(v), math.MaxInt16), math.MinInt16))
}

// emit sets phasor m from the sums.
func (p *discPhase) emit(m int64) {
	sn, cs := sincosf(wrapPi(p.phase(m)))
	p.raw[m&(discPhaseRing-1)] = complex(cs, sn)
}

// filter sets ring sample m from the phasors around it.
func (p *discPhase) filter(m int64) {
	var re, im float32
	for k, t := range &p.taps {
		v := p.raw[(m-discPhaseLag+int64(k))&(discPhaseRing-1)]
		re += t * real(v)
		im += t * imag(v)
	}
	p.hist[m&(cohHist-1)] = complex(re, im)
}

// phase returns the phase of ring sample m, which the sums must hold.
func (p *discPhase) phase(m int64) float64 {
	q := m * p.den
	i, r := q/p.num, q%p.num
	if r == 0 {
		return p.a*float64(p.sum[i&(discPhaseRaw-1)]-p.sumO) - p.b*float64(i-p.o)
	}
	return p.between(i, float64(r)/float64(p.num))
}

// between returns the phase at mu of the way from input sample i to i+1,
// by Catmull-Rom through the sums of samples i-1 to i+2: the sums are
// those of a band-limited frequency, smooth over the samples, and their
// line between i and i+1 errs twice as much.
func (p *discPhase) between(i int64, mu float64) float64 {
	at := func(k int64) float64 { return float64(p.sum[k&(discPhaseRaw-1)] - p.sumO) }
	s0, s1, s2, s3 := at(i-1), at(i), at(i+1), at(i+2)
	s := s1 + 0.5*mu*(s2-s0+mu*(2*s0-5*s1+4*s2-s3+mu*(3*(s1-s2)+s3-s0)))
	return p.a*s - p.b*(float64(i-p.o)+mu)
}

// phaseAt returns the phase at ring time t, and whether the sums hold it.
func (p *discPhase) phaseAt(t float64) (float64, bool) {
	x := t * float64(p.den) / float64(p.num)
	i := int64(math.Floor(x))
	if i < 1 || i-1 < p.sumN-discPhaseRaw || i+2 >= p.sumN {
		return 0, false
	}
	return p.between(i, x-float64(i)), true
}

// trialAt returns the unit phasor at ring time t of the phase from time t0,
// less the carrier offset hz, scaled by 1+e, with the carrier offset, or 0
// where the sums do not hold it.
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

// wrapPi returns x less the multiple of 2π that puts it in [-π, π].
func wrapPi(x float64) float32 {
	return float32(x - 2*math.Pi*math.Round(x/(2*math.Pi)))
}

// update builds the ring again when the scale of the core moved from that
// of the last build. The scale is the fit of the stream, or, before the
// first fit, the deviation scale of the core while it is locked and a guess
// from the gain of its front end before the first stream. Once a fit set
// it, only fits move it: the discriminator of the ring that the core then
// runs on depends on it, so that the levels of a false lock would mislead
// both.
func (p *discPhase) update() {
	c := p.c
	f := c.audio
	if f.gain <= 0 {
		return
	}
	g := float64(f.gain)
	unit := p.unit
	switch {
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

// follow moves the DC offset that the ring removes to the carrier at the
// end of a block of discAFCBlock ring samples, as iqRx.follow moves its
// oscillator: by the phase of the sum of the products of the ring samples
// with the conjugate of the ones before, which the clicks do not pull
// toward 0 Hz of the discriminator as they pull its mean, and which the
// noise pulls only toward the offset that the ring removes. While the
// core searches, a block whose offset is at least iqAFCPullHz moves it,
// or else the mean over the blocks, by iqAFCAvgGain, when it is
// iqAFCFineHz off; while it is locked, the mean, when it is iqAFCLockHz
// off.
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
		// As iqRx.follow: the mean, which the symbols do not move, when it
		// is far off, or else the DC offset of the core, when it agrees.
		hz := p.coreHz()
		switch {
		case abs32(avg) >= iqAFCPullHz:
			// The channel filter pulled the mean of the blocks before
			// toward the offset that the ring removed: it starts again.
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

// ringHz returns the frequency of the phase of a sum of the products of
// ring samples with the conjugate of the ones before.
func ringHz(acc complex64) float32 {
	return atan2f(imag(acc), real(acc)) * (discRate / (2 * math.Pi))
}

// move moves the DC offset that the ring removes by hz Hz.
func (p *discPhase) move(hz float32) {
	p.build(p.unit, p.dcU+float64(hz)/p.unit)
}

// lock starts the fits of a stream that the core locked to, from its DC
// offset, and from its deviation scale until a fit set the scale.
func (p *discPhase) lock() {
	p.fitN, p.seen = 0, true
	c := p.c
	g := float64(c.audio.gain)
	unit := p.unit
	if !p.cal {
		unit = g / float64(max(c.devScale, 0.1))
	}
	p.build(unit, float64(c.audio.dc+c.dcHz)/g)
	// The products of the search are of the offset that it pulled in.
	p.acc, p.accN, p.avg = 0, 0, 0
}

// build sets the scale of the phase to unit Hz per input unit, less the DC
// offset dc in input units, and builds the ring from the sums that it
// holds. The carrier offset of coh moves to the new scale.
func (p *discPhase) build(unit, dc float64) {
	if p.unit > 0 {
		if q := p.coh; q != nil {
			q.freq = (q.freq/p.unit + p.dcU - dc) * unit
		}
		// The phases of the products of follow scale with the unit and
		// turn with the offset that the ring removes.
		k := unit / p.unit
		d := -2 * math.Pi * (dc - p.dcU) * unit / discRate
		p.acc, p.avg = rescaleTurn(p.acc, k, d), rescaleTurn(p.avg, k, d)
	}
	p.unit, p.dcU = unit, dc
	p.a = 2 * math.Pi * unit / (float64(discRate) * float64(p.den) / float64(p.num))
	p.b = p.a * dc
	p.o = max(p.sumN-discPhaseRaw, 0)
	p.sumO = 0
	if p.sumN > 0 {
		p.sumO = p.sum[p.o&(discPhaseRaw-1)]
	}
	if p.lazy {
		return
	}
	lo := max(((p.o+p.wait)*p.num+p.den-1)/p.den, p.rawN-cohHist)
	p.lo = lo + discPhaseLag
	for m := lo; m < p.rawN; m++ {
		p.emit(m)
		if k := m - discPhaseLag; k >= p.lo {
			p.filter(k)
		}
	}
}

// rescaleTurn returns v with its phase times k plus d.
func rescaleTurn(v complex64, k, d float64) complex64 {
	if v == 0 {
		return 0
	}
	ph := k*float64(atan2f(imag(v), real(v))) + d
	m := math.Hypot(float64(real(v)), float64(imag(v)))
	return complex(float32(m*math.Cos(ph)), float32(m*math.Sin(ph)))
}

// offsetHz returns the carrier offset left in the ring, in Hz: the mean
// of follow, or on a direct mode stream, whose noise pulls the mean, the
// DC offset of the core.
func (p *discPhase) offsetHz() float64 {
	if c := p.c; p.lazy || c.locked && !c.bs {
		return float64(p.coreHz())
	}
	return float64(ringHz(p.avg))
}

// coreHz returns the carrier offset left in the ring that the DC offset of
// the core gives, in Hz.
func (p *discPhase) coreHz() float32 {
	c := p.c
	return float32((float64(c.audio.dc+c.dcHz)/float64(c.audio.gain) - p.dcU) * p.unit)
}

// fitDeviation fits the deviation scale of the audio ring to the known
// symbols k0 to k1 of the burst at t0, whose SYNC fits the carrier offset
// fit from that of the stream, by scaleFit. Until a fit sets the scale,
// the fits try a factor of discFitFar either way, since the guess of the
// search and the levels of the core can be far off; the first fit of each
// stream after that tries discFitWide around the scale of the last, for a
// transmitter with another deviation; the later ones try discFitNear and
// move it by discFitGain. It builds the ring again when the fit moves it.
// On I/Q it fits the modulation index of the model by fitIndex instead. It
// does nothing for a SYNC whose carrier is far off.
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
	p.fitN++
	p.cal = true
	if math.Abs(p.fitUnit/p.unit-1) >= discPhaseDev {
		p.build(p.fitUnit, p.dcU)
		s.totalHz = s.freq
	}
}

// seedDeviation sets the scale of an audio ring that no fit set yet from
// the SYNC that core c tests, the known symbols k0 to k1 at t0, when a
// scale within a factor of discFitFar of the guess makes its waveform
// explain at least discSeedRho of the samples: the guess of the search
// from the input level can be far off, and then the coherent tests find
// no SYNC that the discriminator misses. It tries only a SYNC whose
// discriminator correlation is at least leanResample, which noise seldom
// reaches. It reports whether it built the ring again.
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

// scaleFit returns the log of the factor of the scale of the audio ring at
// which the waveform of the known symbols k0 to k1 at t0 explains the most
// of the samples, on a grid of step to span either way, by a parabola
// through the best, and the share of the power that it explains there. On
// I/Q it scales the modulation index hFit of the model instead, for the
// windows of a SYNC, and leaves them at the last index of the grid.
func (s *cohStage) scaleFit(k0, k1 int, span, step float64) (float64, float64) {
	// The far grid has the most scales, 2*ln(3)/0.05+1 of them.
	var ampBuf, noiseBuf [48]float64
	n := min(int(math.Round(2*span/step))+1, len(ampBuf))
	amp, noise := ampBuf[:n], noiseBuf[:n]
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

// fitIndex fits the modulation index of the model on I/Q to the known
// symbols k0 to k1 of the burst at t0, whose SYNC fits the carrier offset
// fit from that of the stream, as fitDeviation fits the scale of an audio
// ring: a transmitter whose deviation strays from the nominal by a few
// percent turns the phase of the model away from that of the signal over
// the runs, which the trellis takes for symbol errors. The first fit of
// each stream tries discFitWide around the fit of the last, the later
// ones discFitNear. The fits weigh by their variance against that of the
// index, as a Kalman filter, in the log of the index: cohIndexVar0 plus
// cohIndexVar over the SNR of the SYNC for a fit, which the sweep
// measured from 1 to 12 dB C/N. The model takes the fit over
// cohSyncIndex.
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
		if f := s.hFit / cohSyncIndex; math.Abs(f/h-1) >= cohIndexMove {
			s.model.setH(f)
			return
		}
	}
	s.model.setOuterH(h)
}
