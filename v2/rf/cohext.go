package rf

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Layout of cohStage.
const (
	// cohSPS is the number of channel filter samples per symbol of iqRx at
	// 24 kHz, the rate of the trellis.
	cohSPS = discRate / symbolRate
	// cohHist is the length of the ring of channel filter samples, a power
	// of 2 that holds a slot and the delay of the core.
	cohHist = 1 << 13
	// cohKnown is the number of known symbols from which a run starts: the
	// four of an EMB half, or the 24 of a SYNC.
	cohKnown = 4
)

// cohStage detects each burst again coherently from the channel filter
// samples of iqRx, at the symbol timing of the core: a trellis over a
// window of three symbols that carries the carrier phase with each
// survivor, as TierCoherent detects, run forward and backward from the
// known symbols of the burst, its SYNC or the EMB that the core decoded.
// It replaces the log-likelihood ratios of the core for the decoders of
// softExt.
type cohStage struct {
	rx    *iqRx
	model *cpmModel
	tr    *trellis
	mu    float32
	llrK  float32
	buf   []complex64
	known [slotSymbols]int8
	isK   [slotSymbols]bool
	app   [slotSymbols][4]float32
	dist  [slotSymbols]int
	// The run under way: its first symbol in the slot, its direction and
	// steps, the time of slot symbol 0 in samples of the ring and the
	// carrier rotation per sample.
	g0    int
	dir   int
	steps int
	t0    float64
	// dt is the offset of the core's symbol timing that the SYNC of the
	// bursts finds, in ring samples, followed from burst to burst.
	dt      float64
	totalHz float64
	// freq is the carrier offset of the stream, in Hz, that the SYNC of
	// the bursts fits, from the DC offset of the core at the first.
	freq   float64
	freqOK bool
	nFit   int
	// The burst under detection: the scale of the trellis metric, its
	// known runs, and the bit priors of its detection again.
	scale    float32
	segs     [2][2]int
	nseg     int
	prior    [slotSymbols][4]float32
	hasPrior bool
	// priorK gives the known symbols their prior: for a SYNC, not for an
	// EMB that the core decoded.
	priorK bool
	sps    float64
	base   int64
}

// newCohStage returns the coherent stage of iqRx r, with the settings
// opt.
func newCohStage(r *iqRx, opt *options) *cohStage {
	m := newCPMModel(1, cohSPS, 0.27)
	s := &cohStage{rx: r, model: m, mu: opt.PSPGain, llrK: opt.LLRScale, dt: cohDT}
	if s.llrK == 0 {
		s.llrK = 1
	}
	s.tr = newTrellis(m, slotSymbols+2*runMargin, slotSymbols+2*runMargin, 0)
	s.buf = make([]complex64, (syncSymbols+2)*cohSPS)
	return s
}

// symTime returns the time of symbol k of the core, in samples of the
// ring of channel filter samples. The DiscReceiver counts its input
// samples, one per ring sample, as two baseband samples.
func symTime(c *leanCore, k int64) float64 {
	t := float64(c.mfStep*(c.n-1)+c.center-(c.ns-1-k)*samplesPerSy) + float64(c.mfStep)*float64(c.lastAt)
	return t / 2
}

// sample returns the ring sample at time t, interpolated by Catmull-Rom,
// rotated by the carrier residual.
func (s *cohStage) sample(t float64) complex64 {
	r := s.rx
	i := int64(math.Floor(t))
	mu := float32(t - float64(i))
	at := func(k int64) complex64 {
		if k < 0 || k >= r.yN || r.yN-k > cohHist {
			return 0
		}
		return r.yHist[k&(cohHist-1)]
	}
	p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
	c := complex(mu, 0)
	v := p1 + 0.5*c*(p2-p0+c*(2*p0-5*p1+4*p2-p3+c*(3*(p1-p2)+p3-p0)))
	return v * complex64(cmplx.Exp(complex(0, s.phaseAt(t))))
}

