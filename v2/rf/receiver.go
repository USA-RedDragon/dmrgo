package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Tier selects how a Receiver detects symbols, by compute budget.
type Tier int

// The tiers, from the least compute to the most. The README gives the
// sensitivity, compute and RAM of each; Tier0 to Tier2 are its Pareto front
// of compute, RAM and sensitivity, and every tier decodes each FEC field
// with soft decisions.
const (
	// TierDefault is the zero value: Tier0 in a Config, and in a DiscConfig
	// Tier1 for NewDisc, or Tier0 under TinyGo and for Init.
	TierDefault Tier = iota
	// Tier0 slices the output of an FM discriminator and its matched
	// filter. On I/Q an FM discriminator at 12 kHz comes before it, or at
	// BasebandRate/2 the complex front end. It fits a microcontroller.
	Tier0
	// Tier1 adds to Tier0 the soft decoders of the coherent tiers: a
	// BPTC(196,96) block whose check fails by ordered statistics, C0 and C1
	// of each voice frame by soft ML, the embedded LC of each superframe
	// over its four fragments and the earlier superframes of the call, and
	// LookbackSlots slots before the first lock decoded again. It detects
	// the bursts that they decode, and each rate 3/4 block, from the
	// symbols of the discriminator with the correlation of their noise.
	// Each burst of a stream that is not clear it detects again as Tier2
	// does, on audio from the carrier phase that it sums from the
	// discriminator samples. It costs about the CPU of Tier0 on a clear
	// stream and of Tier2 on a weak one.
	Tier1
	// Tier2 adds to the soft decoders of Tier1, on I/Q at BasebandRate, the
	// detection again of every burst with a trellis over three symbols that
	// carries the carrier phase with each survivor, from the SYNC or EMB of
	// the burst and from the SYNC of the slots beside it, and the coherent
	// test of the SYNC candidates of the search. It holds each burst one
	// slot longer, and runs on a PC.
	Tier2
	// Tier3 detects over a window of three symbols with one of 52 carrier
	// phases in the trellis state, and detects each voice burst again with
	// the bits that its FEC fixed, at about 20 times the compute of Tier2.
	// It is for offline decoding on a PC.
	Tier3
	// Tier4 is Tier3 over a window of five symbols with 200 carrier
	// phases, the exact count for h = 0.27, at about 50 times the compute
	// of Tier3. It is for offline decoding on a PC and needs about 100 MB.
	Tier4
)

// Parameters of the DMR 4FSK signal (ETSI TS 102 361-1 clause 10.2).
const (
	symbolRate   = 4800
	deviationHz  = 648.0
	basebandRate = 48000
	samplesPerSy = basebandRate / symbolRate
	rrcAlpha     = 0.2
	rrcSpan      = 8
	burstSymbols = 132
	cachSymbols  = 12
	slotSeconds  = 0.030
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
	// the engine without the trellis locks.
	discLock = 0.9
	// anchorMetric is the share of the expected SYNC metric at which a slot
	// counts as an anchor.
	anchorMetric = 0.45
	// levelGain is the weight of each anchor in the level and noise
	// estimates, high enough to follow a receiver gain that settles, and
	// levelJump the ratio of the power of an anchor to the level at which
	// the estimates start again from the anchor.
	levelGain = 0.5
	levelJump = 4.0
	// msQuietSlots is the number of slots in a row without carrier that
	// end the stream of an MS, which sends in every other slot: a radio
	// that keys up again a moment later has a new timing.
	msQuietSlots = 6
	// msPresence is the ratio of the mean power of a slot of an MS to that
	// of the slot before it, which it does not send in, at which the
	// carrier counts as on.
	msPresence = 4.0
	// presenceFactor is the ratio of the mean power of a slot to the noise
	// power at which the carrier counts as on.
	presenceFactor = 1.5
	// bsPresence is the share of the carrier power above the noise power
	// at which a slot of a BS stream counts as on. The carrier of a BS
	// stream is on in every slot, and below 0 dB Eb/N0 the test of
	// presenceFactor dropped most of its bursts. A slot counts as on when
	// it passes either test: r.amp follows only the anchors, and a burst
	// in a fade far below that level but well above the noise is on.
	bsPresence = 0.5
	// lockSpanHz is the carrier error, in Hz, that the receiver searches
	// at lock: the spectrum estimate is no closer than that.
	lockSpanHz = 300.0
	// wideChunk is the number of baseband samples that the receiver filters
	// before it searches or follows the stream again.
	wideChunk = 128
	// trellisWin is the number of symbols of a block of the trellis of the
	// coherent tiers, and trellisLook the symbols of lookahead of its
	// backward pass. Eight symbols of lookahead give the same sweep as
	// whole runs.
	trellisWin  = 64
	trellisLook = 8
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
	// timingSigma is the standard deviation, in samples, of the timing
	// of one anchor that the timing fit assumes, and periodSigma that of
	// the slot period of a stream about slotSamplesN, about 200 ppm: the
	// fit weighs the period of its anchors against slotSamplesN with
	// them, so that two anchors close together do not set a period that
	// misses the next anchor.
	timingSigma = 0.5
	periodSigma = 0.3
	// timingPrior is the weight of slotSamplesN in the period of the fit,
	// in anchors one slot from the mean slot of the fit.
	timingPrior = timingSigma * timingSigma / (periodSigma * periodSigma)
	// maxTauSpan is the most samples that measure searches the SYNC of
	// a slot around where the timing fit puts it.
	maxTauSpan = 5.0
)

// BasebandRate is the sample rate of the complex baseband that
// WriteBaseband takes by default, in Hz.
const BasebandRate = basebandRate

