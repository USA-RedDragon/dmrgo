package main

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Tier selects how a Receiver detects symbols, by compute budget.
type Tier int

// The tiers, from the least compute to the most. Every tier decodes each
// FEC field with soft decisions.
const (
	// TierDisc slices the output of an FM discriminator and its matched
	// filter. It fits a microcontroller.
	TierDisc Tier = iota
	// TierCoherent detects the symbols coherently with a trellis over a
	// window of three symbols that carries the carrier phase with each
	// survivor, at 5 samples per symbol, and detects each voice burst
	// again with the bits that its FEC fixed. It fits a Cortex-M4F class
	// microcontroller.
	TierCoherent
	// TierExact detects over a window of three symbols with one of 52
	// carrier phases in the trellis state, and detects each voice burst
	// again with the bits that its FEC fixed. It is for a PC.
	TierExact
	// TierMax is TierExact over a window of five symbols with 200 carrier
	// phases, the exact count for h = 0.27, at about 50 times the compute.
	// It is for offline decoding on a PC and needs about 100 MB.
	TierMax
)

// Geometry and limits of the receiver, in samples at basebandRate unless
// noted.
const (
	slotSymbols  = 144
	slotSamplesN = slotSymbols * samplesPerSy
	syncFirstSym = cachSymbols + 54
	// maxGapSlots is the most slots between two anchors that one trellis
	// run covers, and exactGapSlots the most for the exact-phase trellis.
	maxGapSlots   = 7
	exactGapSlots = 3
	// searchStep is the spacing of the positions the SYNC search tries.
	searchStep = 2
	// lockMetric is the coherent SYNC metric at which the search locks.
	lockMetric = 0.4
	// discLock is the discriminator SYNC correlation at which the search of
	// TierDisc locks.
	discLock = 0.9
	// anchorMetric is the share of the expected SYNC metric at which a slot
	// counts as an anchor.
	anchorMetric = 0.45
	// levelGain is the weight of each anchor in the level and noise
	// estimates, high enough to follow a receiver gain that settles.
	levelGain = 0.5
	// presenceFactor is the ratio of the mean power of a slot to the noise
	// power at which the carrier counts as on.
	presenceFactor = 1.5
	// lockSpanHz is the carrier error, in Hz, that the receiver searches
	// at lock: the spectrum estimate is no closer than that.
	lockSpanHz = 300.0
	// wideChunk is the number of baseband samples that the receiver filters
	// before it searches or follows the stream again.
	wideChunk = 512
	// warmSlots is the number of slots after the first anchor that the
	// receiver detects twice, the first time only to settle the carrier
	// estimate.
	warmSlots = 6
	// lostSlots is the number of slots without carrier after which the
	// receiver unlocks.
	lostSlots = 24
	// timingForget and freqForget weigh the old measurements in the timing
	// fit and the carrier estimate per anchor.
	timingForget = 0.97
	freqForget   = 0.9
)

// ReceiverConfig sizes a Receiver. NewReceiver allocates every buffer from
// it, and Write allocates nothing.
type ReceiverConfig struct {
	// Rate is the input sample rate in Hz.
	Rate float64
	// Signed selects signed 8-bit I/Q (HackRF) instead of unsigned (rtl_sdr).
	Signed bool
	// Tier selects the detection.
	Tier Tier
	// Options, when not nil, replaces the settings of the tier.
	Options *Options
	// MaxWrite is the largest number of bytes in one Write. 0 means 1 << 16.
	MaxWrite int
	// LookbackSlots is the number of 30 ms slots before the first SYNC it
	// locks to that the receiver keeps and decodes. 0 means 16.
	LookbackSlots int
}

// anchorInfo is the measurement of a slot whose SYNC field holds a known
// pattern.
type anchorInfo struct {
	pattern enums.SyncPattern
	metric  float64
	rot     complex64
	amp     float64
	noise   float64
}

// slotState is one slot of a locked stream on its way through detection.
type slotState struct {
	index   int64
	start   float64
	present bool
	anchor  bool
	a       anchorInfo
	// hasPrior is set while a refinement detects the slot again with the
	// log priors of Receiver.prior.
	hasPrior bool
}