// phaseAt returns the phase, in radians, that removes the carrier left in
// the ring sample at time t: the carrier offset less the frequency that
// the oscillator removed there, summed from the time of slot symbol 0.
// The oscillator moves at most a few times a second, so the sum runs over
// a few pieces.
func (s *cohStage) phaseAt(t float64) float64 {
	r := s.rx
	a, b := s.t0, t
	sign := 1.0
	if b < a {
		a, b, sign = b, a, -1
	}
	var ph float64
	for a < b {
		n := int64(math.Floor(a))
		hz := s.totalHz - float64(r.ncoAt(n))
		// The next move after n, if one comes before b.
		end := b
		for i := 0; i < min(r.nTunes, iqTunes); i++ {
			if tn := float64(r.tunes[(r.nTunes-1-i)%iqTunes].n); tn > a && tn < end {
				end = tn
			}
		}
		ph += hz * (end - a)
		a = end
	}
	return -2 * math.Pi * sign * ph / discRate
}

// fill writes the samples of slot symbols k0 to k1-1 of the run into x,
// as engine.fillRange: time reversed and conjugated for a backward run.
func (s *cohStage) fill(k0, k1 int, x []complex64) {
	sps := s.model.sps
	for i := k0; i < k1; i++ {
		k := s.g0 + s.dir*i
		tc := s.t0 + float64(k)*s.sps
		start := tc - float64(sps/2)
		step := 1.0
		if s.dir < 0 {
			start = tc + float64(sps/2) - 1
			step = -1
		}
		for j := range sps {
			v := s.sample(start + float64(j)*step)
			if s.dir < 0 {
				v = complex(real(v), -imag(v))
			}
			x[(i-k0)*sps+j] = v
		}
	}
}

func (s *cohStage) runSamples(k0, k1 int, x []complex64) {
	d := s.model.d
	s.fill(k0+d, k1+d, x)
}

func (s *cohStage) runPrior(k int, lp []float32) {
	clear(lp)
	j := s.g0 + s.dir*(k+2*s.model.d)
	if j < 0 || j >= slotSymbols {
		return
	}
	if s.isK[j] && s.priorK {
		lp[(s.known[j]+3)/2] = cohKnownPrior
	}
	if s.hasPrior {
		for d := range lp {
			lp[d] += s.prior[j][d]
		}
	}
}

func (s *cohStage) runKeep(k int, app []float32) {
	d := s.model.d
	if k >= s.steps-d {
		return
	}
	dist := k + 2*d
	j := s.g0 + s.dir*dist
	if j < cachSymbols || j >= slotSymbols || dist >= s.dist[j] {
		return
	}
	copy(s.app[j][:], app)
	s.dist[j] = dist
}

// Settings of the fine timing of cohStage: the offset of the symbol
// timing of the core that it starts from, the offsets it tries around the
// last, and the weight of each SYNC in it.
const (
	cohDT     = 1.0
	cohDTSpan = 0.6
	cohDTStep = 0.2
	cohDTGain = 0.2
)

// fineTime moves t0 and dt to the offset of the symbol timing, within
// cohDTSpan of dt, at which the known symbols k0 to k1 correlate best.
func (s *cohStage) fineTime(k0, k1 int) {
	t0, best, bestD := s.t0, -1.0, 0.0
	for d := -cohDTSpan; d <= cohDTSpan+1e-9; d += cohDTStep {
		s.t0 = t0 + d
		if a, _ := s.levels(k0, k1); a > best {
			best, bestD = a, d
		}
	}
	s.t0 = t0 + bestD
	s.dt += cohDTGain * bestD
}

// cohFreqGain is the weight of the carrier offset that each SYNC fits.
const cohFreqGain = 0.1

