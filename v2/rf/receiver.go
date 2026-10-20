package rf

import (
	"errors"
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

// ETSI TS 102 361-1 clause 10.2
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

const (
	slotSymbols    = 144
	slotSamplesN   = slotSymbols * samplesPerSy
	syncFirstSym   = cachSymbols + 54
	maxGapSlots    = 7
	exactGapSlots  = 3
	searchStep     = 2
	lockMetric     = 0.4
	discLock       = 0.9
	anchorMetric   = 0.45
	levelGain      = 0.5
	levelJump      = 4.0
	msQuietSlots   = 6
	msPresence     = 4.0
	presenceFactor = 1.5
	bsPresence     = 0.5
	lockSpanHz     = 300.0
	wideChunk      = 128
	trellisWin     = 64
	trellisLook    = 8
	warmSlots      = 6
	lostSlots      = 24
	timingForget   = 0.97
	freqForget     = 0.9
	timingSigma    = 0.5
	periodSigma    = 0.3
	timingPrior    = timingSigma * timingSigma / (periodSigma * periodSigma)
	maxTauSpan     = 5.0
)

// ErrWriteTooLong reports a call of Write or WriteBaseband, on a Receiver
// or DiscReceiver without OnBurst, that is too long for the burst buffer
// that MaxWrite sizes. The call processes none of its samples.
var ErrWriteTooLong = errors.New("rf: write longer than MaxWrite")

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
	// MaxWrite is the largest number of bytes in one Write, and twice the
	// largest number of samples in one WriteBaseband. 0 means 1 << 16.
	// When OnBurst is nil, a longer call returns ErrWriteTooLong.
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
	opt     *options
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

type anchorInfo struct {
	pattern enums.SyncPattern
	metric  float64
	rot     complex64
	amp     float64
	noise   float64
}

type slotState struct {
	index    int64
	start    float64
	present  bool
	anchor   bool
	a        anchorInfo
	hasPrior bool
}

// Receiver recovers DMR bursts from a stream of I/Q samples. It locks to
// the slot timing of the first SYNC it finds, follows the timing and the
// carrier through the transmission, and returns every burst it recovers.
type Receiver struct {
	e        *engine
	lean     *leanRx
	iqd      *iqRx
	iq0      *iq0Rx
	cb       *combRx
	iql      *iqLowRx
	slot     int
	maxWrite int
}

type engine struct {
	cfg                Config
	opt                options
	fe                 *frontEnd
	wide               []complex64
	inChunk            int
	carry              byteCarry
	chz                *channelizer
	spec               *spectrum
	search             *syncSearch
	model              *cpmModel
	tr                 *trellis
	xt                 *xtrellis
	maxRun             int
	lp                 []float32
	slp                []float32
	buf                []complex64
	tapp               []float32
	scr                []complex64
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
	quiet     int
	devScale  float64
	dcHz      float64
	discNoise float64
	next      int64
	lost      int

	slots    []slotState
	first    int64
	lastAnch int64
	doneAt   int64
	scanAt   int64
	finRec   slotRec
	emitFn   func(*received)
	llrBuf   [layer2.BurstBits]int16
	syms     symStore
	refined  symStore
	refining bool
	refLo    int64
	prior    []float32

	streams                      int
	rep                          *repairer
	scratchRec                   *slotRec
	known                        *[layer2.BurstBits]bool
	data                         *dataPass
	freqTot                      []float64
	h                            float64
	hg                           *hGate
	llrScale                     float32
	offsetHz                     float64
	devHz                        float64
	cb                           Burst
	side                         combSide
	scout, frozen, passSeg, pend bool
	passStart, passEnd           int64
	pendAt, pendDone             int64
	passRecs                     []passRec
}

// New returns a Receiver sized by cfg. It allocates every buffer of the
// Receiver. It returns a *ConfigError when a field of cfg is out of range.
func New(cfg Config) (*Receiver, error) {
	r, err := newReceiver(cfg)
	if err != nil {
		return nil, err
	}
	hz := cfg.BasebandHz
	if hz == 0 {
		hz = basebandRate
	}
	r.slot, r.maxWrite = hz*3/100, cfg.MaxWrite
	if r.maxWrite == 0 {
		r.maxWrite = 1 << 16
	}
	return r, nil
}

func newReceiver(cfg Config) (*Receiver, error) {
	low, err := checkConfig(&cfg)
	if err != nil {
		return nil, err
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
	if !opt.tabled() || opt.IQFront > 0 && opt.CohExt {
		return nil, &ConfigError{Field: "opt"}
	}
	if low && opt.IQLow > 0 {
		return &Receiver{iql: newIQLowRx(cfg, opt)}, nil
	}
	if low {
		return &Receiver{iq0: newIQ0Rx(cfg, opt)}, nil
	}
	if !opt.Coherent {
		if opt.IQDisc && cfg.BasebandHz == basebandRate {
			if opt.IQFront > 0 && !opt.SoftExt && cfg.Rate == float64(cfg.BasebandHz) {
				return newIQ0Receiver(cfg, opt), nil
			}
			return &Receiver{iqd: newIQRx(cfg, opt)}, nil
		}
		return &Receiver{lean: newLean(cfg, opt)}, nil
	}
	if cfg.LookbackSlots == 0 {
		cfg.LookbackSlots = 16
	}
	if opt.Comb {
		c := newComb(cfg, opt)
		return &Receiver{e: c.t3, cb: c}, nil
	}
	return &Receiver{e: newEngine(cfg, opt)}, nil
}

func checkConfig(cfg *Config) (low bool, err error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = Tier0
	}
	if cfg.Tier < Tier0 || cfg.Tier > Tier4 {
		return false, &ConfigError{Field: "Tier"}
	}
	low = cfg.opt != nil && cfg.opt.IQFront == 4 && cfg.opt.IQDisc && (cfg.BasebandHz == iq0LowHz || cfg.BasebandHz == discRate/2)
	switch {
	case cfg.BasebandHz == 0:
		cfg.BasebandHz = basebandRate
	case cfg.BasebandHz == discRate && cfg.Tier <= Tier1, cfg.BasebandHz == basebandRate, low:
	default:
		return false, &ConfigError{Field: "BasebandHz"}
	}
	switch {
	case cfg.Rate == 0:
		cfg.Rate = float64(cfg.BasebandHz)
	case !(cfg.Rate >= float64(cfg.BasebandHz)) || cfg.Rate > 1e9:
		return false, &ConfigError{Field: "Rate"}
	}
	if cfg.MaxWrite < 0 {
		return false, &ConfigError{Field: "MaxWrite"}
	}
	if cfg.LookbackSlots < 0 || cfg.Tier == Tier0 && cfg.LookbackSlots > 0 {
		return false, &ConfigError{Field: "LookbackSlots"}
	}
	if cfg.MaxWrite == 0 {
		cfg.MaxWrite = 1 << 16
	}
	return low, nil
}

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
	size := (cfg.LookbackSlots + opt.passSlots() + maxGapSlots + 4) * slotSamplesN
	dec := 2
	if r.opt.Coherent {
		dec = max(int(r.opt.ChanDecimation), 1)
	}
	r.chz = newChannelizer(size, !r.opt.Coherent, dec, r.opt.CompactChannel, half)
	if r.opt.Coherent && (r.opt.IQTaps > 0 || r.opt.IQKaiser > 0 || r.opt.IQCutHz > 0) {
		r.chz.setFilter(r.opt.IQTaps, r.opt.IQKaiser, r.opt.IQCutHz)
		if r.opt.IQCutAuto && len(r.chz.taps) == chanTaps {
			r.chz.alt = r.chz.taps
			r.chz.useAlt(false)
		}
	}
	r.spec = newSpectrum(float64(cfg.BasebandHz))
	r.search = newSyncSearch(r.opt.Coherent, dec)
	if r.opt.Coherent {
		d := int(r.opt.Window)
		r.model = newCPMModel(d, r.opt.TrellisSPS)
		r.maxRun = (maxGapSlots + 1) * slotSymbols
		if r.opt.ExactPhase {
			r.maxRun = exactGapSlots * slotSymbols
			np := r.opt.PhaseStates
			if np == 0 {
				np = phaseStates
			}
			r.xt = newXTrellis(r.model, np, r.maxRun)
			r.xt.logmap = r.opt.LogMAP
			if r.opt.DriftPenalty > 0 {
				r.xt.pen = r.opt.DriftPenalty
			}
			r.xt.jump, r.xt.fade, r.xt.below = r.opt.PhaseJump, r.opt.PhaseJumpFade, r.opt.PhaseJumpBelow
			if r.opt.DriftSteps > 0 {
				r.xt.drifts = int(r.opt.DriftSteps)
			}
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
	r.slots = make([]slotState, cfg.LookbackSlots+opt.passSlots()+maxGapSlots+4)
	if opt.passSlots() > 0 {
		r.passRecs = make([]passRec, len(r.slots))
		if opt.HGate > 0 && opt.Coherent {
			r.hg = newHGate(len(r.slots), r.search)
		}
	}
	if r.opt.Coherent {
		r.syms = newSymStore(max(cfg.LookbackSlots, maxGapSlots)+2, r.llrScale)
		r.syms.logmap = r.xt != nil && r.opt.LogMAP
		if r.opt.RefinePrior > 0 || r.opt.DataPasses > 0 {
			r.refined = newSymStore(maxGapSlots+1, r.llrScale)
			r.refined.logmap = r.xt != nil && r.opt.LogMAP
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
			r.rep.soft.setBytes(&r.opt)
		}
		r.tk.repair = r.repairEntry
		r.tk.elc = r.rep.embeddedLC
	}
	if cfg.OnBurst == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(cfg.BasebandHz*3/100*2)+2*len(r.slots)+16)
	}
	return r
}