// Receiver recovers DMR bursts from a stream of I/Q bytes. It locks to the
// slot timing of the first SYNC it finds, follows the timing and the
// carrier through the transmission, and returns every burst it recovers.
type Receiver struct {
	cfg  ReceiverConfig
	opt  Options
	fe   *frontEnd
	wide []complex64
	// inChunk is the most input bytes whose baseband samples fit in wide.
	inChunk int
	chz     *channelizer
	spec    *spectrum
	search  *syncSearch
	model   *cpmModel
	tr      *trellis
	xt      *xtrellis
	maxRun  int
	lp      []float32
	slp     []float32
	buf     []complex64
	tapp    []float32
	tk      *tracker
	out     []Received
	pulse   phasePulse
	ddSyms  []int
	ddPh    []float64
	corrX   []complex128

	nsrc      int
	searchAt  int64
	lockAt    int64
	candAt    int64
	candPos   int64
	cand      syncHit
	warm      bool
	lockFreq  float64
	warmStart int64
	locked    bool
	src       int
	bs        bool
	t0        float64
	period    float64
	fit       [5]float64
	freq      float64
	fw        float64
	amp       float64
	noise     float64
	devScale  float64
	dcHz      float64
	discNoise float64
	next      int64
	lost      int

	slots    []slotState
	first    int64
	lastAnch int64
	// apps holds the symbol log-APPs of each slot of slots, four per
	// symbol, and haves the distance of each symbol from the start of the
	// trellis run that set them, for the coherent detection. The
	// discriminator detection finishes each slot at once, so apps holds one
	// slot and haves none.
	apps  []float32
	haves []int16
	// prior holds the log priors of the digits of each symbol of the slot
	// that a refinement detects again, from its decoded FEC.
	prior [slotSymbols * 4]float32

	streams int
	rep     *repairer
	// scratchRec holds a slot record while repairEntry detects a voice
	// burst again.
	scratchRec slotRec
	known      [layer2.BurstBits]bool
	h          float64
	// OffsetHz is the carrier offset of the last locked stream.
	OffsetHz float64
	// DeviationHz is the outer deviation of the last locked stream.
	DeviationHz float64
}

// NewReceiver returns a Receiver sized by cfg.
func NewReceiver(cfg ReceiverConfig) *Receiver {
	if cfg.MaxWrite == 0 {
		cfg.MaxWrite = 1 << 16
	}
	if cfg.LookbackSlots == 0 {
		cfg.LookbackSlots = 16
	}
	r := &Receiver{cfg: cfg, candAt: -1, h: 0.27}
	r.opt = TierOptions(cfg.Tier)
	if cfg.Options != nil {
		r.opt = *cfg.Options
	}
	r.nsrc = r.opt.Sources
	r.fe = newFrontEnd(cfg.Rate, cfg.Signed)
	r.wide = make([]complex64, wideChunk)
	r.inChunk = max(2*int(float64(wideChunk-4)*cfg.Rate/basebandRate), 2)
	size := (cfg.LookbackSlots + maxGapSlots + 4) * slotSamplesN
	dec := max(r.opt.DiscDecimation, 1)
	if r.opt.Coherent {
		dec = max(r.opt.ChanDecimation, 1)
	}
	r.chz = newChannelizer(size, !r.opt.Coherent, r.opt.DiscCutHz, dec)
	r.spec = newSpectrum()
	r.search = newSyncSearch(r.opt.Coherent, dec)
	if r.opt.Coherent {
		d := r.opt.Window
		r.model = newCPMModel(d, r.opt.TrellisSPS, 0.27)
		r.maxRun = (maxGapSlots + 1) * slotSymbols
		if r.opt.ExactPhase {
			r.maxRun = exactGapSlots * slotSymbols
			np := r.opt.PhaseStates
			if np == 0 {
				np = phaseStates
			}
			r.xt = newXTrellis(r.model, np, r.maxRun)
			r.lp, r.slp = r.xt.lp, r.xt.slp
		} else {
			r.tr = newTrellis(r.model, r.maxRun)
			r.lp, r.slp = r.tr.lp, r.tr.slp
		}
		r.buf = make([]complex64, (maxGapSlots+1)*slotSymbols*samplesPerSy)
		r.tapp = make([]float32, 4*(maxGapSlots+1)*slotSymbols)
		r.ddSyms = make([]int, (maxGapSlots+1)*slotSymbols)
		r.ddPh = make([]float64, (maxGapSlots+1)*slotSymbols*samplesPerSy/dec)
	}
	if r.opt.Coherent {
		r.pulse = newPhasePulse(samplesPerSy / dec)
		r.corrX = make([]complex128, syncSpan/dec)
	}
	r.slots = make([]slotState, cfg.LookbackSlots+maxGapSlots+4)
	if r.opt.Coherent {
		r.apps = make([]float32, len(r.slots)*slotSymbols*4)
		r.haves = make([]int16, len(r.slots)*slotSymbols)
	} else {
		r.apps = make([]float32, slotSymbols*4)
	}
	r.tk = newTracker()
	if r.opt.SoftFEC {
		r.rep = newRepairer(r.opt.BPTCOSD)
		r.tk.repair = r.repairEntry
		r.tk.elc = r.rep.embeddedLC
	}
	r.out = make([]Received, 0, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+2*len(r.slots)+16)
	return r
}