// freqFit returns the carrier offset, in Hz, left in the ring samples of
// the known symbols k0 to k1: the phase step from symbol to symbol of
// their correlations with their waveform, less the phase of the symbols.
func (s *cohStage) freqFit(k0, k1 int) float64 {
	m := s.model
	s.g0, s.dir = k0, 1
	n := min(k1-k0+1, syncSymbols)
	var syms [syncSymbols]int
	for i := range n {
		syms[i] = int(s.known[k0+i])
	}
	s.fill(0, n, s.buf)
	var prev, acc complex128
	ph := 0.0
	for k := range n - 2*m.d {
		w := 0
		for i := range 2*m.d + 1 {
			w = w<<2 | (syms[k+i]+3)/2
		}
		var c complex128
		for j := range m.sps {
			c += complex128(s.buf[(k+m.d)*m.sps+j]) * complex128(m.tpl[w*m.sps+j])
		}
		c *= cmplx.Exp(complex(0, -ph))
		if k > 0 {
			acc += c * cmplx.Conj(prev)
		}
		prev = c
		ph += m.h * math.Pi * float64(syms[k])
	}
	if acc == 0 {
		return 0
	}
	return cmplx.Phase(acc) * symbolRate / (2 * math.Pi)
}

// cohKnownPrior is the log prior, in nats, of a known symbol, and
// cohRefinePrior that of each bit that the FEC of a voice burst fixed, for
// its detection again.
const (
	cohKnownPrior  = 4
	cohRefinePrior = 10
)

// detect detects burst b of the core c, whose fields the core decoded,
// coherently, and writes the log-likelihood ratios of its 264 bits into
// llr. It reports false when the burst has too few known symbols, or the
// ring no longer holds it.
func (s *cohStage) detect(c *leanCore, b *Burst, llr *[layer2.BurstBits]int16) bool {
	r := s.rx
	f := &r.d.f
	if f.gain <= 0 {
		return false
	}
	s.base = c.base
	s.sps = float64(samplesPerSy) / 2
	s.t0 = symTime(c, c.base) + s.dt
	if s.t0-float64(cohSPS) < float64(r.yN-cohHist) {
		return false
	}
	if !s.freqOK {
		s.freq, s.freqOK = float64(r.conv.hz)+float64((f.dc+c.dcHz)/(f.gain*iqUnitsPerHz)), true
	}
	s.totalHz = s.freq
	// The known symbols: the SYNC, or the two halves of the EMB that the
	// core decoded.
	clear(s.isK[:])
	var segs [2][2]int
	var nseg int
	if _, _, ok := layer2.SyncSourceOf(b.Sync); ok && b.Distance >= 0 {
		var syms [syncSymbols]int8
		syncSyms(b.Sync, &syms)
		for i, v := range syms {
			s.known[syncFirstSym+i], s.isK[syncFirstSym+i] = v, true
		}
		segs[0] = [2]int{syncFirstSym, syncFirstSym + syncSymbols - 1}
		nseg, s.priorK = 1, true
	} else if b.Voice != enums.VoiceBurstUnknown && b.HasFixed {
		for _, k := range [...]int{syncFirstSym, syncFirstSym + 1, syncFirstSym + 2, syncFirstSym + 3,
			syncFirstSym + syncSymbols - 4, syncFirstSym + syncSymbols - 3, syncFirstSym + syncSymbols - 2, syncFirstSym + syncSymbols - 1} {
			p := 2 * (k - cachSymbols)
			b0 := b.Fixed[p/8] >> (7 - p%8) & 1
			b1 := b.Fixed[(p+1)/8] >> (7 - (p+1)%8) & 1
			s.known[k], s.isK[k] = symbolOf(unpacked.Bit(b0), unpacked.Bit(b1)), true
		}
		segs[0] = [2]int{syncFirstSym, syncFirstSym + 3}
		segs[1] = [2]int{syncFirstSym + syncSymbols - 4, syncFirstSym + syncSymbols - 1}
		// The EMB that the core decoded sets the phase of the runs, but
		// gives no priors, so that the decode of the EMB from the new
		// ratios does not repeat a wrong one.
		nseg, s.priorK = 2, false
	} else {
		return false
	}
	for i := range s.dist {
		s.dist[i] = math.MaxInt
	}
	if nseg == 1 {
		s.fineTime(segs[0][0], segs[0][1])
		// The mean of the first fits, then a slow average.
		s.nFit++
		s.freq += max(cohFreqGain, 1/float64(s.nFit+1)) * s.freqFit(segs[0][0], segs[0][1])
		s.totalHz = s.freq
	}
	var amp, noise float64
	for _, sg := range segs[:nseg] {
		a, v := s.levels(sg[0], sg[1])
		amp += a / float64(nseg)
		noise += v / float64(nseg)
	}
	if amp <= 0 || noise <= 0 {
		return false
	}
	s.scale = float32(2 * amp / noise)
	s.segs, s.nseg, s.hasPrior = segs, nseg, false
	return s.runAll(llr)
}

