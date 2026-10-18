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
// survivor, as the engine of Tier3 detects, run forward and backward from the
// known symbols of the burst, its SYNC or the EMB that the core decoded.
// It replaces the log-likelihood ratios of the core for the decoders of
// softExt. On discriminator audio, discPhase rebuilds the ring from the
// samples of the DiscReceiver.
type cohStage struct {
	// rx is the receiver of the ring on I/Q, or ph on audio, and f the
	// front end of its DiscReceiver.
	rx *iqRx
	ph *discPhase
	f  *audioFront
	// trialing scales the phase of the samples of an audio ring by
	// 1+trial, for the fit of its deviation.
	trial    float64
	trialing bool
	// easySNR is the SNR of the levels from which a burst is easy:
	// cohEasySNR, or discEasySNR on an audio ring.
	easySNR float64
	// skipSNR, when above 0, is the SNR of the levels from which detect
	// leaves a burst to the ratios of the core, and locked marks a stage
	// that verifies no SYNC of the search.
	skipSNR float64
	locked  bool
	model   *cpmModel
	tr      *trellis
	mu      float32
	llrK    float32
	buf     []complex64
	known   [slotSymbols]int8
	isK     [slotSymbols]bool
	app     [slotSymbols][4]float32
	dist    [slotSymbols]int
	// The run under way: its first symbol in the slot, its direction and
	// steps, the time of slot symbol 0 in samples of the ring and the
	// carrier rotation per sample.
	g0    int
	dir   int
	steps int
	t0    float64
	// dt is the offset of the core's symbol timing that the SYNC of the
	// bursts finds, in ring samples, followed from burst to burst, and
	// relock marks a stream whose first SYNC sets it.
	dt      float64
	relock  bool
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
	ampAvg   float64
	noiseAvg float64
	// easy marks a burst whose levels are clear enough that it runs only
	// from its own known symbols.
	easy     bool
	segs     [2][2]int
	nseg     int
	prior    [slotSymbols][4]float32
	hasPrior bool
	// prevOK marks a burst whose first half and SYNC field run again from
	// the SYNC of the slot before, prevSyms at slot symbol prevK0, and
	// cross that run; lastT0 and lastSync are the time of symbol 0 and the
	// SYNC of the last burst, 0 for none.
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
	// div is the number of baseband samples per ring sample, rate the
	// rate of the ring, and step the ring samples per sample of the model.
	div, rate, step float64
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
	// hFit is the modulation index that the SYNC fits of the streams
	// measured on I/Q, hVar the variance of its log, and hNew marks a
	// stream that no fit measured yet.
	hFit float64
	hVar float64
	hNew bool
}

// Settings of cohStage.fitIndex. cohSyncIndex is the share of the
// modulation index of a signal that the fit of its SYNC measures: the
// channel filter rounds the phase of the outer symbols of the SYNC, so that
// a model of a lower index fits it best, by 2.3% from 1 to 12 dB C/N in
// the sweep. The variance of the log of the index starts at cohIndexStart,
// a deviation 5% off, and grows by cohIndexWalk at each stream, which may
// come from another transmitter. A fit of a SYNC whose waveform explains
// the share rho of the power of its samples has the variance cohIndexVar0
// plus cohIndexVar times (1-rho)/rho, the noise over the signal, as the
// sweep measured from 1 to 12 dB C/N. The model takes the fit when it
// moves by cohIndexMove, as a share.
const (
	cohSyncIndex  = 0.977
	cohIndexStart = 0.05 * 0.05
	cohIndexWalk  = 0.03 * 0.03
	cohIndexVar0  = 1e-4
	cohIndexVar   = 2e-3
	cohIndexMove  = 0.002
)

// newCohStage returns the coherent stage of iqRx r, with the settings
// opt.
func newCohStage(r *iqRx, opt *options) *cohStage {
	s := newCohStageOf(opt)
	s.rx, s.f = r, &r.d.f
	s.setRing(float64(r.conv.decim))
	return s
}

// setRing sets the ring of s to div baseband samples per ring sample.
func (s *cohStage) setRing(div float64) {
	s.div, s.rate = div, basebandRate/div
	s.dt = cohDT
	if div == 4 {
		s.dt = cohDT12
	}
	s.sps = samplesPerSy / div
	s.step = s.sps / float64(s.model.sps)
}

