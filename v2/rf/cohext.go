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
	// fitVar is the variance of freq, in Hz squared.
	fitVar float64
	// The burst under detection: the scale of the trellis metric, its
	// known runs, and the bit priors of its detection again.
	scale    float32
	segs     [2][2]int
	nseg     int
	prior    [slotSymbols][4]float32
	hasPrior bool
	// prevOK marks a burst whose first half runs again from the SYNC of
	// the slot before, prevSyms at slot symbol prevK0, and cross that run;
	// lastT0 and lastSync are the time of symbol 0 and the SYNC of the last
	// burst, 0 for none.
	prevOK   bool
	cross    bool
	prevK0   int
	prevSyms [syncSymbols]int8
	lastT0   float64
	lastSync enums.SyncPattern
	// bs marks a burst of a BS stream. The backward run of a burst with a
	// SYNC of one goes on into the second half of the slot before, whose
	// log-APPs it keeps in next, and nextOK marks them; qllr holds the
	// ratios of each burst of the queue of softExt, for the second half
	// of the burst before to take them.
	bs     bool
	next   [cohNext][4]float32
	nextOK bool
	// mine holds the bit priors of the second half of the last burst,
	// slot mineSlot, that its detection again set, and carry those of the
	// slot before the burst under detection, for its backward run there.
	mine     [cohNext][4]float32
	mineOK   bool
	mineSlot int64
	carry    [cohNext][4]float32
	carryOK  bool
	qllr     [extQueue][layer2.BurstBits]int16
	// priorK gives the known symbols their prior: for a SYNC, not for an
	// EMB that the core decoded.
	priorK bool
	sps    float64
	// The SYNC scan: the templates of the voice and the data SYNC of each
	// of nsrc sources, and the ring samples under test.
	scanTpl [4][2][cohScanLen]complex64
	scanX   [cohScanLen]complex64
	nsrc    int
	// peakOK marks a SYNC that scan found ending at core symbol peakAt,
	// of source peakSrc, whose share peakRho a later symbol may top.
	peakOK   bool
	peakAt   int64
	peakRho  float64
	peakSrc  int
	peakData bool
	base     int64
}

// newCohStage returns the coherent stage of iqRx r, with the settings
// opt.
func newCohStage(r *iqRx, opt *options) *cohStage {
	m := newCPMModel(1, cohSPS, 0.27)
	s := &cohStage{rx: r, model: m, mu: opt.PSPGain, llrK: opt.LLRScale, dt: cohDT}
	if s.llrK == 0 {
		s.llrK = 1
	}
	s.tr = newTrellis(m, 2*slotSymbols+2*runMargin, 2*slotSymbols+2*runMargin, 0)
	s.buf = make([]complex64, (syncSymbols+2)*cohSPS)
	s.nsrc = min(opt.Sources, len(s.scanTpl))
	for i := range s.nsrc {
		voice, data, _ := sourceAt(i).SyncPatterns()
		s.scanTemplate(voice, &s.scanTpl[i][0])
		s.scanTemplate(data, &s.scanTpl[i][1])
	}
	return s
}

// Layout of the coherent SYNC scan: the samples of the waveform of a SYNC
// that it correlates, without the first and last symbol, in segments that
// it adds without their phase, so that a carrier offset of a few hundred
// Hz does not cancel them, and the sample offsets it tries around the
// timing of the core.
const (
	cohScanLen  = (syncSymbols - 2) * cohSPS
	cohScanSegs = 4
	cohScanOff  = 2
	// cohScanRho is the least share of the power of the samples that the
	// waveform of a SYNC must explain for scan.
	cohScanRho = 0.45
	// cohScanWait is the number of symbols after a peak that scan waits
	// for a higher one.
	cohScanWait = 3
)