// Write processes the I/Q bytes in iq and returns the bursts that are
// complete. The returned slice is valid until the next call.
func (r *Receiver) Write(iq []byte) []Received {
	r.out = r.out[:0]
	for len(iq) > 0 {
		n := min(len(iq), r.cfg.MaxWrite, r.inChunk) &^ 1
		if n == 0 {
			break
		}
		w := r.fe.process(iq[:n], r.wide)
		iq = iq[n:]
		r.push(r.wide[:w])
	}
	return r.out
}

// WriteBaseband processes complex baseband samples at basebandRate, as a
// radio front end delivers them, in place of Write. The returned slice is
// valid until the next call.
func (r *Receiver) WriteBaseband(x []complex64) []Received {
	r.out = r.out[:0]
	for len(x) > 0 {
		n := min(len(x), len(r.wide))
		r.push(x[:n])
		x = x[n:]
	}
	return r.out
}

// push runs wide baseband samples through the spectrum estimate and the
// channel filters, then searches or follows the stream.
func (r *Receiver) push(x []complex64) {
	for _, v := range x {
		if r.spec.push(v) && !r.locked && r.spec.OK && math.Abs(r.spec.Hz-r.chz.ncoHz) > 50 {
			r.chz.tune(r.spec.Hz)
			r.searchAt, r.candAt = 0, -1
		}
		r.chz.push(v)
	}
	r.step(false)
}

// Flush finishes the stream at its end and returns the remaining bursts.
func (r *Receiver) Flush() []Received {
	r.out = r.out[:0]
	for range slotSamplesN {
		r.chz.push(0)
	}
	r.step(true)
	for r.locked {
		r.finishStream()
		r.step(true)
	}
	r.tk.flush(r.emit)
	return r.out
}

// emit appends burst x to the bursts that Write returns.
func (r *Receiver) emit(x Received) {
	if len(r.out) < cap(r.out) {
		r.out = append(r.out, x)
	}
}

// step searches or follows the stream over the samples that arrived.
func (r *Receiver) step(end bool) {
	for {
		if !r.locked {
			if !r.searchStep() {
				return
			}
			continue
		}
		if !r.followStep(end) {
			return
		}
	}
}