// OffsetHz returns the carrier offset of the last stream that ended, in Hz.
func (r *Receiver) OffsetHz() float64 {
	if r.iql != nil {
		return float64(r.iql.hz) + r.iql.d.OffsetHz()/iqUnitsPerHz
	}
	if r.iq0 != nil {
		return float64(r.iq0.hz) + r.iq0.d.OffsetHz()/iqUnitsPerHz
	}
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
	if r.iql != nil {
		return r.iql.d.DeviationHz() / iqUnitsPerHz
	}
	if r.iq0 != nil {
		return r.iq0.d.DeviationHz() / iqUnitsPerHz
	}
	if r.iqd != nil {
		return r.iqd.d.DeviationHz() / iqUnitsPerHz
	}
	if r.lean != nil {
		return float64(r.lean.devHz)
	}
	return r.e.devHz
}

// MaxBursts returns the most bursts that one call can return for a call
// of samples I/Q samples. Write returns at most r.MaxBursts((len(iq)+1)/2)
// bursts, WriteBaseband at most r.MaxBursts(len(x)) and Flush at most
// r.MaxBursts(0), and all three return none when Config.OnBurst is set. It
// does not allocate.
func (r *Receiver) MaxBursts(samples int) int {
	c := cap(r.outBuf())
	if r.cb != nil || c == 0 {
		return c
	}
	held := 16
	if r.e != nil {
		held += 2 * len(r.e.slots)
	}
	return min(c, 4*(max(samples, 0)/r.slot+2)+held)
}