// Config sizes a Receiver. New allocates every buffer from it, and the
// Receiver allocates nothing after that.
type Config struct {
	// Rate is the sample rate of the I/Q bytes of Write, in Hz, at least
	// BasebandHz. 0 means BasebandHz. WriteBaseband does not use it.
	Rate float64
	// Signed selects signed 8-bit I/Q (HackRF) for Write instead of
	// unsigned (rtl_sdr).
	Signed bool
	// Tier selects the detection. The zero value TierDefault is Tier0.
	Tier Tier
	// BasebandHz is the sample rate of the complex baseband that
	// WriteBaseband takes and that the front end of Write makes: 0 or
	// BasebandRate, or BasebandRate/2 with Tier0 or Tier1, which selects
	// their receiver with a complex front end in place of the FM
	// discriminator, in about 1 KB of RAM for Tier0. 0 means BasebandRate.
	BasebandHz int
	// MaxWrite is the largest number of bytes in one Write. 0 means 1 << 16.
	MaxWrite int
	// LookbackSlots is the number of 30 ms slots before the first SYNC it
	// locks to that the receiver keeps and decodes: 0 means 8 for Tier1
	// and Tier2, and 16 for Tier3 and Tier4. Tier0 keeps none, and loses
	// the start of a stream that it enters late.
	LookbackSlots int
	// OnBurst, when not nil, takes each burst as the receiver recovers it,
	// in place of the slice that Write, WriteBaseband and Flush return,
	// which are then empty. The receiver then keeps no buffer of bursts.
	// The burst is valid only during the call; copy it to keep it. A
	// pointer and not a value, because TinyGo passes a struct value one
	// element per stack slot.
	OnBurst func(*Burst)
	// opt, when not nil, replaces the settings of the tier.
	opt *options
}

// ConfigError reports a Config that New cannot use.
type ConfigError struct {
	// Field is the name of the Config field, such as "Rate".
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "rf: Config." + e.Field + " is out of range"
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
	// log priors of engine.prior.
	hasPrior bool
}

// Receiver recovers DMR bursts from a stream of I/Q samples. It locks to
// the slot timing of the first SYNC it finds, follows the timing and the
// carrier through the transmission, and returns every burst it recovers.
type Receiver struct {
	e    *engine
	lean *leanRx
	iqd  *iqRx
}

// engine is the receiver of every tier. It keeps the samples of a number
// of slots and detects the slots between the anchors of a stream.
type engine struct {
	cfg  Config
	opt  options
	fe   *frontEnd
	wide []complex64
	// inChunk is the most input bytes whose baseband samples fit in wide.
	inChunk int
	carry   byteCarry
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
	// scr holds the ring samples that fillRange interpolates from.
	scr []complex64
	// runG, runDir, runSteps, runLo and runHi describe the trellis run
	// that runSamples, runPrior and runKeep serve.
	runG, runLo, runHi symRef
	runDir, runSteps   int
	tk                 *tracker
	out                []Burst
	pulse              phasePulse
	corrX              []complex64

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
	offEnergy float64
	// quiet counts the slots in a row without carrier.
	quiet     int
	devScale  float64
	dcHz      float64
	discNoise float64
	next      int64
	lost      int

	slots    []slotState
	first    int64
	lastAnch int64
	// doneAt is the channel sample where the last stream ended, before
	// which the search does not go again, nor the lookback of a lock.
	doneAt int64
	// scanAt is the next channel sample that scanStale tries.
	scanAt int64
	// finRec holds the record of the slot that finish hands the tracker,
	// which TinyGo puts on the heap for every slot as a local, and emitFn
	// is emit as a func value made once.
	finRec slotRec
	emitFn func(*received)
	// llrBuf holds the ratios of a burst that fill264 sets.
	llrBuf [layer2.BurstBits]int16
	// syms holds the detection of the slots on their way to the tracker,
	// as many as the gap that the first detection after a lock spans, and
	// refined that of the slots that a refinement detects again, from
	// refLo, while refining.
	syms     symStore
	refined  symStore
	refining bool
	refLo    int64
	// prior holds the log priors of the digits of each symbol of the slot
	// that a refinement detects again, from its decoded FEC.
	prior []float32

	streams int
	rep     *repairer
	// scratchRec holds a slot record while repairEntry detects a voice
	// burst again, and known the burst bits it fixed, for the tiers that
	// refine.
	scratchRec *slotRec
	known      *[layer2.BurstBits]bool
	// data holds the ratios of repairData, for the tiers that detect a
	// data burst again.
	data *dataPass
	// freqTot holds the anchor scores of the carrier offsets of bestFreq.
	freqTot []float64
	h       float64
	// llrScale is options.LLRScale, 1 for 0.
	llrScale float32
	// offsetHz and devHz are the carrier offset and the outer deviation
	// of the last locked stream.
	offsetHz float64
	devHz    float64
	// cb holds the burst that emit hands OnBurst.
	cb Burst
}

// New returns a Receiver sized by cfg. It allocates every buffer of the
// Receiver. It returns a *ConfigError when a field of cfg is out of range.
func New(cfg Config) (*Receiver, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = Tier0
	}
	if cfg.Tier < Tier0 || cfg.Tier > Tier4 {
		return nil, &ConfigError{Field: "Tier"}
	}
	switch {
	case cfg.BasebandHz == 0:
		cfg.BasebandHz = basebandRate
	case cfg.BasebandHz == discRate && cfg.Tier <= Tier1, cfg.BasebandHz == basebandRate:
	default:
		return nil, &ConfigError{Field: "BasebandHz"}
	}
	switch {
	case cfg.Rate == 0:
		cfg.Rate = float64(cfg.BasebandHz)
	case !(cfg.Rate >= float64(cfg.BasebandHz)) || cfg.Rate > 1e9:
		return nil, &ConfigError{Field: "Rate"}
	}
	if cfg.MaxWrite < 0 {
		return nil, &ConfigError{Field: "MaxWrite"}
	}
	if cfg.LookbackSlots < 0 || cfg.Tier == Tier0 && cfg.LookbackSlots > 0 {
		return nil, &ConfigError{Field: "LookbackSlots"}
	}
	if cfg.MaxWrite == 0 {
		cfg.MaxWrite = 1 << 16
	}
	opt := tierOptions(cfg.Tier)
	if cfg.Tier == Tier1 || cfg.Tier == Tier2 {
		opt.ExtLookback = cfg.LookbackSlots
		if opt.ExtLookback == 0 {
			opt.ExtLookback = 8
		}
	}
	if cfg.opt != nil {
		opt = *cfg.opt
	}
	if !opt.tabled() {
		return nil, &ConfigError{Field: "opt"}
	}
	if !opt.Coherent {
		if opt.IQDisc && cfg.BasebandHz == basebandRate {
			return &Receiver{iqd: newIQRx(cfg, opt)}, nil
		}
		return &Receiver{lean: newLean(cfg, opt)}, nil
	}
	if cfg.LookbackSlots == 0 {
		cfg.LookbackSlots = 16
	}
	return &Receiver{e: newEngine(cfg, opt)}, nil
}