// scanTemplate writes the conjugate waveform of SYNC p, symbols 1 to 22,
// at cohSPS samples per symbol, into tpl.
func (s *cohStage) scanTemplate(p enums.SyncPattern, tpl *[cohScanLen]complex64) {
	m := s.model
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	ph := 0.0
	for k := 1; k < syncSymbols-1; k++ {
		w := 0
		for i := -1; i <= 1; i++ {
			w = w<<2 | (int(syms[k+i])+3)/2
		}
		rot := complex64(cmplx.Exp(complex(0, -ph)))
		for j := range m.sps {
			tpl[(k-1)*m.sps+j] = m.tpl[w*m.sps+j] * rot
		}
		ph += m.h * math.Pi * float64(syms[k])
	}
}

// scan tests whether the last symbols of the core c end the SYNC of a
// source, at a few sample offsets around the timing of the core, by the
// share of the power of the ring samples that its waveform explains.
func (s *cohStage) scan(c *leanCore) (int, bool, int64, bool) {
	r := s.rx
	if r.yN < cohHist/2 {
		return 0, false, 0, false
	}
	sps := float64(samplesPerSy) / 2
	// The first sample of symbol 1 of the SYNC that ends at the last
	// symbol.
	t := symTime(c, c.ns-1) + s.dt - float64(syncSymbols-2)*sps - float64(cohSPS/2)
	b0 := int64(math.Round(t))
	if b0-cohScanOff < r.yN-cohHist || b0+cohScanOff+cohScanLen > r.yN {
		return 0, false, 0, false
	}
	best, bestSrc, bestData := 0.0, 0, false
	for o := -cohScanOff; o <= cohScanOff; o++ {
		var e float64
		x := s.scanX[:]
		for i := range x {
			v := r.yHist[(b0+int64(o)+int64(i))&(cohHist-1)]
			x[i] = v
			e += float64(real(v)*real(v) + imag(v)*imag(v))
		}
		if e == 0 {
			continue
		}
		for i := range s.nsrc {
			for d := range 2 {
				tpl := &s.scanTpl[i][d]
				var m float64
				const seg = cohScanLen / cohScanSegs
				for g := range cohScanSegs {
					var c complex64
					for j := g * seg; j < (g+1)*seg; j++ {
						c += x[j] * tpl[j]
					}
					m += float64(real(c)*real(c) + imag(c)*imag(c))
				}
				// Each segment explains at most its share of the power.
				m *= cohScanSegs / (e * cohScanLen)
				if m > best {
					best, bestSrc, bestData = m, i, d == 1
				}
			}
		}
	}
	// The SYNC ends at the symbol where the share peaks over cohScanWait
	// symbols: a side lobe a symbol off falls short of it.
	// A peak older than that is from before the core last locked.
	if s.peakOK && c.ns-1-s.peakAt > 2*cohScanWait {
		s.peakOK = false
	}
	if best >= cohScanRho && (!s.peakOK || best > s.peakRho) {
		s.peakOK, s.peakAt = true, c.ns-1
		s.peakRho, s.peakSrc, s.peakData = best, bestSrc, bestData
	}
	back := c.ns - 1 - s.peakAt
	if !s.peakOK || back < cohScanWait {
		return 0, false, 0, false
	}
	s.peakOK = false
	return s.peakSrc, s.peakData, back, true
}

// symTime returns the time of symbol k of the core, in samples of the
// ring of channel filter samples. The DiscReceiver counts its input
// samples, one per ring sample, as two baseband samples.
func symTime(c *leanCore, k int64) float64 {
	t := float64(c.mfStep*(c.n-1)+c.center-(c.ns-1-k)*samplesPerSy) + float64(c.mfStep)*float64(c.lastAt)
	return t / 2
}