// searchStep tries the next search positions and reports whether it locked.
func (r *Receiver) searchStep() bool {
	ch := &r.chz.ch
	if from := r.chz.oldest() + slotSamplesN; r.searchAt < from {
		r.searchAt = from
	}
	if r.opt.Coherent {
		r.searchAt = r.alignSearch(r.searchAt)
	}
	for {
		if !r.opt.Coherent {
			base, u := r.chz.symAt(r.searchAt)
			step := r.chz.symStep()
			if base+int64(syncSymbols*step)+2 >= r.chz.sym.n {
				return false
			}
			rho, src, data := r.search.discAt(&r.chz.sym, base, u, step, r.nsrc)
			if rho >= discLock {
				best := r.searchAt
				for k := int64(1); k < 6; k++ {
					b, v := r.chz.symAt(r.searchAt + k)
					if q, s2, d2 := r.search.discAt(&r.chz.sym, b, v, step, r.nsrc); q > rho {
						rho, src, data, best = q, s2, d2, r.searchAt+k
					}
				}
				r.lock(float64(best), src, data, 0)
				return true
			}
		} else {
			if r.searchAt+syncSpan+3*samplesPerSy >= r.chz.count() {
				return false
			}
			h := r.search.at(ch, r.searchAt, r.nsrc)
			if h.Metric >= lockMetric && (r.candAt < 0 || h.Metric > r.cand.Metric) {
				if r.candAt < 0 {
					r.candAt = r.searchAt
				}
				r.cand, r.candPos = h, r.searchAt
			}
			if r.candAt >= 0 && r.searchAt >= r.candAt+slotSamplesN {
				best, bt := r.cand, r.candPos
				for k := int64(-searchStep); k <= searchStep; k += int64(r.search.dec) {
					if h2 := r.search.at(ch, r.candPos+k, r.nsrc); h2.Metric > best.Metric {
						best, bt = h2, r.candPos+k
					}
				}
				r.candAt = -1
				r.searchAt = bt
				r.lock(float64(bt-syncEdge*samplesPerSy), best.Source, best.Data, best.Hz)
				return true
			}
		}
		r.searchAt += searchStep
	}
}

// alignSearch returns the first channel sample from t at which the
// coherent search can start, a multiple of the decimation of the channel
// ring.
func (r *Receiver) alignSearch(t int64) int64 {
	d := int64(r.search.dec)
	return t + (d-(t%d+d)%d)%d
}

// lock starts following a stream whose SYNC starts (center of its first
// symbol) at channel sample t.
func (r *Receiver) lock(t float64, src int, data bool, hz float64) {
	r.locked = true
	r.streams++
	r.lockAt = r.searchAt
	r.src = src
	r.bs = sourceAt(src) == layer2.SyncSourceBS
	r.period = slotSamplesN
	r.t0 = t - float64(syncFirstSym*samplesPerSy)
	r.fit = [5]float64{}
	r.freq = 0
	_ = hz
	r.fw = 0
	r.amp, r.noise = 0, 0
	r.devScale = 1
	r.dcHz = 0
	r.lost = 0
	back := int64(0)
	for back < int64(r.cfg.LookbackSlots) && r.slotTime(-back-1)-2*slotSamplesN > float64(r.chz.oldest()) {
		back++
	}
	r.first = -back
	r.next = -back
	r.warm, r.warmStart = r.opt.WarmUp, -back
	r.lastAnch = math.MinInt64
	r.tk.reset(r.bs)
	if r.opt.Coherent {
		s := r.resetSlot(0)
		voice, dp, _ := sourceAt(src).SyncPatterns()
		p := voice
		if data {
			p = dp
		}
		var best anchorInfo
		bestF, bestTau := 0.0, 0.0
		for tau := -2.0; tau <= 2; tau += 0.25 {
			for f := -lockSpanHz; f <= lockSpanHz; f += 5 {
				r.freq = f
				if a := r.syncCorr(p, r.syncSamples(s, tau)); a.metric > best.metric {
					best, bestF, bestTau = a, f, tau
				}
			}
		}
		r.freq, r.lockFreq = bestF, bestF
		r.t0 += bestTau
		r.amp, r.noise = best.amp, best.noise
	}
}

// warmFreq sets the carrier estimate and the modulation index from the
// anchors from slot 0 to j at the timing of the slot fit: the values that
// maximize the sum of their squared SYNC correlations, searched in turn.
// The discriminator detection does not use the carrier estimate, so for it
// warmFreq only reports the deviation.
func (r *Receiver) warmFreq(j int64) {
	if !r.opt.Coherent {
		r.DeviationHz = 3 * deviationHz * r.devScale
		return
	}
	r.searchFreq(j, r.lockFreq, lockSpanHz/2, 0.5)
	if !r.opt.TrackH {
		r.fw = 1 / (1 - freqForget)
		r.DeviationHz = 3 * deviationHz * r.h / 0.27
		return
	}
	best, bestH := -1.0, r.h
	for h := 0.24; h <= 0.3; h += 0.0025 {
		r.search.setH(h, &r.pulse)
		if v := r.anchorScore(j); v > best {
			best, bestH = v, h
		}
	}
	r.setH(bestH)
	r.searchFreq(j, r.freq, 5, 0.25)
	r.fw = 1 / (1 - freqForget)
}