func (r *Receiver) outBuf() []Burst {
	switch {
	case r.cb != nil:
		return r.cb.out
	case r.iql != nil:
		return r.iql.d.out
	case r.iq0 != nil:
		return r.iq0.out
	case r.iqd != nil:
		return r.iqd.out
	case r.lean != nil:
		return r.lean.out
	default:
		return r.e.out
	}
}

// Write processes interleaved 8-bit I/Q at Config.Rate and returns the
// bursts that are complete. It returns at most r.MaxBursts((len(iq)+1)/2)
// bursts. The returned slice is valid until the next call. Without
// OnBurst, it returns ErrWriteTooLong and processes nothing when iq is
// longer than Config.MaxWrite.
func (r *Receiver) Write(iq []byte) ([]Burst, error) {
	if len(iq) > r.maxWrite && cap(r.outBuf()) > 0 {
		return nil, ErrWriteTooLong
	}
	return r.write(iq), nil
}

func (r *Receiver) write(iq []byte) []Burst {
	if r.cb != nil {
		return r.cb.write(iq)
	}
	if r.iql != nil {
		return nil
	}
	if r.iq0 != nil {
		return r.iq0.write(iq)
	}
	if r.iqd != nil {
		return r.iqd.write(iq)
	}
	if r.lean != nil {
		return r.lean.write(iq)
	}
	return r.e.write(iq)
}