// cohPlaceRho is the least share of the power of the SYNC field that its
// waveform must explain for place to take it as a SYNC.
const cohPlaceRho = 0.4

// place tests the SYNC field of burst b of slot j, which the core could
// not place, coherently against the voice and the data SYNC of the
// stream, and places it as the burst of the one whose waveform explains
// most of its power, when that is at least cohPlaceRho of it.
func (s *cohStage) place(c *leanCore, b *Burst, j int32) bool {
	r := s.rx
	if r.d.f.gain <= 0 || !s.freqOK {
		return false
	}
	s.sps = float64(samplesPerSy) / 2
	t0 := symTime(c, c.base) + s.dt
	if t0-float64(cohSPS) < float64(r.yN-cohHist) {
		return false
	}
	s.totalHz = s.freq
	voice, data, _ := sourceAt(int(c.src)).SyncPatterns()
	best, bestP := 0.0, enums.SyncPattern(0)
	for _, p := range [...]enums.SyncPattern{voice, data} {
		var syms [syncSymbols]int8
		syncSyms(p, &syms)
		for i, v := range syms {
			s.known[syncFirstSym+i] = v
		}
		s.t0 = t0
		s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
		amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
		if rho := amp * amp / (amp*amp + max(noise, 0)); amp > 0 && rho > best {
			best, bestP = rho, p
		}
	}
	if best < cohPlaceRho {
		return false
	}
	ch := &c.ch[j&1]
	b.Sync, b.Distance = bestP, syncDist(b.Data, bestP)
	if bestP == voice {
		ch.sf = j
	} else {
		ch.sf = leanNoSlot
	}
	return true
}

// cohVerifyRho is the least share of the power of a SYNC field that its
// waveform must explain for verify.
const cohVerifyRho = 0.35

// verify reports whether the last symbols of the core c end SYNC p by
// the coherent test of place: the ring samples of the SYNC field against
// its waveform, at the carrier offset that the oscillator and the core
// give, or that the stream had.
func (s *cohStage) verify(c *leanCore, p enums.SyncPattern) bool {
	r := s.rx
	f := &r.d.f
	if f.gain <= 0 {
		return false
	}
	s.sps = float64(samplesPerSy) / 2
	base := c.ns - 1 - leanSyncEnd
	s.t0 = symTime(c, base) + s.dt
	if s.t0-float64(cohSPS) < float64(r.yN-cohHist) {
		return false
	}
	s.totalHz = s.freq
	if !s.freqOK {
		s.totalHz = float64(r.conv.hz) + float64(f.dc/(f.gain*iqUnitsPerHz))
	}
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	for i, v := range syms {
		s.known[syncFirstSym+i] = v
	}
	s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
	amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
	return amp > 0 && amp*amp/(amp*amp+max(noise, 0)) >= cohVerifyRho
}

// runAll runs the trellis forward from the start of the first known run
// of the burst to its end, and backward from the end of the last to its
// start, and writes the log-likelihood ratios into llr. It reports false
// when a symbol got none.
func (s *cohStage) runAll(llr *[layer2.BurstBits]int16) bool {
	for i := range s.dist {
		s.dist[i] = math.MaxInt
	}
	lo, hi := cachSymbols, slotSymbols-1
	g0, g1 := s.segs[0][0], s.segs[s.nseg-1][1]
	s.run(g0, hi-g0+1+runMargin, 1, s.scale)
	s.run(g1, g1-lo+1+runMargin, -1, s.scale)
	for k := lo; k <= hi; k++ {
		if s.dist[k] == math.MaxInt {
			return false
		}
		p := 2 * (k - cachSymbols)
		llr[p], llr[p+1] = dibitLLR(s.app[k][:], s.llrK)
	}
	return true
}