// searchFreq sets the carrier estimate to the frequency, from center-span
// to center+span in steps of step, with the best anchorScore up to slot j.
func (r *Receiver) searchFreq(j int64, center, span, step float64) {
	best, bestF := -1.0, r.freq
	for f := center - span; f <= center+span; f += step {
		r.freq = f
		if v := r.anchorScore(j); v > best {
			best, bestF = v, f
		}
	}
	r.freq = bestF
}

// anchorScore returns the sum of the SYNC metrics of the anchors from slot
// 0 to j at the timing of the slot fit and the current carrier estimate.
func (r *Receiver) anchorScore(j int64) float64 {
	var tot float64
	for k := int64(0); k <= j; k++ {
		s := r.slot(k)
		if s.index == k && s.anchor {
			tot += r.syncCorr(s.a.pattern, r.syncSamples(s, 0)).metric
		}
	}
	return tot
}

// setH sets the modulation index of the SYNC waveforms and of the trellis.
func (r *Receiver) setH(h float64) {
	r.h = h
	r.search.setH(h, &r.pulse)
	if r.model != nil {
		r.model.setH(h)
	}
	r.DeviationHz = 3 * deviationHz * h / 0.27
}

// syncSamples interpolates the SYNC field of slot s, shifted by tau
// samples, into r.corrX with the residual carrier removed, and returns its
// energy.
func (r *Receiver) syncSamples(s *slotState, tau float64) float64 {
	ch := &r.chz.ch
	dec := r.search.dec
	t0 := r.symTime(s.index, syncFirstSym+syncEdge) + tau - float64(r.search.sps/2*dec)
	rot, step := r.rotAt(t0), r.rotAt(float64(dec))
	var e float64
	for j := range r.search.span {
		v := complex128(ch.interp(t0+float64(j*dec))) * rot
		rot *= step
		r.corrX[j] = v
		e += real(v)*real(v) + imag(v)*imag(v)
	}
	return e
}

// slotTime returns the channel sample of the center of the first CACH
// symbol of slot j.
func (r *Receiver) slotTime(j int64) float64 {
	return r.t0 + float64(j)*r.period
}

// symTime returns the channel sample of the center of symbol k (0 to 143,
// CACH first) of slot j.
func (r *Receiver) symTime(j int64, k int) float64 {
	return r.slotTime(j) + float64(k)*r.period/slotSymbols
}

// slot returns the state of slot j in the ring of slot states.
func (r *Receiver) slot(j int64) *slotState {
	return &r.slots[r.slotPos(j)]
}

// resetSlot clears the state of slot j and returns it.
func (r *Receiver) resetSlot(j int64) *slotState {
	s := r.slot(j)
	*s = slotState{index: j, start: r.slotTime(j)}
	clear(r.appOf(s))
	if r.haves != nil {
		clear(r.haveOf(s))
	}
	return s
}

// slotPos returns the position of slot j in the ring of slot states.
func (r *Receiver) slotPos(j int64) int {
	n := int64(len(r.slots))
	return int(((j % n) + n) % n)
}

// appOf returns the symbol log-APPs of slot s.
func (r *Receiver) appOf(s *slotState) []float32 {
	if len(r.apps) == slotSymbols*4 {
		return r.apps
	}
	p := r.slotPos(s.index) * slotSymbols * 4
	return r.apps[p : p+slotSymbols*4]
}

// haveOf returns the run distances of the symbols of slot s.
func (r *Receiver) haveOf(s *slotState) []int16 {
	p := r.slotPos(s.index) * slotSymbols
	return r.haves[p : p+slotSymbols]
}