// WriteBaseband processes complex baseband samples at Config.BasebandHz, as
// a radio front end delivers them, in place of Write. It returns at most
// r.MaxBursts(len(x)) bursts. The returned slice is valid until the next
// call. Without OnBurst, it returns ErrWriteTooLong and processes nothing
// when x has more than Config.MaxWrite/2 samples.
func (r *Receiver) WriteBaseband(x []complex64) ([]Burst, error) {
	if 2*len(x) > r.maxWrite && cap(r.outBuf()) > 0 {
		return nil, ErrWriteTooLong
	}
	return r.writeBaseband(x), nil
}

func (r *Receiver) writeBaseband(x []complex64) []Burst {
	if r.cb != nil {
		return r.cb.writeBaseband(x)
	}
	if r.iql != nil {
		return r.iql.writeBaseband(x)
	}
	if r.iq0 != nil {
		return r.iq0.writeBaseband(x)
	}
	if r.iqd != nil {
		return r.iqd.writeBaseband(x)
	}
	if r.lean != nil {
		return r.lean.writeBaseband(x)
	}
	return r.e.writeBaseband(x)
}

// Flush finishes the stream at its end and returns the remaining bursts.
// It returns at most r.MaxBursts(0) bursts.
func (r *Receiver) Flush() []Burst {
	if r.cb != nil {
		return r.cb.flush()
	}
	if r.iql != nil {
		return r.iql.flush()
	}
	if r.iq0 != nil {
		return r.iq0.flush()
	}
	if r.iqd != nil {
		return r.iqd.flush()
	}
	if r.lean != nil {
		return r.lean.flush()
	}
	return r.e.flush()
}

func (r *engine) write(iq []byte) []Burst {
	r.out = r.out[:0]
	pair, iq := r.carry.split(iq)
	r.bytes(pair)
	r.bytes(iq)
	return r.out
}

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

func (r *engine) writeBaseband(x []complex64) []Burst {
	r.out = r.out[:0]
	for len(x) > 0 {
		n := min(len(x), len(r.wide))
		r.push(x[:n])
		x = x[n:]
	}
	return r.out
}

func (r *engine) push(x []complex64) {
	for _, v := range x {
		if !r.locked && r.spec.push(v) && r.spec.OK {
			moved := r.chz.alt != nil && r.chz.useAlt(!r.tuneOK(r.spec.Hz))
			if math.Abs(r.spec.Hz-r.chz.ncoHz) > 50 && r.tuneOK(r.spec.Hz) {
				r.chz.tune(r.spec.Hz)
				moved = true
			}
			if moved {
				r.search.xOK = false
				r.searchAt, r.candAt = 0, -1
			}
		}
		r.chz.push(v)
	}
	r.step(false)
}

func (r *engine) tuneOK(hz float64) bool {
	return r.opt.IQTuneMaxHz <= 0 || math.Abs(hz) <= float64(r.opt.IQTuneMaxHz)
}

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