// ringTime returns the time of symbol k of the core, in samples of the
// ring.
func (s *cohStage) ringTime(c *leanCore, k int64) float64 {
	return symTime(c, k) * 2 / s.div
}

// newCohStageOf returns a coherent stage with the settings opt, without
// its ring.
func newCohStageOf(opt *options) *cohStage {
	sps := cohSPS
	if opt.CohSPS != 0 {
		sps = opt.CohSPS
	}
	m := newCPMModel(1, sps, 0.27)
	s := &cohStage{model: m, hFit: 0.27 * cohSyncIndex, hVar: cohIndexStart, hNew: true, mu: opt.PSPGain, llrK: opt.LLRScale, dt: cohDT, easySNR: cohEasySNR, skipSNR: opt.CohSkipSNR, locked: opt.CohLocked}
	if s.llrK == 0 {
		s.llrK = 1
	}
	s.tr = newTrellis(m, cohMaxRun, cohMaxRun, 0)
	s.setRing(2)
	s.buf = make([]complex64, (syncSymbols+2)*sps)
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
	hist, n, lo := s.ring()
	if n < cohHist/2 || s.step != 1 || s.div != 2 {
		return 0, false, 0, false
	}
	sps := float64(samplesPerSy) / 2
	// The first sample of symbol 1 of the SYNC that ends at the last
	// symbol.
	t := symTime(c, c.ns-1) + s.dt - float64(syncSymbols-2)*sps - float64(cohSPS/2)
	b0 := int64(math.Round(t))
	if b0-cohScanOff < lo || b0+cohScanOff+cohScanLen > n {
		return 0, false, 0, false
	}
	best, bestSrc, bestData := 0.0, 0, false
	for o := -cohScanOff; o <= cohScanOff; o++ {
		var e float64
		x := s.scanX[:]
		for i := range x {
			v := hist[(b0+int64(o)+int64(i))&(cohHist-1)]
			x[i] = v
			e += float64(real(v)*real(v) + imag(v)*imag(v))
		}
		if e == 0 {
			continue
		}
		ns := noiseShare(x)
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
				if m > best && 1-m <= cohResidual*ns {
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

// ring returns the ring of samples, the number of samples it took and the
// oldest that it holds.
func (s *cohStage) ring() ([]complex64, int64, int64) {
	if p := s.ph; p != nil {
		return p.hist, p.n, max(p.lo, p.n-cohHist)
	}
	return s.rx.yHist, s.rx.yN, s.rx.yN - cohHist
}

// sample returns the ring sample at time t, interpolated by Catmull-Rom;
// fill rotates it by the carrier residual.
func (s *cohStage) sample(t float64) complex64 {
	if s.trialing || s.ph != nil && s.ph.lazy {
		return s.ph.trialAt(t, s.t0, s.trial, s.totalHz)
	}
	hist, n, lo := s.ring()
	i := int64(math.Floor(t))
	mu := float32(t - float64(i))
	at := func(k int64) complex64 {
		if k < 0 || k < lo || k >= n {
			return 0
		}
		return hist[k&(cohHist-1)]
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
	if r == nil {
		return -2 * math.Pi * sign * s.totalHz * (b - a) / s.rate
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
	return -2 * math.Pi * sign * ph / s.rate
}

// fill writes the samples of slot symbols k0 to k1-1 of the run into x,
// as engine.fillRange: time reversed and conjugated for a backward run.
func (s *cohStage) fill(k0, k1 int, x []complex64) {
	sps := s.model.sps
	for i := k0; i < k1; i++ {
		k := s.g0 + s.dir*i
		tc := s.t0 + float64(k)*s.sps
		step := s.step
		start := tc - float64(sps/2)*step
		if s.dir < 0 {
			start = tc + float64(sps/2)*step - 2/s.div
			step = -step
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
	// the burst detect the first half and the SYNC field from both sides:
	// their mean log-APPs spread less than either, as the engine of Tier3 detects
	// across the slots. The EMB of a burst B to F, where its own runs
	// start, gains the most.
	if s.cross {
		if j >= syncFirstSym+syncSymbols || s.dist[j] == math.MaxInt {
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
// timing of the core that it starts from, in ring samples, on a ring at
// 24 kHz and at 12 kHz, the offsets it tries around the last, and at the
// first SYNC of a stream, in samples at 24 kHz, and the weight of each
// SYNC in it.
const (
	cohDT     = 1.0
	cohDT12   = -0.4
	cohDTSpan = 0.6
	cohDTWide = 2.4
	cohDTStep = 0.2
	cohDTGain = 0.2
)

// fineTime moves t0 to the offset of the symbol timing, within cohDTSpan
// of dt, at which the known symbols k0 to k1 correlate best, and returns
// the move. The caller moves dt by it when the symbols are a SYNC: the
// move of a field that holds none is noise.
func (s *cohStage) fineTime(k0, k1 int) float64 {
	return s.timeIn(k0, k1, cohDTSpan)
}

// syncTime returns the move of dt that the SYNC of a burst, the known
// symbols k0 to k1, fits, and moves t0 by its fine timing. The timing of
// the core relative to the ring can jump at a lock: when the first SYNC of
// a stream does not fit at dt, the offset within cohDTWide at which it
// fits best sets it, if the SYNC fits there.
func (s *cohStage) syncTime(k0, k1 int) float64 {
	relock := s.relock
	s.relock = false
	rho := func() float64 {
		a, n := s.levels(k0, k1)
		return a * a / (a*a + max(n, 0))
	}
	if t0 := s.t0; relock {
		if r0 := rho(); r0 < cohRelockRho {
			d := s.timeIn(k0, k1, cohDTWide)
			if r := rho(); r >= cohPlaceRho && r > r0 {
				return d
			}
			s.t0 = t0
		}
	}
	return cohDTGain * s.fineTime(k0, k1)
}

// cohRelockRho is the least share of the power of the first SYNC of a
// stream that its waveform explains at dt for the fine timing to follow
// it from there.
const cohRelockRho = 0.5

// timeIn is fineTime within span samples at 24 kHz of dt.
func (s *cohStage) timeIn(k0, k1 int, span float64) float64 {
	t0, best, bestD := s.t0, -1.0, 0.0
	for d := -span; d <= span+1e-9; d += cohDTStep {
		s.t0 = t0 + d*2/s.div
		if a, _ := s.levels(k0, k1); a > best {
			best, bestD = a, d
		}
	}
	s.t0 = t0 + bestD*2/s.div
	return bestD * 2 / s.div
}

// cohFreqGain is the least weight of each fit of the carrier offset, from
// a SYNC or from the detected symbols of a burst, and cohFreqSpan the
// most, in Hz, that the fits move it from the mean frequency of the I/Q
// samples. The first fit may move it by up to cohFreqFirst: a lock can
// come while the oscillator still pulls in, so that the mean is far off.
// The fits weigh by their variance against that of the offset, as a
// Kalman filter: cohFreqVar, in Hz squared, for the mean frequency, and
// cohFitVar over the square of the SNR of the SYNC for the fit of 24
// symbols, which the sweep measured from -0.5 to 4.5 dB Eb/N0. The
// variance of the offset grows by cohFreqWalk before each burst, as its
// carrier drifts: a crystal that warms by 100 Hz/s.
const (
	cohFreqGain  = 0.03
	cohFreqSpan  = 200
	cohFreqFirst = 1000
	cohFreqVar   = 80 * 80
	cohFitVar    = 5000
	cohFreqWalk  = 10
)

// track sets the carrier offset of the runs from that of the SYNC fits of
// the stream, or, until a SYNC fits it, from the offset of the stream when
// it is more than cohFreqSpan away: the fits of the bursts that the ring
// took while the oscillator pulled in can be far off, and a wrong offset
// makes the next fits wrong. The offset of a locked stream is that of the
// SYNC fits of the core, and of another the mean frequency of the I/Q
// samples, which the symbols of a burst move by hundreds of Hz for the
// bursts of a voice call or of rate 1 data, and noise pulls toward the
// oscillator. After a fit the offset of the stream counts only when it is
// more than cohFreqFirst away, which no fit moves: the oscillator was far
// off at the lock.
func (s *cohStage) track() {
	var hz float64
	var locked bool
	if r := s.rx; r != nil {
		hz = float64(r.conv.hz) + float64(r.conv.hzOf(r.afcAvg))
		if f := &r.d.f; r.d.locked {
			hz = float64(r.conv.hz) + float64((f.dc+r.d.dcHz)/(f.gain*iqUnitsPerHz))
		}
		locked = r.d.locked
	} else {
		hz, locked = s.ph.offsetHz(), s.ph.c.locked
	}
	if d := math.Abs(s.freq - hz); !s.freqOK || s.nFit == 0 && d > cohFreqSpan || locked && d > cohFreqFirst {
		s.freq, s.freqOK, s.nFit, s.fitVar = hz, true, 0, cohFreqVar
	}
	s.fitVar += cohFreqWalk
	s.totalHz = s.freq
}

// lost reports whether the ring, whose oldest sample is lo, no longer
// holds the burst at t0, or holds audio that clipped there, whose phase is
// lost: the core detects the burst.
func (s *cohStage) lost(lo int64) bool {
	p := s.ph
	return s.t0-s.sps < float64(lo) || p != nil && p.clipped(s.t0-s.sps, s.t0+slotSymbols*s.sps)
}

// lock forgets the bursts of the stream before a lock: the runs from the
// SYNC of the slot before and the priors of its second half are of a
// stream with another timing.
func (s *cohStage) lock() {
	s.lastT0, s.lastSync, s.mineOK, s.relock, s.hNew = 0, 0, false, true, true
	s.hVar += cohIndexWalk
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
// that detect detected last, whose bits fixed holds after its FEC, as the
// engine of Tier3 tracks the carrier from the symbols it detected: the
// SYNC of one burst fits it too coarsely for the trellis.
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
	if s.f.gain <= 0 {
		return false
	}
	_, _, lo := s.ring()
	s.base, s.bs, s.nextOK = c.base, c.bs, false
	s.carryOK = s.mineOK && s.mineSlot == b.Slot-1
	if s.carryOK {
		s.carry = s.mine
	}
	s.mineOK, s.mineSlot = false, b.Slot
	s.t0 = s.ringTime(c, c.base) + s.dt
	if s.lost(lo) {
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
	if s.quiet(c, segs[0], nseg) {
		return false
	}
	if nseg == 1 {
		s.dt += s.syncTime(segs[0][0], segs[0][1])
		// The mean of the first fits, then a slow average: the offset that
		// the fits start from is no fit.
		// The noise of a fit falls with the square of the SNR of the SYNC
		// at the offset that it fits. A far fit counts when the SYNC fits
		// that offset, and better than the offset before: at a low C/N the
		// fit of one SYNC is noise, but a lock can come while the
		// oscillator still pulls in, so that the offset is far off. It
		// starts the fits again.
		a0, n0 := s.levels(segs[0][0], segs[0][1])
		fit := s.freqFit(segs[0][0], segs[0][1])
		f0 := s.freq
		s.totalHz = s.freq + fit
		amp, noise := s.levels(segs[0][0], segs[0][1])
		s.totalHz = s.freq
		ok := amp > 0 && noise > 0 && math.Abs(fit) < cohFreqFirst
		if ok && math.Abs(fit) >= cohFreqSpan {
			rho := amp * amp / (amp*amp + noise)
			ok = rho >= cohPlaceRho && rho > a0*a0/(a0*a0+max(n0, 0))
			if ok {
				s.nFit, s.fitVar = 0, cohFreqVar
			}
		}
		if ok {
			snr := amp * amp / noise
			g := s.fitVar / (s.fitVar + cohFitVar/(snr*snr))
			s.fitVar *= 1 - g
			s.nFit++
			s.freq += max(cohFreqGain, g) * fit
			s.totalHz = s.freq
		}
		s.fitDeviation(segs[0][0], segs[0][1], f0+fit-s.freq)
	}
	// The phase of a lazy audio ring is noise until a fit sets its scale.
	if p := s.ph; p != nil && p.lazy && !p.cal {
		return false
	}
	if !s.average(segs, nseg) {
		return false
	}
	s.scale = float32(2 * s.ampAvg / s.noiseAvg)
	s.easy = s.ampAvg*s.ampAvg/s.noiseAvg >= s.easySNR
	s.segs, s.nseg, s.hasPrior = segs, nseg, false
	// The carrier of a BS stream stays on, so the first half and the SYNC
	// field of a burst after one with a SYNC run again from that SYNC.
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

// average adds the levels of the known runs segs[:nseg] of the burst to
// their mean over the bursts, and reports false when they have none.
func (s *cohStage) average(segs [2][2]int, nseg int) bool {
	var amp, noise float64
	for _, sg := range segs[:nseg] {
		a, v := s.levels(sg[0], sg[1])
		amp += a / float64(nseg)
		noise += v / float64(nseg)
	}
	if amp <= 0 || noise <= 0 {
		return false
	}
	// The levels of a few known symbols are noisy at a low C/N: they
	// average over the bursts, each by its known symbols.
	n := float64(segs[nseg-1][1] - segs[0][0] + 1)
	if nseg == 2 {
		n = float64(segs[0][1] - segs[0][0] + 1 + segs[1][1] - segs[1][0] + 1)
	}
	if s.ampAvg == 0 {
		s.ampAvg, s.noiseAvg = amp, noise
	} else {
		g := n / (n + cohLevelSyms)
		s.ampAvg += g * (amp - s.ampAvg)
		s.noiseAvg += g * (noise - s.noiseAvg)
	}
	return true
}

// quiet reports whether the stream is clear enough, by the levels of the
// bursts before, that detect leaves the burst to the ratios of the core,
// once cohQuietFits SYNC fits set its carrier and timing. The levels then
// follow the SYNC seg of a burst at the timing and the carrier offset of
// the stream, and the fits wait, as they move little at that SNR; nseg is
// 2 for the EMB of a burst without a SYNC.
func (s *cohStage) quiet(c *leanCore, seg [2]int, nseg int) bool {
	if !s.clear(c) {
		return false
	}
	if nseg == 1 {
		if amp, noise := s.levels(seg[0], seg[1]); amp > 0 && noise > 0 {
			g := float64(syncSymbols) / (syncSymbols + cohLevelSyms)
			s.ampAvg += g * (amp - s.ampAvg)
			s.noiseAvg += g * (noise - s.noiseAvg)
		}
	}
	s.lastSync = 0
	return true
}

// clear reports whether the levels of the bursts before and of the core c
// show a clear stream, whose bursts the core detects and places.
func (s *cohStage) clear(c *leanCore) bool {
	if s.skipSNR <= 0 {
		return false
	}
	return c.dev*c.dev >= cohClearCore*c.discVar || s.nFit >= cohQuietFits && s.ampAvg*s.ampAvg >= s.skipSNR*s.noiseAvg && c.dev*c.dev >= cohQuietCore*c.discVar
}

// cohQuietFits is the number of SYNC fits of the carrier after which a
// clear stream leaves its bursts to the core, and cohQuietCore the least
// SNR of the levels of the core, the square of the deviation of level 1
// over the noise variance, for it: the core detects and places the bursts
// by its own levels, which can take some bursts to settle after a lock.
// From cohClearCore the core alone shows a clear stream: a transmitter
// whose phase strays from the model, as one whose modulator passes no low
// frequencies, then loses no burst to the coherent detection.
const (
	cohQuietFits = 4
	cohQuietCore = 4
	cohClearCore = 16
)

// cohLevelSyms is the number of known symbols that the levels of the
// trellis average over.
const cohLevelSyms = 48

// cohEasySNR is the SNR of the levels, the power of the signal over that of
// the noise, from which a burst runs only from its own known symbols and
// detects no voice burst again: about 7 dB Eb/N0, 1.5 dB above the
// Eb/N0 of 90% of every payload, where those runs gain nothing.
const cohEasySNR = 6

// cohShift is the most symbols by which the timing of the core may move
// the slot before.
const cohShift = 3

// cohMaxRun is the most intervals of a run of the trellis: the run from
// the SYNC of the slot before across the first half of the burst.
const cohMaxRun = slotSymbols + syncSymbols + cohShift + runMargin

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
// most of its power, when that is at least cohPlaceRho of it. On a clear
// stream a burst that the core does not place has no SYNC.
func (s *cohStage) place(c *leanCore, b *Burst, j int32) bool {
	if s.f.gain <= 0 || !s.freqOK || s.clear(c) {
		return false
	}
	_, _, lo := s.ring()
	t0 := s.ringTime(c, c.base) + s.dt
	if t0-s.sps < float64(lo) {
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
// waveform must explain for verify, and cohVerifyPre the least at the
// timing of the core, before the fine timing.
const (
	cohVerifyRho = 0.35
	cohVerifyPre = 0.2
)

// cohResidual is the most share of the power of a SYNC field that its
// waveform may leave for verify and scan on I/Q, in multiples of the share
// of the noise in the power of its samples. A SYNC leaves the noise and
// the error of the model: from -2 to 4 dB C/N, at most 3 times the noise.
// The data of a burst whose symbols match a SYNC in most signs leaves half
// of the signal or more: from 8 dB C/N, over 4 times the noise. The share
// of the power alone passes such data once the carrier offset is known, as
// after an unmodulated carrier or at the start of a stream.
const cohResidual = 4

// noiseShare returns the share of the noise in the power of the samples x,
// by the M2M4 estimate: the FM carrier has a constant envelope, so that the
// mean square of its power is the square of the mean, while that of noise
// is twice it.
func noiseShare(x []complex64) float64 {
	var p, p2 float64
	for _, v := range x {
		q := float64(real(v)*real(v) + imag(v)*imag(v))
		p += q
		p2 += q * q
	}
	if p <= 0 {
		return 1
	}
	p /= float64(len(x))
	p2 /= float64(len(x))
	return 1 - math.Sqrt(max(2*p*p-p2, 0))/p
}

// verify reports whether the last symbols of the core c end SYNC p by
// the coherent test of place: the ring samples of the SYNC field against
// its waveform, at the carrier offset that the oscillator and the core
// give, or that the stream had.
func (s *cohStage) verify(c *leanCore, p enums.SyncPattern) bool {
	f := s.f
	if f.gain <= 0 {
		return false
	}
	_, _, lo := s.ring()
	base := c.ns - 1 - leanSyncEnd
	s.t0 = s.ringTime(c, base) + s.dt
	if s.t0-s.sps < float64(lo) {
		return false
	}
	s.verifyHz()
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	for i, v := range syms {
		s.known[syncFirstSym+i] = v
	}
	if s.seedDeviation(c, syncFirstSym, syncFirstSym+syncSymbols-1) {
		s.verifyHz()
	}
	// Most candidates of the search on noise fail at the timing of the
	// core already, which the fine timing would test six times more.
	if amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1); amp <= 0 || amp*amp/(amp*amp+max(noise, 0)) < cohVerifyPre {
		return false
	}
	d := s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
	amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
	// The ring of audio holds phasors of unit power, whose envelope shows
	// no noise.
	rho := amp * amp / (amp*amp + max(noise, 0))
	if amp <= 0 || rho < cohVerifyRho || s.ph == nil && 1-rho > cohResidual*noiseShare(s.buf[:syncSymbols*s.model.sps]) {
		return false
	}
	s.dt += cohDTGain * d
	return true
}

// verifyHz sets the carrier offset of verify: that of the stream, or
// before the first fit the one that the oscillator and the core give.
func (s *cohStage) verifyHz() {
	s.totalHz = s.freq
	switch {
	case s.freqOK:
	case s.rx != nil:
		f := s.f
		s.totalHz = float64(s.rx.conv.hz) + float64(f.dc/(f.gain*iqUnitsPerHz))
	default:
		s.totalHz = s.ph.offsetHz()
	}
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
	if s.bs && s.nseg == 1 && !s.easy {
		s.nextOK = true
		s.run(g1, g1+cohNext+1+runMargin, -1, s.scale)
	} else {
		s.run(g1, g1-lo+1+runMargin, -1, s.scale)
	}
	if s.prevOK && !s.easy {
		s.cross = true
		s.run(s.prevK0, syncFirstSym+syncSymbols-s.prevK0+runMargin, 1, s.scale)
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
			// the engine of Tier3 divides its priors by its LLRScale.
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
		rot := cmplx.Exp(complex(0, -ph))
		for j := range m.sps {
			x := complex128(s.buf[(k+m.d)*m.sps+j])
			c += x * complex128(m.tpl[w*m.sps+j]) * rot
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