// followStep measures the next slot once its samples and a lookahead have
// arrived, and detects the slots between anchors. It reports whether it
// made progress.
func (r *Receiver) followStep(end bool) bool {
	need := r.symTime(r.next, slotSymbols) + float64(2*samplesPerSy+sincTaps)
	if !end && need >= float64(r.chz.count()) {
		return false
	}
	if end && r.slotTime(r.next) >= float64(r.chz.count()) {
		return false
	}
	j := r.next
	r.next++
	s := r.resetSlot(j)
	r.measure(s)
	switch {
	case j < 0:
	case s.present && (s.anchor || r.lastAnch != math.MinInt64 && j-r.lastAnch < lostSlots):
		r.lost = 0
	default:
		r.lost++
	}
	switch {
	case !r.bs:
		r.detectBurst(s)
		if s.anchor {
			r.lastAnch = j
		}
	case s.anchor:
		r.detectUpTo(j)
		r.lastAnch = j
	case j >= 0 && j-r.first >= maxGapSlots-1:
		r.detectUpTo(j)
	}
	if r.lost >= lostSlots || r.lastAnch == math.MinInt64 && j >= 6 {
		r.finishStream()
		return true
	}
	if r.warm && j >= warmSlots {
		r.warmFreq(j)
		r.warm = false
		r.next, r.first = r.warmStart, r.warmStart
		r.lastAnch, r.lost = math.MinInt64, 0
	}
	return true
}

// finishStream detects the slots that are still pending and unlocks.
func (r *Receiver) finishStream() {
	if r.warm {
		r.warmFreq(r.next - 1)
		r.warm = false
		r.next, r.first = r.warmStart, r.warmStart
		r.lastAnch, r.lost = math.MinInt64, 0
		return
	}
	if r.bs {
		r.detectUpTo(r.next - 1)
	}
	if r.first < r.next {
		if r.opt.Coherent {
			r.coherentGap(r.first, r.next-1, true)
		}
		for k := r.first; k < r.next; k++ {
			if !r.opt.Coherent {
				r.discDetect(r.slot(k))
			}
			if !r.warm {
				r.finish(r.slot(k))
			}
		}
		r.first = r.next
	}
	r.OffsetHz = r.chz.ncoHz + r.freq
	if !r.opt.Coherent {
		r.OffsetHz = r.chz.ncoHz + r.dcHz
	}
	r.locked = false
	r.searchAt = max(int64(r.slotTime(r.next)), r.lockAt+searchStep)
	if r.opt.Coherent {
		r.searchAt = r.alignSearch(r.searchAt)
	}
	r.tk.flush(r.emit)
}

// measure classifies the SYNC field of slot s, measures its timing,
// carrier and level when it is an anchor, and tests the carrier.
func (r *Receiver) measure(s *slotState) {
	if !r.opt.Coherent {
		r.measureDisc(s)
		return
	}
	voice, data, _ := sourceAt(r.src).SyncPatterns()
	pats := [3]enums.SyncPattern{voice, data, enums.SyncMSSourcedRC}
	np := 2
	if sourceAt(r.src) == layer2.SyncSourceMS {
		np = 3
	}
	var best anchorInfo
	bestTau := 0.0
	for tau := -1.5; tau <= 1.5; tau += 0.25 {
		e := r.syncSamples(s, tau)
		for _, p := range pats[:np] {
			if a := r.syncCorr(p, e); a.metric > best.metric {
				best, bestTau = a, tau
			}
		}
	}
	energy := r.slotEnergy(s)
	expect := 1.0
	if r.amp > 0 {
		expect = r.amp * r.amp / (r.amp*r.amp + r.noise)
	}
	s.present = energy > max(presenceFactor*r.noise, r.noise+0.25*r.amp*r.amp)
	s.anchor = best.metric >= anchorMetric*expect && best.metric >= 4.0/float64(r.search.span)
	s.present = s.present || s.anchor
	if !s.anchor {
		return
	}
	s.a = best
	r.timing(s, bestTau)
	if r.amp == 0 {
		r.amp, r.noise = best.amp, best.noise
	} else {
		r.amp = (1-levelGain)*r.amp + levelGain*best.amp
		r.noise = (1-levelGain)*r.noise + levelGain*best.noise
	}
}

// timing adds the SYNC of anchor slot s, tau samples from where the fit
// put it, to the fit of the slot timing.
func (r *Receiver) timing(s *slotState, tau float64) {
	k := s.index
	r.fit[0] = timingForget*r.fit[0] + 1
	r.fit[1] = timingForget*r.fit[1] + float64(k)
	r.fit[2] = timingForget*r.fit[2] + float64(k*k)
	r.fit[3] = timingForget*r.fit[3] + (s.start + tau)
	r.fit[4] = timingForget*r.fit[4] + float64(k)*(s.start+tau)
	if det := r.fit[0]*r.fit[2] - r.fit[1]*r.fit[1]; r.fit[0] > 3 && det > 1e-9 {
		b := (r.fit[0]*r.fit[4] - r.fit[1]*r.fit[3]) / det
		if math.Abs(b-slotSamplesN) < 2 {
			r.period = b
			r.t0 = (r.fit[3] - b*r.fit[1]) / r.fit[0]
		}
	} else {
		r.t0 += tau * 0.5
	}
	s.start = r.slotTime(k)
}