func (r *engine) emit(x *received) {
	r.side = x.side
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

func (r *engine) alignSearch(t int64) int64 {
	d := int64(r.search.dec)
	return t + (d-(t%d+d)%d)%d
}

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
	r.scout, r.frozen, r.pend, r.passStart = r.opt.passSlots() > 0, false, false, -back
	if r.hg != nil && r.h != cohIndex {
		r.setH(cohIndex)
	}
	clear(r.passRecs)
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

func (r *engine) searchFreq(j int64, center, span, step float64) {
	if span > 20*step {
		coarse := 20 * step
		center, span = r.bestFreq(j, center, span, coarse), coarse
	}
	r.freq = r.bestFreq(j, center, span, step)
}

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

func (r *engine) setH(h float64) {
	r.h = h
	r.search.setH(h, &r.pulse)
	if r.model != nil {
		r.model.setH(h)
	}
	if r.xt != nil && r.opt.ExactH {
		r.xt.setH(h)
	}
	r.devHz = 3 * deviationHz * h / 0.27
}

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

func (r *engine) syncStart(s *slotState, tau float64) float64 {
	return r.symTime(s.index, syncFirstSym+syncEdge) + tau - float64(r.search.sps/2*r.search.dec)
}

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

func (r *engine) slotTime(j int64) float64 {
	return r.t0 + float64(j)*r.period
}

func (r *engine) symTime(j int64, k int) float64 {
	return r.slotTime(j) + float64(k)*r.period/slotSymbols
}

func (r *engine) slot(j int64) *slotState {
	return &r.slots[r.slotPos(j)]
}

func (r *engine) resetSlot(j int64) *slotState {
	s := r.slot(j)
	*s = slotState{index: j, start: r.slotTime(j)}
	r.syms.clear(r.syms.base(j), slotSymbols, 0)
	r.syms.own[r.syms.base(j)/slotSymbols] = j
	return s
}

func (r *engine) slotPos(j int64) int {
	return floorMod(j, len(r.slots))
}

func floorMod(j int64, n int) int {
	x := int(j) % n
	if x < 0 {
		x += n
	}
	return x
}

type symStore struct {
	slots  int
	digit  []int8
	llr    []int16
	have   []int16
	own    []int64
	scale  float32
	logmap bool
}

func newSymStore(n int, scale float32) symStore {
	return symStore{slots: n, digit: make([]int8, n*slotSymbols), llr: make([]int16, 2*n*slotSymbols), have: make([]int16, n*slotSymbols),
		own: make([]int64, n), scale: scale}
}

func (st *symStore) base(j int64) int {
	return floorMod(j, st.slots) * slotSymbols
}

func (st *symStore) clear(p, n int, have int16) {
	clear(st.digit[p : p+n])
	clear(st.llr[2*p : 2*(p+n)])
	for i := p; i < p+n; i++ {
		st.have[i] = have
	}
}

func (st *symStore) set(p int, a []float32) {
	best := 0
	for v := 1; v < 4; v++ {
		if a[v] > a[best] {
			best = v
		}
	}
	st.digit[p] = int8(best)
	if st.logmap {
		st.llr[2*p], st.llr[2*p+1] = dibitLLRExact(a, st.scale)
		return
	}
	st.llr[2*p], st.llr[2*p+1] = dibitLLR(a, st.scale)
}

func dibitLLRExact(a []float32, scale float32) (int16, int16) {
	b0 := logAdd(a[2], a[3]) - logAdd(a[0], a[1])
	b1 := logAdd(a[1], a[2]) - logAdd(a[0], a[3])
	return satLLR(scale * b0), satLLR(scale * b1)
}

func (r *engine) symsOf(s *slotState) (*symStore, int) {
	if j := s.index; r.refining && j >= r.refLo && j < r.refLo+int64(r.refined.slots) {
		return &r.refined, int(j-r.refLo) * slotSymbols
	}
	return &r.syms, r.syms.base(s.index)
}

func (r *engine) followStep(end bool) bool {
	if r.frozen && r.next >= r.passEnd {
		r.endPass()
		return true
	}
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
	if r.scout {
		*r.pass(j) = passRec{}
	}
	r.passAt(j)
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
	r.passKeep(j)
	if !r.frozen {
		r.checkStream(j)
	}
	return true
}

func (r *engine) checkStream(j int64) {
	switch {
	case r.lost >= lostSlots || r.lastAnch == math.MinInt64 && j >= 6 || !r.bs && r.quiet >= msQuietSlots:
		r.finishStream()
	case r.opt.Coherent && !r.warm && r.stale(j) && r.scanStale(j):
	case r.scout && !r.warm && r.next-r.passStart >= int64(r.opt.passSlots()):
		r.startPass(true)
	case r.warm && j >= warmSlots:
		r.endWarm(j)
	}
}

func (r *engine) endWarm(j int64) {
	r.warmFreq(j)
	r.warm = false
	r.next, r.first = r.warmStart, r.warmStart
	r.lastAnch, r.lost = math.MinInt64, 0
	r.fit = [5]float64{}
}

func (r *engine) stale(j int64) bool {
	if r.lastAnch == math.MinInt64 || j < 0 {
		return false
	}
	if r.bs {
		return j-r.lastAnch > staleSlots
	}
	return j-r.lastAnch > 2*layer2.VoiceBurstsPerSuperframe+1
}

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
	if r.locked {
		r.pend, r.pendAt, r.pendDone = true, r.alignSearch(at-searchStep), done
		return true
	}
	r.searchAt, r.doneAt, r.candAt = r.alignSearch(at-searchStep), done, -1
	return true
}

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