// sample returns the ring sample at time t, interpolated by Catmull-Rom;
// fill rotates it by the carrier residual.
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
	return p1 + 0.5*c*(p2-p0+c*(2*p0-5*p1+4*p2-p3+c*(3*(p1-p2)+p3-p0)))
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
		// The carrier phase moves linearly over the samples of a symbol,
		// but across a move of the oscillator.
		ph0 := s.phaseAt(start)
		dph := (s.phaseAt(start+float64(sps-1)*step) - ph0) / float64(sps-1)
		sn, cs := math.Sincos(ph0)
		rot := complex64(complex(cs, sn))
		sn, cs = math.Sincos(dph)
		rs := complex64(complex(cs, sn))
		for j := range sps {
			v := s.sample(start+float64(j)*step) * rot
			rot *= rs
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
	if v, ok := s.prevAt(j); ok {
		lp[(v+3)/2] = cohKnownPrior
	}
	if j < 0 && j >= -cohNext && s.carryOK {
		for d := range lp {
			lp[d] += s.carry[j+cohNext][d]
		}
	}
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
	if s.dir < 0 && j < 0 && j >= -cohNext && s.nextOK {
		copy(s.next[j+cohNext][:], app)
		return
	}
	if j < cachSymbols || j >= slotSymbols {
		return
	}
	// The run from the SYNC of the slot before and the backward run of
	// the burst detect the first half from both sides of it: their mean
	// log-APPs spread less than either, as TierCoherent detects across the
	// slots.
	if s.cross {
		if j >= syncFirstSym || s.dist[j] == math.MaxInt {
			return
		}
		ma, mb := app[0], s.app[j][0]
		for i := 1; i < 4; i++ {
			ma, mb = max(ma, app[i]), max(mb, s.app[j][i])
		}
		for i := range 4 {
			s.app[j][i] = 0.5 * ((app[i] - ma) + (s.app[j][i] - mb))
		}
		return
	} else if dist >= s.dist[j] {
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

// fineTime moves t0 to the offset of the symbol timing, within cohDTSpan
// of dt, at which the known symbols k0 to k1 correlate best, and returns
// the move. The caller moves dt by it when the symbols are a SYNC: the
// move of a field that holds none is noise.
func (s *cohStage) fineTime(k0, k1 int) float64 {
	t0, best, bestD := s.t0, -1.0, 0.0
	for d := -cohDTSpan; d <= cohDTSpan+1e-9; d += cohDTStep {
		s.t0 = t0 + d
		if a, _ := s.levels(k0, k1); a > best {
			best, bestD = a, d
		}
	}
	s.t0 = t0 + bestD
	return bestD
}

// cohFreqGain is the least weight of each fit of the carrier offset, from
// a SYNC or from the detected symbols of a burst, and cohFreqSpan the
// most, in Hz, that the fits move it from the mean frequency of the I/Q
// samples. The first fit may move it by up to cohFreqFirst: a lock can
// come while the oscillator still pulls in, so that the mean is far off.
// The fits weigh by their variance against that of the offset, as a
// Kalman filter: cohFreqVar, in Hz squared, for the mean frequency, and
// cohFitVar over the square of the SNR of the SYNC for the fit of 24
// symbols, which the sweep measured from -0.5 to 4.5 dB Eb/N0.
const (
	cohFreqGain  = 0.03
	cohFreqSpan  = 200
	cohFreqFirst = 1000
	cohFreqVar   = 80 * 80
	cohFitVar    = 5000
)

// track sets the carrier offset of the runs from that of the SYNC fits of
// the stream, or from the mean frequency of the I/Q samples when the fits
// are more than cohFreqSpan from it: the fits of the bursts that the ring
// took while the oscillator pulled in can be far off, and a wrong offset
// makes the next fits wrong.
func (s *cohStage) track() {
	r := s.rx
	hz := float64(r.conv.hz) + float64(r.conv.hzOf(r.afcAvg))
	if !s.freqOK || math.Abs(s.freq-hz) > cohFreqSpan {
		s.freq, s.freqOK, s.nFit, s.fitVar = hz, true, 0, cohFreqVar
	}
	s.totalHz = s.freq
}

// freqFit returns the carrier offset, in Hz, left in the ring samples of
// the known symbols k0 to k1: the phase step from symbol to symbol of
// their correlations with their waveform, less the phase of the symbols.
func (s *cohStage) freqFit(k0, k1 int) float64 {
	acc := s.freqAcc(k0, k1)
	if acc == 0 {
		return 0
	}
	return cmplx.Phase(acc) * symbolRate / (2 * math.Pi)
}

// freqAcc returns the sum of the products of the correlations of the
// known symbols k0 to k1 with the conjugate of the one before, whose phase
// freqFit takes.
func (s *cohStage) freqAcc(k0, k1 int) complex128 {
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
	return acc
}

// trackBurst fits the carrier offset again from every symbol of the burst
// that detect detected last, whose bits fixed holds after its FEC, as
// TierCoherent tracks the carrier from the symbols it detected: the SYNC
// of one burst fits it too coarsely for the trellis.
func (s *cohStage) trackBurst(fixed *layer2.RawBurst) {
	for k := range burstSymbols {
		p := 2 * k
		b0 := fixed[p/8] >> (7 - p%8) & 1
		b1 := fixed[(p+1)/8] >> (7 - (p+1)%8) & 1
		s.known[cachSymbols+k] = symbolOf(unpacked.Bit(b0), unpacked.Bit(b1))
	}
	var acc complex128
	var snr float64
	n := 0
	for k0 := cachSymbols; k0+syncSymbols <= slotSymbols; k0 += syncSymbols {
		acc += s.freqAcc(k0, k0+syncSymbols-1)
		if amp, noise := s.levels(k0, k0+syncSymbols-1); noise > 0 {
			snr += amp * amp / noise
			n++
		}
	}
	if acc == 0 || n == 0 {
		return
	}
	snr /= float64(n)
	fit := cmplx.Phase(acc) * symbolRate / (2 * math.Pi)
	if math.Abs(fit) >= cohFreqSpan {
		return
	}
	g := s.fitVar / (s.fitVar + cohFitVar/(snr*snr)/float64(n))
	s.fitVar *= 1 - g
	s.freq += max(cohFreqGain, g) * fit
	s.totalHz = s.freq
}

// cohKnownPrior is the log prior, in nats, of a known symbol, and
// cohRefinePrior that of each bit that the FEC of a voice burst fixed, for
// its detection again.
const (
	cohKnownPrior  = 4
	cohRefinePrior = 20
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
	s.base, s.bs, s.nextOK = c.base, c.bs, false
	s.carryOK = s.mineOK && s.mineSlot == b.Slot-1
	if s.carryOK {
		s.carry = s.mine
	}
	s.mineOK, s.mineSlot = false, b.Slot
	s.sps = float64(samplesPerSy) / 2
	s.t0 = symTime(c, c.base) + s.dt
	if s.t0-float64(cohSPS) < float64(r.yN-cohHist) {
		return false
	}
	s.track()
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
		s.dt += cohDTGain * s.fineTime(segs[0][0], segs[0][1])
		// The mean of the first fits, then a slow average: the offset that
		// the fits start from is no fit.
		// The noise of a fit falls with the square of the SNR of the SYNC
		// at the offset that it fits. A far first fit counts when the SYNC
		// fits that offset: at a low C/N the fit of one SYNC is noise.
		fit := s.freqFit(segs[0][0], segs[0][1])
		s.totalHz = s.freq + fit
		amp, noise := s.levels(segs[0][0], segs[0][1])
		s.totalHz = s.freq
		ok := amp > 0 && noise > 0 && math.Abs(fit) < cohFreqFirst
		if math.Abs(fit) >= cohFreqSpan {
			ok = ok && s.nFit == 0 && amp*amp/(amp*amp+noise) >= cohPlaceRho
		}
		if ok {
			snr := amp * amp / noise
			g := s.fitVar / (s.fitVar + cohFitVar/(snr*snr))
			s.fitVar *= 1 - g
			s.nFit++
			s.freq += max(cohFreqGain, g) * fit
			s.totalHz = s.freq
		}
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
	// The carrier of a BS stream stays on, so the first half of a burst
	// after one with a SYNC runs again from that SYNC.
	s.prevOK = false
	if c.bs && s.lastSync != 0 {
		k := int(math.Round((s.lastT0-s.t0)/s.sps)) + syncFirstSym
		if k > syncFirstSym-slotSymbols-cohShift && k < syncFirstSym-slotSymbols+cohShift {
			s.prevOK, s.prevK0 = true, k
			syncSyms(s.lastSync, &s.prevSyms)
		}
	}
	s.lastT0, s.lastSync = s.t0, 0
	if nseg == 1 {
		s.lastSync = b.Sync
	}
	return s.runAll(llr)
}

// cohShift is the most symbols by which the timing of the core may move
// the slot before.
const cohShift = 3

// cohNext is the number of symbols of the second half of a slot, after
// its SYNC field.
const cohNext = slotSymbols - syncFirstSym - syncSymbols

// nextInto averages the ratios of the second half of the slot before the
// last burst that detect detected, in llr, with those of the run from the
// SYNC of the last burst, as runKeep averages the first half.
func (s *cohStage) nextInto(llr *[layer2.BurstBits]int16) {
	for i := range cohNext {
		p := 2 * (syncFirstSym + syncSymbols + i - cachSymbols)
		a0, a1 := dibitLLR(s.next[i][:], s.llrK)
		llr[p] = llr[p]/2 + a0/2
		llr[p+1] = llr[p+1]/2 + a1/2
	}
}

// prevAt returns the symbol of the SYNC of the slot before at slot symbol
// j, and whether it is one.
func (s *cohStage) prevAt(j int) (int8, bool) {
	i := j - s.prevK0
	if !s.prevOK || i < 0 || i >= syncSymbols {
		return 0, false
	}
	return s.prevSyms[i], true
}

// cohPlaceRho is the least share of the power of the SYNC field that its
// waveform must explain for place to take it as a SYNC.
const cohPlaceRho = 0.3

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
	best, bestP, bestD := 0.0, enums.SyncPattern(0), 0.0
	for _, p := range [...]enums.SyncPattern{voice, data} {
		var syms [syncSymbols]int8
		syncSyms(p, &syms)
		for i, v := range syms {
			s.known[syncFirstSym+i] = v
		}
		s.t0 = t0
		d := s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
		amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
		if rho := amp * amp / (amp*amp + max(noise, 0)); amp > 0 && rho > best {
			best, bestP, bestD = rho, p, d
		}
	}
	if best < cohPlaceRho {
		return false
	}
	s.dt += cohDTGain * bestD
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
	d := s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
	amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
	if amp <= 0 || amp*amp/(amp*amp+max(noise, 0)) < cohVerifyRho {
		return false
	}
	s.dt += cohDTGain * d
	return true
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
	if s.bs && s.nseg == 1 {
		s.nextOK = true
		s.run(g1, g1+cohNext+1+runMargin, -1, s.scale)
	} else {
		s.run(g1, g1-lo+1+runMargin, -1, s.scale)
	}
	if s.prevOK {
		s.cross = true
		s.run(s.prevK0, syncFirstSym-s.prevK0+runMargin, 1, s.scale)
		s.cross = false
	}
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
			// The ratios of the trellis are llrK times its metric, as
			// TierCoherent divides its priors by its LLRScale.
			lp[d] = (bitPrior(w*ext[2*k], b0) + bitPrior(w*ext[2*k+1], b1)) / s.llrK
		}
	}
	s.hasPrior = true
	ok := s.runAll(llr)
	s.hasPrior = false
	copy(s.mine[:], s.prior[slotSymbols-cohNext:])
	s.mineOK = true
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
	copy(s.mine[:], s.prior[slotSymbols-cohNext:])
	s.mineOK = true
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
		if v, ok := s.prevAt(g + dir*i); ok {
			syms[i] = int(v)
		} else {
			syms[i] = int(s.known[g+dir*i])
		}
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