// syncCorr correlates r.corrX, the SYNC field samples of energy e, with the
// waveform of pattern p.
func (r *Receiver) syncCorr(p enums.SyncPattern, e float64) anchorInfo {
	if e == 0 {
		return anchorInfo{pattern: p}
	}
	tpl := r.search.conjOf(p)
	var c complex128
	for j, v := range r.corrX {
		c += v * tpl[j]
	}
	a := math.Hypot(real(c), imag(c))
	span := float64(r.search.span)
	amp := a / span
	noise := math.Max(e/span-amp*amp, 1e-12)
	rot := complex64(complex(real(c)/a, -imag(c)/a))
	return anchorInfo{pattern: p, metric: a * a / (e * span), rot: rot, amp: amp, noise: noise}
}

// rotAt returns the rotation that removes the residual carrier at channel
// sample t.
func (r *Receiver) rotAt(t float64) complex128 {
	p := -2 * math.Pi * r.freq * t / basebandRate
	return complex(math.Cos(p), math.Sin(p))
}

// slotEnergy returns the mean power over the burst of slot s.
func (r *Receiver) slotEnergy(s *slotState) float64 {
	n := burstSymbols * samplesPerSy
	if !r.opt.Coherent {
		return r.chz.slotEnergyAt(r.symTime(s.index, cachSymbols), n)
	}
	ch := &r.chz.ch
	dec := int64(ch.dec)
	a := int64(r.symTime(s.index, cachSymbols))
	a = (a - (a%dec+dec)%dec) / dec
	n /= ch.dec
	var e float64
	for i := range int64(n) {
		v := ch.at(a + i)
		e += float64(real(v)*real(v) + imag(v)*imag(v))
	}
	return e / float64(n)
}

// detectUpTo detects the symbols of the slots from r.first to j and hands
// them to the tracker: up to j-1 when slot j is an anchor, whose second
// half the next gap detects, and up to j otherwise.
func (r *Receiver) detectUpTo(j int64) {
	if j < r.first {
		return
	}
	s := r.slot(j)
	last := j
	if s.index == j && s.anchor {
		last = j - 1
	}
	if r.opt.Coherent {
		r.coherentGap(r.first, j, false)
	}
	for k := r.first; k <= last; k++ {
		if !r.opt.Coherent {
			r.discDetect(r.slot(k))
		}
		if !r.warm {
			r.finish(r.slot(k))
		}
	}
	r.first = last + 1
}

// detectBurst detects and finishes slot s of a stream whose carrier is off
// between bursts.
func (r *Receiver) detectBurst(s *slotState) {
	if s.present {
		if !r.opt.Coherent {
			r.discDetect(s)
		} else {
			r.coherentBurst(s.index)
		}
		if !r.warm {
			r.finish(s)
		}
	}
	r.first = s.index + 1
}

// finish turns the detected symbols of slot s into a received burst for
// the tracker.
func (r *Receiver) finish(s *slotState) {
	if !s.present {
		return
	}
	if r.opt.Coherent {
		for _, h := range r.haveOf(s)[cachSymbols:] {
			if h == noRun {
				return
			}
		}
	}
	var rec slotRec
	rec.index = s.index
	rec.bs = r.bs
	rec.src = sourceAt(r.src)
	rec.stream = r.streams
	rec.seconds = (r.symTime(s.index, cachSymbols) - float64(r.chz.delayCh)) / basebandRate
	r.fill264(s, &rec)
	if s.anchor {
		rec.anchor = s.a.pattern
	}
	r.tk.add(&rec, r.emit)
}