func (r *engine) finishStream() {
	if r.warm {
		r.endWarm(r.next - 1)
		return
	}
	if r.scout {
		r.startPass(false)
		return
	}
	r.frozen = false
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
			if r.emits() {
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
	if r.frozen {
		return
	}
	r.passAnchor(s, bestTau)
	r.timing(s, bestTau)
	if g := (best.amp / r.amp) * (best.amp / r.amp); r.amp == 0 || g > levelJump || g < 1/levelJump {
		r.amp, r.noise = best.amp, best.noise
	} else {
		r.amp = (1-levelGain)*r.amp + levelGain*best.amp
		r.noise = (1-levelGain)*r.noise + levelGain*best.noise
	}
}

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

func (r *engine) rotAt(t float64) complex128 {
	return complex128(turn(-r.freq * t / basebandRate))
}

func turn(c float64) complex64 {
	s, co := sincosf(float32(2 * math.Pi * (c - math.Round(c))))
	return complex(co, s)
}

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
		if r.emits() {
			r.finish(r.slot(k))
		}
	}
	r.first = last + 1
}

func (r *engine) detectBurst(s *slotState) {
	if s.present {
		if !r.opt.Coherent {
			r.discDetect(s)
		} else {
			r.coherentBurst(s.index)
		}
		if r.emits() {
			r.finish(s)
		}
	}
	r.first = s.index + 1
}

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

func dibitLLR(a []float32, scale float32) (int16, int16) {
	b0 := max32(a[2], a[3]) - max32(a[0], a[1])
	b1 := max32(a[1], a[2]) - max32(a[0], a[3])
	return satLLR(scale * b0), satLLR(scale * b1)
}

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

func (r *engine) repairEntry(e *trackEntry, cc enums.ColorCode, ccOK bool) {
	c := &r.tk.ch[((e.rec.index%2)+2)%2]
	r.rep.raw = c.raw
	r.rep.repair(e, cc, ccOK)
	if _, kind, known := layer2.SyncSourceOf(e.out.sync); known && kind == layer2.KindData {
		noteCallLC(&c.raw, &e.out.fixed, r.rep.checks[enums.DataTypeVoiceLCHeader])
	}
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
	if r.opt.Comb {
		e.out.side.flips, e.out.side.hasFlips = voiceFlips(&e.out.data, &e.out.fixed), true
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

type dataPass struct {
	ext  [layer2.BurstBits]float32
	ch   [layer2.BurstBits]int16
	llr  [layer2.BurstBits]int16
	next [layer2.BurstBits]int16
}

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

func bitPrior(l float32, b unpacked.Bit) float32 {
	if b == 0 {
		return l / 2
	}
	return -l / 2
}

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