// refineData detects the data burst again with the bit priors w*ext, the
// extrinsic ratios of its grid decode weighed by w, and writes the new
// ratios into llr.
func (s *cohStage) refineData(ext *[layer2.BurstBits]float32, w float32, llr *[layer2.BurstBits]int16) bool {
	clear(s.prior[:])
	for k := range burstSymbols {
		lp := &s.prior[cachSymbols+k]
		for d := range 4 {
			b0, b1 := dibitOfDigit(d)
			lp[d] = bitPrior(w*ext[2*k], b0) + bitPrior(w*ext[2*k+1], b1)
		}
	}
	s.hasPrior = true
	ok := s.runAll(llr)
	s.hasPrior = false
	return ok
}

// refine detects the voice burst again with the bits of fixed that known
// marks, which its FEC fixed, as priors of cohRefinePrior nats, and writes
// the new log-likelihood ratios into llr.
func (s *cohStage) refine(fixed *layer2.RawBurst, known *[layer2.BurstBits]bool, llr *[layer2.BurstBits]int16) bool {
	clear(s.prior[:])
	for k := range burstSymbols {
		lp := &s.prior[cachSymbols+k]
		for b := range 2 {
			i := 2*k + b
			if !known[i] {
				continue
			}
			bit := unpacked.Bit(fixed[i/8] >> (7 - i%8) & 1)
			for d := range 4 {
				b0, b1 := dibitOfDigit(d)
				v := b0
				if b == 1 {
					v = b1
				}
				if v == bit {
					lp[d] += cohRefinePrior
				}
			}
		}
	}
	s.hasPrior = true
	ok := s.runAll(llr)
	s.hasPrior = false
	return ok
}

// run runs the trellis n symbols from slot symbol g in direction dir, from
// the phase of the known symbols at g.
func (s *cohStage) run(g, n, dir int, scale float32) {
	m := s.model
	d := m.d
	s.g0, s.dir = g, dir
	s.steps = n - 2*d
	var syms [cohKnown]int
	for i := range cohKnown {
		syms[i] = int(s.known[g+dir*i])
	}
	s.fill(0, cohKnown, s.buf)
	rot := m.anchor(s.buf, syms[:])
	for i := range 2 * d {
		s.runPrior(i-2*d, s.tr.slp[4*i:4*i+4])
	}
	s.tr.run(s, s.steps, rot, scale, s.mu)
}

// levels returns the amplitude and the noise variance of the ring samples
// from the known symbols k0 to k1 of the slot, against their waveform at
// the phase that fits them best.
func (s *cohStage) levels(k0, k1 int) (amp, noise float64) {
	m := s.model
	s.g0, s.dir = k0, 1
	n := min(k1-k0+1, syncSymbols)
	var syms [syncSymbols]int
	for i := range n {
		syms[i] = int(s.known[k0+i])
	}
	s.fill(0, n, s.buf)
	var c complex128
	var e float64
	ph := 0.0
	cnt := 0
	for k := range n - 2*m.d {
		w := 0
		for i := range 2*m.d + 1 {
			w = w<<2 | (syms[k+i]+3)/2
		}
		for j := range m.sps {
			x := complex128(s.buf[(k+m.d)*m.sps+j])
			c += x * complex128(m.tpl[w*m.sps+j]) * cmplx.Exp(complex(0, -ph))
			e += real(x)*real(x) + imag(x)*imag(x)
			cnt++
		}
		ph += m.h * math.Pi * float64(syms[k])
	}
	if cnt == 0 {
		return 0, 0
	}
	amp = cmplx.Abs(c) / float64(cnt)
	noise = e/float64(cnt) - amp*amp
	return amp, noise
}