// newEngine returns the receiver of cfg with the settings opt.
func newEngine(cfg Config, opt options) *engine {
	r := &engine{cfg: cfg, opt: opt, candAt: -1, h: 0.27, doneAt: math.MinInt64, llrScale: opt.LLRScale}
	if r.llrScale == 0 {
		r.llrScale = 1
	}
	r.nsrc = r.opt.Sources
	half := cfg.BasebandHz == discRate
	r.fe = newFrontEnd(cfg.Rate, cfg.Signed, float64(cfg.BasebandHz))
	r.wide = make([]complex64, wideChunk)
	r.inChunk = max(2*int(float64(wideChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), 2)
	size := (cfg.LookbackSlots + maxGapSlots + 4) * slotSamplesN
	dec := 2
	if r.opt.Coherent {
		dec = max(r.opt.ChanDecimation, 1)
	}
	r.chz = newChannelizer(size, !r.opt.Coherent, dec, r.opt.CompactChannel, half)
	r.spec = newSpectrum(float64(cfg.BasebandHz))
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
			r.buf = make([]complex64, r.maxRun*r.opt.TrellisSPS)
			r.tapp = make([]float32, 4*r.maxRun)
		} else {
			r.tr = newTrellis(r.model, r.maxRun, trellisWin, trellisLook)
			r.slp = r.tr.slp
			r.buf = make([]complex64, syncSymbols*r.opt.TrellisSPS)
		}
	}
	if r.opt.Coherent {
		r.scr = make([]complex64, (fillChunk+1)*samplesPerSy/dec+2*sincTaps+4)
		r.freqTot = make([]float64, 2*20+1)
		r.pulse = newPhasePulse(samplesPerSy / dec)
		r.corrX = make([]complex64, syncSpan/dec)
	}
	r.slots = make([]slotState, cfg.LookbackSlots+maxGapSlots+4)
	if r.opt.Coherent {
		r.syms = newSymStore(max(cfg.LookbackSlots, maxGapSlots)+2, r.llrScale)
		if r.opt.RefinePrior > 0 || r.opt.DataPasses > 0 {
			r.refined = newSymStore(maxGapSlots+1, r.llrScale)
			r.prior = make([]float32, slotSymbols*4)
			r.scratchRec, r.known = new(slotRec), new([layer2.BurstBits]bool)
		}
		if r.opt.DataPasses > 0 {
			r.data = new(dataPass)
		}
	} else {
		r.syms = newSymStore(1, 1)
	}
	r.tk = newTracker()
	r.emitFn = r.emit
	if r.opt.SoftFEC {
		r.rep = newRepairer(r.opt.BPTCOSD, r.opt.GridBPTC)
		if r.rep != nil {
			r.rep.retryUnchecked = r.opt.DataUnchecked
		}
		r.tk.repair = r.repairEntry
		r.tk.elc = r.rep.embeddedLC
	}
	if cfg.OnBurst == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+2*len(r.slots)+16)
	}
	return r
}

// OffsetHz returns the carrier offset of the last stream that ended, in Hz.
func (r *Receiver) OffsetHz() float64 {
	if r.iqd != nil {
		return float64(r.iqd.conv.hz) + r.iqd.d.OffsetHz()/iqUnitsPerHz
	}
	if r.lean != nil {
		return float64(r.lean.offsetHz)
	}
	return r.e.offsetHz
}

// DeviationHz returns the outer deviation of the last locked stream, in Hz.
func (r *Receiver) DeviationHz() float64 {
	if r.iqd != nil {
		return r.iqd.d.DeviationHz() / iqUnitsPerHz
	}
	if r.lean != nil {
		return float64(r.lean.devHz)
	}
	return r.e.devHz
}

// Write processes interleaved 8-bit I/Q at Config.Rate and returns the
// bursts that are complete. The returned slice is valid until the next
// call.
func (r *Receiver) Write(iq []byte) []Burst {
	if r.iqd != nil {
		return r.iqd.write(iq)
	}
	if r.lean != nil {
		return r.lean.write(iq)
	}
	return r.e.write(iq)
}

// WriteBaseband processes complex baseband samples at Config.BasebandHz, as
// a radio front end delivers them, in place of Write. The returned slice is
// valid until the next call.
func (r *Receiver) WriteBaseband(x []complex64) []Burst {
	if r.iqd != nil {
		return r.iqd.writeBaseband(x)
	}
	if r.lean != nil {
		return r.lean.writeBaseband(x)
	}
	return r.e.writeBaseband(x)
}

// Flush finishes the stream at its end and returns the remaining bursts.
func (r *Receiver) Flush() []Burst {
	if r.iqd != nil {
		return r.iqd.flush()
	}
	if r.lean != nil {
		return r.lean.flush()
	}
	return r.e.flush()
}

// write processes the I/Q bytes of Receiver.Write.
func (r *engine) write(iq []byte) []Burst {
	r.out = r.out[:0]
	pair, iq := r.carry.split(iq)
	r.bytes(pair)
	r.bytes(iq)
	return r.out
}

// bytes processes whole I/Q pairs of bytes.
func (r *engine) bytes(iq []byte) {
	for len(iq) > 0 {
		n := min(len(iq), r.inChunk) &^ 1
		if n == 0 {
			break
		}
		w := r.fe.process(iq[:n], r.wide)
		iq = iq[n:]
		r.push(r.wide[:w])
	}
}

// writeBaseband processes the samples of Receiver.WriteBaseband.
func (r *engine) writeBaseband(x []complex64) []Burst {
	r.out = r.out[:0]
	for len(x) > 0 {
		n := min(len(x), len(r.wide))
		r.push(x[:n])
		x = x[n:]
	}
	return r.out
}