// fill264 sets the hard bits and log-likelihood ratios of rec, burst and
// CACH, from the symbol APPs of slot s.
func (r *Receiver) fill264(s *slotState, rec *slotRec) {
	var bits [layer2.BurstBits]bit.Bit
	app := r.appOf(s)
	for k := range slotSymbols {
		best := 0
		a := app[4*k : 4*k+4]
		for v := 1; v < 4; v++ {
			if a[v] > a[best] {
				best = v
			}
		}
		b0, b1 := dibitOfDigit(best)
		l0, l1 := dibitLLR(a)
		if k < cachSymbols {
			rec.cach[2*k], rec.cach[2*k+1] = b0, b1
			rec.cachLLR[2*k], rec.cachLLR[2*k+1] = l0, l1
		} else {
			i := 2 * (k - cachSymbols)
			bits[i], bits[i+1] = b0, b1
			rec.llr[i], rec.llr[i+1] = l0, l1
		}
	}
	rec.data = bit.Pack264(bits)
}

// dibitLLR returns the log-likelihood ratios of the two bits of a symbol
// from its four log-APPs (digits 0 to 3 for symbols -3, -1, +1, +3; dibits
// 11, 10, 00, 01), positive for bit 0, in units of 1/8, saturated.
func dibitLLR(a []float32) (int16, int16) {
	b0 := max(a[2], a[3]) - max(a[0], a[1])
	b1 := max(a[1], a[2]) - max(a[0], a[3])
	return satLLR(b0), satLLR(b1)
}

// satLLR returns v in units of 1/8, saturated to the int16 range.
func satLLR(v float32) int16 {
	v *= 8
	if v > 32000 {
		return 32000
	}
	if v < -32000 {
		return -32000
	}
	return int16(v)
}

// repairEntry repairs the FEC fields of entry e and, for a voice burst whose
// slot the ring still holds, detects the burst again with the repaired bits
// as priors, then repairs it again with the new log-likelihood ratios.
func (r *Receiver) repairEntry(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	r.rep.repair(e, cc, ccOK)
	j := e.rec.index
	s := r.slot(j)
	if r.opt.RefinePrior == 0 || !r.opt.Coherent || s.index != j || !r.rep.voiceBits(e, &r.known) {
		return
	}
	bits := bit.Unpack264(e.out.Fixed)
	clear(r.prior[:])
	for k := range burstSymbols {
		lp := r.prior[4*(cachSymbols+k) : 4*(cachSymbols+k)+4]
		for b := range 2 {
			i := 2*k + b
			if !r.known[i] {
				continue
			}
			for d := range 4 {
				b0, b1 := dibitOfDigit(d)
				v := b0
				if b == 1 {
					v = b1
				}
				if v == bits[i] {
					lp[d] += r.opt.RefinePrior
				}
			}
		}
	}
	s.hasPrior = true
	if r.bs {
		la, ra := j, j
		for la > j-maxGapSlots/2 && (r.slot(la).index != la || !r.slot(la).anchor || la >= j) {
			la--
		}
		for ra < j+maxGapSlots/2 && (r.slot(ra).index != ra || !r.slot(ra).anchor || ra <= j) {
			ra++
		}
		if r.slot(la).index != la || r.slot(ra).index != ra {
			s.hasPrior = false
			return
		}
		r.coherentGap(la, ra, false)
	} else {
		r.coherentBurst(j)
	}
	s.hasPrior = false
	rec := &r.scratchRec
	r.fill264(s, rec)
	e.rec.llr, e.rec.cachLLR = rec.llr, rec.cachLLR
	elc := e.out.Fixed.EmbeddedData()
	e.out.Data, e.out.Fixed, e.out.FixedCACH = rec.data, rec.data, rec.cach
	if e.elcOK {
		e.out.Fixed.SetEmbeddedData(elc)
	}
	r.rep.repair(e, cc, ccOK)
}

// detect runs the trellis of the receiver, exact-phase or per-survivor.
func (r *Receiver) detect(x []complex64, steps int, rot complex64, scale, mu float32, app []float32) {
	if r.xt != nil {
		r.xt.run(x, steps, rot, scale, anchorKappa, app)
		return
	}
	r.tr.run(x, steps, rot, scale, mu, app)
}

// detBest returns the best path metric of the last detect run.
func (r *Receiver) detBest(steps int) float32 {
	if r.xt != nil {
		return r.xt.best(steps)
	}
	return r.tr.best(steps)
}