// push runs wide baseband samples through the spectrum estimate, which
// only an unlocked receiver uses and so only it runs, and through the
// channel filters, then searches or follows the stream.
func (r *engine) push(x []complex64) {
	for _, v := range x {
		if !r.locked && r.spec.push(v) && r.spec.OK && math.Abs(r.spec.Hz-r.chz.ncoHz) > 50 {
			r.chz.tune(r.spec.Hz)
			r.search.xOK = false
			r.searchAt, r.candAt = 0, -1
		}
		r.chz.push(v)
	}
	r.step(false)
}

// flush finishes the stream at its end and returns the remaining bursts.
func (r *engine) flush() []Burst {
	r.out = r.out[:0]
	for range slotSamplesN {
		r.chz.push(0)
	}
	r.step(true)
	for r.locked {
		r.finishStream()
		r.step(true)
	}
	r.tk.flush(r.emitFn)
	return r.out
}

// emit hands burst x to OnBurst, or appends it to the bursts that Write
// returns.
func (r *engine) emit(x *received) {
	if r.cfg.OnBurst != nil {
		x.put(&r.cb)
		r.cfg.OnBurst(&r.cb)
		return
	}
	if len(r.out) < cap(r.out) {
		r.out = r.out[:len(r.out)+1]
		x.put(&r.out[len(r.out)-1])
	}
}

// step searches or follows the stream over the samples that arrived.
func (r *engine) step(end bool) {
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
// The discriminator tier first waits for a spectrum estimate that finds a
// signal and tunes the channel to it: it never tunes while locked, and a
// carrier offset through its prefilter cost it 1 dB or more.
func (r *engine) searchStep() bool {
	ch := &r.chz.ch
	if from := max(r.chz.oldest()+slotSamplesN, r.doneAt); r.searchAt < from {
		r.searchAt = from
	}
	if r.opt.Coherent {
		r.searchAt = r.alignSearch(r.searchAt)
	}
	for {
		if !r.opt.Coherent {
			if !r.spec.OK {
				return false
			}
			base, u := r.chz.symAt(r.searchAt)
			if u != 0 {
				if b, v := r.chz.symAt(r.searchAt + 1); v == 0 {
					r.searchAt, base, u = r.searchAt+1, b, v
				}
			}
			step := r.chz.symStep()
			if base+int64(syncSymbols*step)+2 >= r.chz.sym.n {
				return false
			}
			rho2, src, data := r.search.discAt(&r.chz.sym, base, u, step, r.nsrc)
			if rho2 >= discLock*discLock {
				best := r.searchAt
				for k := int64(1); k < 6; k++ {
					b, v := r.chz.symAt(r.searchAt + k)
					if q, s2, d2 := r.search.discAt(&r.chz.sym, b, v, step, r.nsrc); q > rho2 {
						rho2, src, data, best = q, s2, d2, r.searchAt+k
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
func (r *engine) alignSearch(t int64) int64 {
	d := int64(r.search.dec)
	return t + (d-(t%d+d)%d)%d
}

// lock starts following a stream whose SYNC starts (center of its first
// symbol) at channel sample t.
func (r *engine) lock(t float64, src int, data bool, hz float64) {
	r.locked, r.scanAt = true, 0
	r.spec.reset()
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
	r.amp, r.noise, r.offEnergy, r.quiet = 0, 0, 0, 0
	r.devScale = 1
	r.dcHz = 0
	r.lost = 0
	back := int64(0)
	for back < int64(r.cfg.LookbackSlots) && r.slotTime(-back-1)-2*slotSamplesN > float64(r.chz.oldest()) && r.slotTime(-back-1) >= float64(r.doneAt) {
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
		bestM, bestF, bestTau := 0.0, 0.0, 0.0
		for tau := -2.0; tau <= 2; tau += 0.25 {
			r.freq = 0
			e := r.syncSamples(s, tau)
			t0 := r.syncStart(s, tau)
			for f := -lockSpanHz; f <= lockSpanHz; f += 5 {
				if m := r.syncMetricAt(p, e, t0, f); m > bestM {
					bestM, bestF, bestTau = m, f, tau
				}
			}
		}
		r.freq, r.lockFreq = bestF, bestF
		best := r.syncCorr(p, r.syncSamples(s, bestTau))
		r.t0 += bestTau
		r.amp, r.noise = best.amp, best.noise
	}
}

// warmFreq sets the carrier estimate and the modulation index from the
// slots from 0 to j at the timing of the slot fit: the values that
// maximize the sum of their squared SYNC correlations, searched in turn.
// The discriminator detection does not use the carrier estimate, so for it
// warmFreq only reports the deviation.
func (r *engine) warmFreq(j int64) {
	if !r.opt.Coherent {
		r.devHz = 3 * deviationHz * r.devScale
		return
	}
	r.searchFreq(j, r.lockFreq, lockSpanHz/2, 0.5)
	if !r.opt.TrackH {
		r.fw = 1 / (1 - freqForget)
		r.devHz = 3 * deviationHz * r.h / 0.27
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
// A wide search first takes steps 20 times as large, then searches one of
// them around the best.
func (r *engine) searchFreq(j int64, center, span, step float64) {
	if span > 20*step {
		coarse := 20 * step
		center, span = r.bestFreq(j, center, span, coarse), coarse
	}
	r.freq = r.bestFreq(j, center, span, step)
}

// bestFreq returns the frequency, from center-span to center+span in steps
// of step, with the best sum over the slots from 0 to j of the better SYNC
// metric of the voice and data patterns, at the timing of the slot fit. It
// interpolates each slot once and removes each carrier offset from the
// interpolated samples.
func (r *engine) bestFreq(j int64, center, span, step float64) float64 {
	tot := r.freqTot[:int(math.Round(2*span/step))+1]
	clear(tot)
	saved := r.freq
	r.freq = 0
	voice, data, _ := sourceAt(r.src).SyncPatterns()
	for k := int64(0); k <= j; k++ {
		s := r.slot(k)
		if s.index != k {
			continue
		}
		e := r.syncSamples(s, 0)
		t0 := r.syncStart(s, 0)
		for i := range tot {
			f := center - span + float64(i)*step
			tot[i] += max(r.syncMetricAt(voice, e, t0, f), r.syncMetricAt(data, e, t0, f))
		}
	}
	r.freq = saved
	best := 0
	for i, v := range tot {
		if v > tot[best] {
			best = i
		}
	}
	return center - span + float64(best)*step
}

// anchorScore returns the sum over the slots from 0 to j of the better
// SYNC metric of the voice and data patterns, at the timing of the slot
// fit and the current carrier estimate. It does not use the anchors that
// the warm pass found, which a wrong lock frequency makes wrong.
func (r *engine) anchorScore(j int64) float64 {
	var tot float64
	voice, data, _ := sourceAt(r.src).SyncPatterns()
	for k := int64(0); k <= j; k++ {
		s := r.slot(k)
		if s.index != k {
			continue
		}
		e := r.syncSamples(s, 0)
		tot += max(r.syncCorr(voice, e).metric, r.syncCorr(data, e).metric)
	}
	return tot
}

// setH sets the modulation index of the SYNC waveforms and of the trellis.
func (r *engine) setH(h float64) {
	r.h = h
	r.search.setH(h, &r.pulse)
	if r.model != nil {
		r.model.setH(h)
	}
	r.devHz = 3 * deviationHz * h / 0.27
}

// syncSamples interpolates the SYNC field of slot s, shifted by tau
// samples, into r.corrX with the residual carrier removed, and returns its
// energy.
func (r *engine) syncSamples(s *slotState, tau float64) float64 {
	ch := &r.chz.ch
	dec := r.search.dec
	t0 := r.syncStart(s, tau)
	rot, step := complex64(r.rotAt(t0)), complex64(r.rotAt(float64(dec)))
	base, u := ch.ringAt(t0, 0)
	first := base - sincTaps
	scr := r.scr[:min(r.search.span+2*sincTaps+2, len(r.scr))]
	ch.gather(first, scr)
	u += sincTaps
	var e float32
	for j := range r.search.span {
		v := ch.interpIn(scr, u+float32(j)) * rot
		rot *= step
		r.corrX[j] = v
		e += real(v)*real(v) + imag(v)*imag(v)
	}
	return float64(e)
}

// syncStart returns the channel sample of the first sample that
// syncSamples interpolates.
func (r *engine) syncStart(s *slotState, tau float64) float64 {
	return r.symTime(s.index, syncFirstSym+syncEdge) + tau - float64(r.search.sps/2*r.search.dec)
}

// syncMetricAt returns the SYNC metric of pattern p, as syncCorr, over
// r.corrX, interpolated without the carrier from channel sample t0 and of
// energy e, with the carrier at offset f removed.
func (r *engine) syncMetricAt(p enums.SyncPattern, e, t0, f float64) float64 {
	if e == 0 {
		return 0
	}
	rot, step := turn(-f*t0/basebandRate), turn(-f*float64(r.search.dec)/basebandRate)
	tpl := r.search.conjOf(p)
	var c complex64
	for j, v := range r.corrX {
		c += v * rot * tpl[j]
		rot *= step
	}
	return float64(real(c)*real(c)+imag(c)*imag(c)) / (e * float64(r.search.span))
}

// slotTime returns the channel sample of the center of the first CACH
// symbol of slot j.
func (r *engine) slotTime(j int64) float64 {
	return r.t0 + float64(j)*r.period
}

// symTime returns the channel sample of the center of symbol k (0 to 143,
// CACH first) of slot j.
func (r *engine) symTime(j int64, k int) float64 {
	return r.slotTime(j) + float64(k)*r.period/slotSymbols
}

// slot returns the state of slot j in the ring of slot states.
func (r *engine) slot(j int64) *slotState {
	return &r.slots[r.slotPos(j)]
}

// resetSlot clears the state of slot j and returns it.
func (r *engine) resetSlot(j int64) *slotState {
	s := r.slot(j)
	*s = slotState{index: j, start: r.slotTime(j)}
	r.syms.clear(r.syms.base(j), slotSymbols, 0)
	r.syms.own[r.syms.base(j)/slotSymbols] = j
	return s
}

// slotPos returns the position of slot j in the ring of slot states.
func (r *engine) slotPos(j int64) int {
	return floorMod(j, len(r.slots))
}

// floorMod returns j modulo n, from 0 to n-1. Slot numbers count from the
// lock of a stream, so j fits 32 bits and a 32-bit CPU divides it as an
// int instead of in software.
func floorMod(j int64, n int) int {
	x := int(j) % n
	if x < 0 {
		x += n
	}
	return x
}

// symStore holds, for each symbol of a number of slots, the digit with the
// highest log-APP, the log-likelihood ratios of its two bits, and the
// distance from the start of the trellis run that set them.
type symStore struct {
	slots int
	digit []int8
	llr   []int16
	have  []int16
	// own holds the slot whose symbols each place holds.
	own []int64
	// scale multiplies the ratios that set stores.
	scale float32
}

// newSymStore returns a store of n slots whose ratios set scales by scale.
func newSymStore(n int, scale float32) symStore {
	return symStore{slots: n, digit: make([]int8, n*slotSymbols), llr: make([]int16, 2*n*slotSymbols), have: make([]int16, n*slotSymbols),
		own: make([]int64, n), scale: scale}
}

// base returns the store index of the first symbol of slot j, which shares
// its place with every slot a multiple of slots away.
func (st *symStore) base(j int64) int {
	return floorMod(j, st.slots) * slotSymbols
}

// clear resets n symbols from store index p, with run distance have.
func (st *symStore) clear(p, n int, have int16) {
	clear(st.digit[p : p+n])
	clear(st.llr[2*p : 2*(p+n)])
	for i := p; i < p+n; i++ {
		st.have[i] = have
	}
}

// set stores the four log-APPs a of the symbol at store index p.
func (st *symStore) set(p int, a []float32) {
	best := 0
	for v := 1; v < 4; v++ {
		if a[v] > a[best] {
			best = v
		}
	}
	st.digit[p] = int8(best)
	st.llr[2*p], st.llr[2*p+1] = dibitLLR(a, st.scale)
}

// symsOf returns the store that holds slot s and the store index of its
// first symbol: the refinement store while a refinement detects s again.
func (r *engine) symsOf(s *slotState) (*symStore, int) {
	if j := s.index; r.refining && j >= r.refLo && j < r.refLo+int64(r.refined.slots) {
		return &r.refined, int(j-r.refLo) * slotSymbols
	}
	return &r.syms, r.syms.base(s.index)
}

// followStep measures the next slot once its samples and a lookahead have
// arrived, and detects the slots between anchors. It reports whether it
// made progress.
func (r *engine) followStep(end bool) bool {
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
	if r.symTime(j, syncFirstSym) >= float64(r.chz.delayCh) {
		r.measure(s)
	}
	switch {
	case j < 0:
	case s.present && (s.anchor || r.lastAnch != math.MinInt64 && j-r.lastAnch < lostSlots):
		r.lost = 0
	default:
		r.lost++
	}
	if r.quiet++; s.present || j < 0 {
		r.quiet = 0
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
	if r.lost >= lostSlots || r.lastAnch == math.MinInt64 && j >= 6 || !r.bs && r.quiet >= msQuietSlots {
		r.finishStream()
		return true
	}
	if r.opt.Coherent && !r.warm && r.stale(j) && r.scanStale(j) {
		return true
	}
	if r.warm && j >= warmSlots {
		r.endWarm(j)
	}
	return true
}

// endWarm ends the warm pass at slot j: it sets the carrier estimate from
// the slots up to j and starts the detection again from the first slot.
// The timing fit starts again too, as the detection measures the anchors
// of the warm pass again, and the same anchor twice in the fit made a
// period from two anchors two slots apart.
func (r *engine) endWarm(j int64) {
	r.warmFreq(j)
	r.warm = false
	r.next, r.first = r.warmStart, r.warmStart
	r.lastAnch, r.lost = math.MinInt64, 0
	r.fit = [5]float64{}
}

// stale reports whether the stream may have moved by slot j: a BS stream
// without a SYNC for staleSlots, or an MS stream without one for more than
// a superframe, as a repeater or a radio that keys up again with a new
// timing sends, or one whose timing jumps.
func (r *engine) stale(j int64) bool {
	if r.lastAnch == math.MinInt64 || j < 0 {
		return false
	}
	if r.bs {
		return j-r.lastAnch > staleSlots
	}
	return j-r.lastAnch > 2*layer2.VoiceBurstsPerSuperframe+1
}

// scanStale searches the slots of a stale stream up to slot j for a SYNC
// off its slots, and when it finds one, ends the stream and moves the
// search there. It reports whether it ended the stream.
func (r *engine) scanStale(j int64) bool {
	ch := &r.chz.ch
	from := max(r.scanAt, int64(r.slotTime(r.lastAnch+1)), r.chz.oldest()+slotSamplesN)
	from = r.alignSearch(from)
	to := int64(r.slotTime(j + 1))
	var best syncHit
	at := int64(-1)
	t := from
	for ; t < to && t+syncSpan+3*samplesPerSy < r.chz.count(); t += searchStep {
		if at >= 0 && t >= at+slotSamplesN {
			break
		}
		if h := r.search.at(ch, t, r.nsrc); h.Metric >= lockMetric && h.Metric > best.Metric && r.offGrid(t) {
			best, at = h, t
		}
	}
	r.scanAt = t
	if at < 0 {
		return false
	}
	done := int64(r.slotTime(r.lastAnch + 1))
	r.finishStream()
	r.searchAt, r.doneAt, r.candAt = r.alignSearch(at-searchStep), done, -1
	return true
}

// offGrid reports whether a SYNC that the search finds at channel sample
// t lies off the slots of the stream by more than a symbol: for an MS,
// which sends in one slot of two, off the slots of its bursts.
func (r *engine) offGrid(t int64) bool {
	k := float64(t-syncEdge*samplesPerSy) - float64(syncFirstSym*samplesPerSy) - r.slotTime(r.lastAnch)
	n := r.period
	if !r.bs {
		n *= 2
	}
	k = math.Mod(k, n)
	if k < 0 {
		k += n
	}
	return k > samplesPerSy && k < n-samplesPerSy
}

// finishStream detects the slots that are still pending and unlocks.
func (r *engine) finishStream() {
	if r.warm {
		r.endWarm(r.next - 1)
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
	r.offsetHz = r.chz.ncoHz + r.freq
	if !r.opt.Coherent {
		r.offsetHz = r.chz.ncoHz + r.dcHz
	}
	r.locked = false
	r.searchAt = max(int64(r.slotTime(r.next)), r.lockAt+searchStep)
	r.doneAt = int64(r.slotTime(r.next))
	if r.opt.Coherent {
		r.searchAt = r.alignSearch(r.searchAt)
	}
	r.tk.flush(r.emitFn)
}

// measure classifies the SYNC field of slot s, measures its timing,
// carrier and level when it is an anchor, and tests the carrier.
func (r *engine) measure(s *slotState) {
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
	span := r.tauSpan(s.index)
	for tau := -span; tau <= span; tau += 0.25 {
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
	if r.bs {
		s.present = energy > min(presenceFactor*r.noise, r.noise+bsPresence*r.amp*r.amp)
	}
	if !r.bs && r.lastAnch != math.MinInt64 {
		// The slots between the bursts of an MS hold its noise, which a
		// burst stands above at any gain of the input.
		if (s.index-r.lastAnch)&1 != 0 {
			r.offEnergy = energy
		} else if r.offEnergy > 0 && energy > msPresence*r.offEnergy {
			s.present = true
		}
	}
	s.anchor = best.metric >= anchorMetric*expect && best.metric >= 4.0/float64(r.search.span)
	s.present = s.present || s.anchor
	if !s.anchor {
		return
	}
	s.a = best
	r.timing(s, bestTau)
	if g := (best.amp / r.amp) * (best.amp / r.amp); r.amp == 0 || g > levelJump || g < 1/levelJump {
		// The first anchor, or the gain of the input moved: the levels
		// start from the anchor.
		r.amp, r.noise = best.amp, best.noise
	} else {
		r.amp = (1-levelGain)*r.amp + levelGain*best.amp
		r.noise = (1-levelGain)*r.noise + levelGain*best.noise
	}
}

// tauSpan returns how far, in channel samples, measure searches the SYNC
// of slot j around where the timing fit puts it: three standard
// deviations of the prediction of the fit, up to the step of 0.25 samples
// of the search, at least 1.5 and at most maxTauSpan samples.
func (r *engine) tauSpan(j int64) float64 {
	f0, f1, f2 := r.fit[0], r.fit[1], r.fit[2]+timingPrior
	det := f0*f2 - f1*f1
	if f0 == 0 || det <= 1e-9 {
		return 1.5
	}
	k := float64(j)
	v := timingSigma * timingSigma * (f2 - 2*k*f1 + k*k*f0) / det
	return min(max(1.5, math.Ceil(4*3*math.Sqrt(v))/4), maxTauSpan)
}

// timing adds the SYNC of anchor slot s, tau samples from where the fit
// put it, to the fit of the slot timing: a line through the anchors, with
// old anchors forgotten and the period weighed against slotSamplesN by
// timingPrior. The first anchor moves the timing halfway to it.
func (r *engine) timing(s *slotState, tau float64) {
	k := s.index
	r.fit[0] = timingForget*r.fit[0] + 1
	r.fit[1] = timingForget*r.fit[1] + float64(k)
	r.fit[2] = timingForget*r.fit[2] + float64(k*k)
	r.fit[3] = timingForget*r.fit[3] + (s.start + tau)
	r.fit[4] = timingForget*r.fit[4] + float64(k)*(s.start+tau)
	s2, s4 := r.fit[2]+timingPrior, r.fit[4]+timingPrior*slotSamplesN
	if det := r.fit[0]*s2 - r.fit[1]*r.fit[1]; r.fit[0] > 1.5 && det > 1e-9 {
		b := (r.fit[0]*s4 - r.fit[1]*r.fit[3]) / det
		if math.Abs(b-slotSamplesN) < 2 {
			r.period = b
			r.t0 = (r.fit[3]*s2 - r.fit[1]*s4) / det
		}
	} else {
		r.t0 += tau * 0.5
	}
	s.start = r.slotTime(k)
}

// syncCorr correlates r.corrX, the SYNC field samples of energy e, with the
// waveform of pattern p.
func (r *engine) syncCorr(p enums.SyncPattern, e float64) anchorInfo {
	if e == 0 {
		return anchorInfo{pattern: p}
	}
	tpl := r.search.conjOf(p)
	var c complex64
	for j, v := range r.corrX {
		c += v * tpl[j]
	}
	a := float64(sqrtf(real(c)*real(c) + imag(c)*imag(c)))
	span := float64(r.search.span)
	amp := a / span
	noise := math.Max(e/span-amp*amp, 1e-12)
	rot := complex(real(c)/float32(a), -imag(c)/float32(a))
	return anchorInfo{pattern: p, metric: a * a / (e * span), rot: rot, amp: amp, noise: noise}
}

// rotAt returns the rotation that removes the residual carrier at channel
// sample t.
func (r *engine) rotAt(t float64) complex128 {
	return complex128(turn(-r.freq * t / basebandRate))
}

// turn returns e^(2*pi*j*c) in single precision, from the fraction of the
// cycles c.
func turn(c float64) complex64 {
	s, co := sincosf(float32(2 * math.Pi * (c - math.Round(c))))
	return complex(co, s)
}

// slotEnergy returns the mean power over the burst of slot s.
func (r *engine) slotEnergy(s *slotState) float64 {
	n := burstSymbols * samplesPerSy
	if !r.opt.Coherent {
		return r.chz.slotEnergyAt(r.symTime(s.index, cachSymbols), n)
	}
	ch := &r.chz.ch
	dec := int64(ch.dec)
	a := int64(r.symTime(s.index, cachSymbols))
	a = (a - (a%dec+dec)%dec) / dec
	n /= ch.dec
	var e float32
	for i := range int64(n) {
		v := ch.at(a + i)
		e += real(v)*real(v) + imag(v)*imag(v)
	}
	return float64(e) / float64(n)
}

// detectUpTo detects the symbols of the slots from r.first to j and hands
// them to the tracker: up to j-1 when slot j is an anchor, whose second
// half the next gap detects, and up to j otherwise.
func (r *engine) detectUpTo(j int64) {
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
func (r *engine) detectBurst(s *slotState) {
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
func (r *engine) finish(s *slotState) {
	if !s.present {
		return
	}
	if r.opt.Coherent {
		st, p := r.symsOf(s)
		for _, h := range st.have[p+cachSymbols : p+slotSymbols] {
			if h == noRun {
				return
			}
		}
	}
	rec := &r.finRec
	*rec = slotRec{index: s.index, bs: r.bs, src: sourceAt(r.src), stream: r.streams}
	rec.seconds = (r.symTime(s.index, cachSymbols) - float64(r.chz.delayCh)) / basebandRate
	r.fill264(s, rec)
	if s.anchor {
		rec.anchor = s.a.pattern
	}
	r.tk.add(rec, r.emitFn)
}

// fill264 sets the hard bits and log-likelihood ratios of rec, burst and
// CACH, from the detected symbols of slot s.
func (r *engine) fill264(s *slotState, rec *slotRec) {
	rec.data = layer2.RawBurst{}
	st, p := r.symsOf(s)
	for k := range slotSymbols {
		b0, b1 := dibitOfDigit(int(st.digit[p+k]))
		l0, l1 := st.llr[2*(p+k)], st.llr[2*(p+k)+1]
		if k < cachSymbols {
			rec.cach[2*k], rec.cach[2*k+1] = b0, b1
			rec.cachLLR[2*k], rec.cachLLR[2*k+1] = l0, l1
		} else {
			i := 2 * (k - cachSymbols)
			rec.data[i/8] |= byte(b0<<1|b1) << (6 - i%8)
			r.llrBuf[i], r.llrBuf[i+1] = l0, l1
		}
	}
	rec.llr.set(&r.llrBuf)
}

// dibitLLR returns the log-likelihood ratios of the two bits of a symbol
// from its four log-APPs (digits 0 to 3 for symbols -3, -1, +1, +3; dibits
// 11, 10, 00, 01), positive for bit 0, times scale, in units of 1/8,
// saturated.
func dibitLLR(a []float32, scale float32) (int16, int16) {
	b0 := max32(a[2], a[3]) - max32(a[0], a[1])
	b1 := max32(a[1], a[2]) - max32(a[0], a[3])
	return satLLR(scale * b0), satLLR(scale * b1)
}

// satLLR returns v in units of 1/8, saturated to the int16 range, and at
// least one unit from 0 with the sign of v, so that the decoders that read
// a hard decision from the sign keep it.
func satLLR(v float32) int16 {
	v = max32(min32(8*v, 32000), -32000)
	i := int16(v)
	if i == 0 {
		switch {
		case v > 0:
			return 1
		case v < 0:
			return -1
		}
	}
	return i
}

// refine starts a refinement of the slots from lo, which detects them into
// the refinement store, from their detection on the way to the tracker,
// and leaves that as it is.
func (r *engine) refine(lo int64) {
	r.refining, r.refLo = true, lo
	st := &r.syms
	for i := range r.refined.slots {
		j, p := lo+int64(i), i*slotSymbols
		if q := st.base(j); st.own[q/slotSymbols] == j {
			copy(r.refined.digit[p:p+slotSymbols], st.digit[q:q+slotSymbols])
			copy(r.refined.llr[2*p:2*(p+slotSymbols)], st.llr[2*q:2*(q+slotSymbols)])
			copy(r.refined.have[p:p+slotSymbols], st.have[q:q+slotSymbols])
		} else {
			r.refined.clear(p, slotSymbols, noRun)
		}
	}
}

// repairEntry repairs the FEC fields of entry e and, for a voice burst whose
// slot the ring still holds, detects the burst again with the repaired bits
// as priors, then repairs it again with the new log-likelihood ratios.
func (r *engine) repairEntry(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	r.rep.repair(e, cc, ccOK)
	j := e.rec.index
	s := r.slot(j)
	if r.data != nil && s.index == j && r.rep.bptcRetry {
		r.repairData(e, cc, ccOK)
		return
	}
	if r.opt.RefinePrior == 0 || !r.opt.Coherent || s.index != j || !r.rep.voiceBits(e, r.known) {
		return
	}
	fixed := &e.out.fixed
	clear(r.prior)
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
				if v == unpacked.Bit(fixed[i/8]>>(7-i%8)&1) {
					lp[d] += r.opt.RefinePrior
				}
			}
		}
	}
	if !r.redetect(j) {
		return
	}
	rec := r.scratchRec
	e.rec.llr, e.rec.cachLLR = rec.llr, rec.cachLLR
	elc := e.out.fixed.EmbeddedData()
	e.out.data, e.out.fixed, e.out.fixedCACH = rec.data, rec.data, rec.cach
	if e.elcOK {
		e.out.fixed.SetEmbeddedData(elc)
	}
	r.rep.repair(e, cc, ccOK)
}

// dataPass holds the ratios of repairData: the extrinsic ratios of the
// grid decode, in nats, the channel ratios and the ratios of a burst.
type dataPass struct {
	ext  [layer2.BurstBits]float32
	ch   [layer2.BurstBits]int16
	llr  [layer2.BurstBits]int16
	next [layer2.BurstBits]int16
}

// repairData detects the BPTC(196,96) data burst of entry e again with
// the extrinsic information of the grid decode of its LLRs as bit priors,
// weighed by options.DataPrior, and repairs it with the new channel
// information and the grid's, up to options.DataPasses times or until it
// passes its check. The other fields of the burst keep their first
// detection, which the priors of a wrong block would only make worse.
func (r *engine) repairData(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	j := e.rec.index
	w := r.opt.DataPrior
	dp := r.data
	e.rec.llr.get(&dp.ch)
	for range r.opt.DataPasses {
		r.rep.soft.bptcExtrinsic(&dp.ch, &dp.ext)
		clear(r.prior)
		for k := range burstSymbols {
			lp := r.prior[4*(cachSymbols+k) : 4*(cachSymbols+k)+4]
			for d := range 4 {
				b0, b1 := dibitOfDigit(d)
				lp[d] = bitPrior(w*dp.ext[2*k], b0) + bitPrior(w*dp.ext[2*k+1], b1)
			}
		}
		if !r.redetect(j) {
			return
		}
		r.scratchRec.llr.get(&dp.next)
		e.rec.llr.get(&dp.llr)
		bp := &softOSDCodes[softBPTCCode]
		for _, i := range bp.pos[:bp.n] {
			c := float32(dp.next[i])/8 - w*dp.ext[i]
			dp.ch[i] = satLLR(c)
			dp.llr[i] = satLLR(c + dp.ext[i])
		}
		e.rec.llr.set(&dp.llr)
		r.rep.repair(e, cc, ccOK)
		if !r.rep.bptcRetry {
			return
		}
	}
}

// bitPrior returns the log prior, in nats, of bit value b from the
// log-likelihood ratio l, positive for 0: l/2 for 0 and -l/2 for 1.
func bitPrior(l float32, b unpacked.Bit) float32 {
	if b == 0 {
		return l / 2
	}
	return -l / 2
}

// redetect detects slot j again with the priors in r.prior, between the
// anchors on either side of it in a BS stream, into r.scratchRec, and
// reports whether it could. The carrier estimate stays as it is, as the
// priors would bias it.
func (r *engine) redetect(j int64) bool {
	s := r.slot(j)
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
			return false
		}
		r.refine(la)
		r.coherentGap(la, ra, false)
	} else {
		r.refine(j)
		r.coherentBurst(j)
	}
	s.hasPrior = false
	r.fill264(s, r.scratchRec)
	r.refining = false
	return true
}
