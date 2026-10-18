package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Layout of the streaming discriminator receiver.
const (
	// leanRing is the number of symbols that the receiver keeps: a slot,
	// whose SYNC ends 54 symbols before its end, and the symbol on each
	// side that a slip of the slot timing reads.
	leanRing = slotSymbols + 2
	// leanChunk is the number of baseband samples that Write converts at
	// a time from input at another rate.
	leanChunk = 6
	// leanSyncEnd is the slot symbol of the last SYNC symbol.
	leanSyncEnd = syncFirstSym + syncSymbols - 1
	// leanAcqSymbols is the number of symbols from the start of a burst to
	// the end of its SYNC, which the receiver samples again from the
	// matched filter samples when it locks, and leanAcqSamples the length
	// of their ring, for up to 2.5 samples a symbol, even for the pairs of
	// leanFEC.s.
	leanAcqSymbols = leanSyncEnd + 1 - cachSymbols
	leanAcqSamples = (leanAcqSymbols*5/2 + 7) &^ 1
	// leanResample is the SYNC correlation from which searchSym samples a
	// burst again before it tests the SYNC for lock with discLock.
	leanResample = 0.7
	// leanCoarse is the SYNC correlation on whole kept samples from which
	// resample interpolates.
	leanCoarse = 0.75
	// leanAcqWait is the number of symbols after a SYNC that failed in
	// which searchSym does not sample a burst again, which near the
	// noise floor would otherwise cost more than the search.
	leanAcqWait = 48
	// leanVoteErrors is the most slot type errors of a burst whose color
	// code votes: the vote takes the most common color code, and a known
	// color code makes the soft decodes of the EMB and the slot type
	// search 16 times fewer codewords.
	leanVoteErrors = 2
	// leanSyncWindow is the number of kept samples around a SYNC that
	// resample interpolates: the SYNC at up to 2.5 samples a symbol, half
	// a symbol either way, and the taps of the interpolation.
	leanSyncWindow = (syncSymbols-1)*5/2 + 3 + 6
	// leanAcqGain is the gain of the symbol timing loop, in samples per
	// unit of normalized timing error, while the receiver searches, and
	// leanTrackGain while it follows a stream.
	leanAcqGain   = 0.15
	leanTrackGain = 0.02
	// leanPowerForget weighs the old symbol power in the normalization of
	// the timing error.
	leanPowerForget = 0.99
	// leanRetuneHz is the discriminator offset at an anchor above which
	// the receiver moves its oscillator to the carrier.
	leanRetuneHz = 100
	// leanBlock is the number of matched filter samples of a block of the
	// carrier estimate before lock, a quarter slot, and leanFloorRise the
	// factor by which the noise floor estimate rises per block.
	leanBlock     = slotSymbols * discSPS / 4
	leanFloorRise = 1.01
	// leanPresent is the ratio of the power of a block to the noise floor
	// at which the estimate before lock takes it as carrier, and
	// leanSearchHz the frequency of such a block above which it moves the
	// oscillator.
	leanPresent  = 2
	leanSearchHz = 200
	// leanSignMiss is the most symbols of a SYNC field whose sign, against
	// the mean of the field, may differ from the voice or the data SYNC of
	// a source for the search to correlate the field. The SYNC symbols are
	// all outer symbols, so a field that passes discLock differs in few.
	leanSignMiss = 5
	// leanExtSignMiss and leanExtRho are the sign misses and the least
	// SYNC correlation of a SYNC that a stage verifies.
	leanExtSignMiss = 9
	leanExtRho      = 0.3
	// llrStepHz is the resolution of the symbols in symLLR, and llrShift
	// the fixed point of its likelihood scale.
	llrStepHz = 4
	llrShift  = 24
	// leanClickGain is the weight of each burst in the share of clicks, and
	// leanMinClicks the least share that the floor takes.
	leanClickGain = 0.1
	leanMinClicks = 1e-3
	// leanClickOut is twice the distance from the DC offset, in multiples of
	// the deviation of level 1, beyond which a symbol counts as a click:
	// beyond the outer levels at 3 by more than the noise of a weak signal.
	leanClickOut = 9
	// leanCubicSPS is the most samples per symbol at which the symbol
	// timing interpolates by Catmull-Rom instead of linearly.
	leanCubicSPS = 2.5
	// leanMMSPS is the most samples per symbol at which a locked stream
	// detects the timing error by Mueller and Muller, at the symbols only,
	// with the sign leanMMSign and the gain leanMMGain over the Gardner
	// gain.
	leanMMSPS  = 2.5
	leanMMSign = 1
	leanMMGain = 2
)

// leanTiming recovers the symbol timing from the matched filter output at
// 2*step samples per symbol, with a Gardner timing error detector, and
// samples each symbol by linear interpolation.
type leanTiming struct {
	step   float32
	s1, s2 float32
	// cubic interpolates the strobes between s1 and s2 by Catmull-Rom with
	// s0 and s3, one sample late, for few samples per symbol.
	cubic  bool
	s0, s3 float32
	// t is the time of the next strobe, in samples after the last one.
	// The strobes alternate between symbols and the midpoints between them.
	t    float32
	mid  bool
	last float32
	half float32
	pow  float32
	// hold stops the loop, while the carrier of a burst stream is off.
	hold bool
	gain float32
	// mm, set while a stream is locked at few samples per symbol, strobes
	// only the symbols and detects the timing error by Mueller and Muller
	// from the decisions of the levels at dc ± dev and ± 3 dev; half then
	// holds the last decision.
	mm      bool
	dc, dev float32
}

// push takes one matched filter sample and returns the symbol value when
// a symbol strobe falls between the last two samples, with the time of the
// strobe from the last sample, from -1 to 0. It takes every strobe that
// falls there, so that at few samples per symbol a midpoint after a symbol
// strobe is not left for the next sample to extrapolate.
func (tm *leanTiming) push(s float32) (v, at float32, ok bool) {
	if tm.cubic {
		tm.s0, tm.s1, tm.s2, tm.s3 = tm.s1, tm.s2, tm.s3, s
	} else {
		tm.s1, tm.s2 = tm.s2, s
	}
	tm.t--
	for tm.t <= 0 {
		t := tm.t
		if tm.strobe(tm.at(t)) {
			v, at, ok = tm.last, t, true
		}
	}
	if ok && tm.cubic {
		// The cubic strobes are one sample late.
		at--
	}
	return v, at, ok
}

// strobe takes the matched filter value x of the strobe at t, moves t to
// the next strobe and reports whether the strobe was of a symbol, whose
// value is then last.
func (tm *leanTiming) strobe(x float32) bool {
	if tm.mm {
		y := x - tm.dc
		d := tm.dev
		if abs32(y) > 2*tm.dev {
			d = 3 * tm.dev
		}
		if y < 0 {
			d = -d
		}
		e := leanMMSign * (tm.half*y - d*(tm.last-tm.dc))
		tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*y*y
		tm.last, tm.half = x, d
		tm.t += 2 * tm.step
		if !tm.hold && tm.pow > 0 {
			tm.t += max32(min32(leanMMGain*tm.gain*e/tm.pow, 0.5), -0.5)
		}
		return true
	}
	if tm.mid {
		tm.half, tm.mid = x, false
		tm.t += tm.step
		return false
	}
	tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*x*x
	e := tm.half * (tm.last - x)
	tm.last, tm.mid = x, true
	tm.t += tm.step
	if !tm.hold && tm.pow > 0 {
		tm.t += max32(min32(tm.gain*e/tm.pow, 0.5), -0.5)
	}
	return true
}

// at returns the matched filter output at time t, from -1 to 0, from the
// last sample: by Catmull-Rom between the two samples before the last when
// cubic, one sample late, and linearly between the last two otherwise.
func (tm *leanTiming) at(t float32) float32 {
	if !tm.cubic {
		return tm.s2 + t*(tm.s2-tm.s1)
	}
	p0, p1, p2, p3 := tm.s0, tm.s1, tm.s2, tm.s3
	mu := 1 + t
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0)))
}

// leanChan is the superframe state of one TDMA channel of the streaming
// tracker: the slot of the last burst A and of the last voice LC or PI
// header.
type leanChan struct {
	sf  int32
	hdr int32
}

// leanNoSlot marks an unset slot of leanChan.
const leanNoSlot = math.MinInt32

// leanTuner is the I/Q front end of a leanCore.
type leanTuner interface {
	retuneFront(hz, d float32)
}

// leanCore is the streaming receiver after the discriminator: it takes the
// matched filter samples at discSPS samples per symbol, keeps the symbols of
// one slot and the state of the stream, and decodes each burst as its slot
// ends, without waiting for later slots.
type leanCore struct {
	onBurst func(*Burst)
	out     []Burst
	// audio or iq is the front end, whose retuneFront moves the carrier
	// that it removes to hz, d Hz from the last, and adds -d to the
	// discriminator samples that it holds. audio is a pointer, so that
	// TinyGo links neither type data nor the I/Q front end for a
	// DiscReceiver; a method value would allocate.
	audio *audioFront
	iq    leanTuner
	// signMiss is the most SYNC symbols whose sign may miss before the
	// search tests them: leanSignMiss, more with a stage that verifies.
	signMiss uint8
	// suspect marks a lock whose last slot that should have anchored did
	// not: after a data burst, or where a superframe expects burst A, or
	// whose burst B to F has an EMB that does not decode. hunt marks a
	// lock that was stale at the end of its last slot: the receiver then
	// keeps the matched filter samples and searches for a SYNC off the
	// slots, as it does while unlocked. dataAnch marks each TDMA channel
	// whose last slot anchored with the data SYNC, after which every
	// burst has a SYNC. They fill the room before ext, so that the
	// receiver keeps its size.
	suspect, hunt bool
	dataAnch      [2]bool
	// ext is the stage of a higher tier that decodes and emits each burst
	// in place of repair, nil for Tier 0, and replaying is set while it
	// runs the slots before a lock through slot; with replayDry the slots
	// only collect the color code and emit nothing.
	ext                  leanExt
	replaying, replayDry bool
	// center is the input sample at the center of matched filter sample 0,
	// in baseband samples, and mfStep the baseband samples per matched
	// filter sample, a whole number at every input rate.
	center int64
	mfStep int64
	// acqGain and trackGain are leanAcqGain and leanTrackGain for the
	// samples per symbol of the matched filter, and mmOK selects Mueller
	// and Muller while a stream is locked.
	acqGain, trackGain float32
	mmOK               bool
	tm                 leanTiming
	// n counts the matched filter samples and ns the symbols. ring holds
	// the last symbols in units of discStepHz, plus off.
	n    int64
	ns   int64
	ring [leanRing]int8
	// acqHead is the index in the ring of samples of leanFEC.s of the next
	// matched filter sample, and acqFrom the matched filter sample from
	// which the receiver searches, and keeps them.
	acqHead int32
	acqFrom int64
	// acqLow is the last kept sample while acqHead is odd, whose pair the
	// ring does not hold yet.
	acqLow int16
	// acqNext is the first symbol at which searchSym samples a burst
	// again after a SYNC that failed.
	acqNext int64
	off     float32
	// slotEnd is the symbol after the slot that ends next, first plus next
	// plus 1 slots.
	slotEnd int64
	// ringSum is the sum of the last syncSymbols ring values, and signs
	// holds the sign of each against ringSum as it entered, the last in
	// bit 0.
	ringSum int32
	signs   uint32
	// tuneHz is the carrier offset that the front end removes.
	tuneHz float32
	// lastAt places the last symbol, lastAt samples after matched filter
	// sample n-1: the slots end at a symbol, so n-1 is then its sample.
	lastAt float32

	locked bool
	// forced marks a lock that a stage forced, or that a search while
	// locked took, and that no later SYNC or EMB has confirmed yet; lockNs
	// is the symbol of the lock.
	forced  bool
	softFEC bool
	src     uint8
	sources uint8
	bs      bool
	// first is the symbol of the start of slot 0 of the stream, next the
	// slot that ends next, lastAnch the last slot with a SYNC, lost the
	// slots since the last slot with carrier.
	first    int64
	lockNs   int64
	next     int32
	lastAnch int32
	lost     int32
	// level estimates of the stream: the discriminator offset, the
	// deviation scale and the noise variance, in Hz and Hz squared, and
	// amp and noise the power of the bursts and of the noise.
	dcHz     float32
	devScale float32
	discVar  float32
	amp      float32
	noise    float32
	// eAcc sums the prefilter power of the symbols of the slot that ends
	// next, eSyms counts them.
	eAcc  float32
	eSyms int32

	ch      [2]leanChan
	votes   [16]uint8
	streams int32
	// base is the first symbol of the slot under detection, anchor its
	// SYNC pattern when it is an anchor, and ll, lfloor and dev the factor
	// and the floor of the symbol log-likelihoods and the deviation of
	// level 1, as discDetect has them.
	base   int64
	anchor enums.SyncPattern
	ll     float32
	lfloor float32
	// floorC is the part of lfloor that does not depend on the noise.
	floorC float32
	// llrOff, llrDev, llrK and llrFloor are the settings of symLLR in
	// fixed point: the offset and the deviation in units of llrStepHz,
	// -ll for distances in those units in units of 1/8 in Q llrShift, and
	// lfloor in units of 1/8.
	llrOff, llrDev int32
	llrK           int64
	llrFloor       int32
	dev            float32
	outlier        float32
	// clickSpan, when above 0, sets the floor of the symbol likelihoods from
	// clicks, the share of symbols beyond the outer levels that the bursts
	// show, spread over clickSpan Hz, instead of from outlier.
	clickSpan float32
	clicks    float32

	offsetHz int16
	devHz    int16
	// cb holds the burst under detection and fec the scratch of its
	// decode, which as locals TinyGo puts on the heap.
	cb  Burst
	fec leanFEC
}

// leanRx is the receiver of Tier0 and Tier1 at BasebandRate/2: the
// complex front end leanFront before a leanCore.
type leanRx struct {
	leanCore
	// fe and wide convert the bytes of Write at another rate than the
	// baseband, and dc and dcN remove the DC offset of the bytes of Write
	// at the baseband rate.
	fe      *frontEnd
	wide    []complex64
	inChunk int
	carry   byteCarry
	dc      complex64
	dcN     int32
	signed  bool
	df      leanFront
	// bPow and bLag sum the prefilter power and the lag products of the
	// block of the carrier estimate before lock, bN counts its samples,
	// floor is the noise floor estimate and noiseLag the mean lag product
	// of the noise.
	bPow     float32
	bLag     complex64
	bN       int32
	floor    float32
	noiseLag complex64
}

// retuneFront moves the oscillator of f to hz and adds -d to the
// discriminator samples that it holds.
func (f *leanFront) retuneFront(hz, d float32) {
	f.tune(float64(hz))
	f.shift(-d)
}

// newLean returns the streaming receiver of cfg with the settings opt.
func newLean(cfg Config, opt options) *leanRx {
	r := &leanRx{signed: cfg.Signed}
	r.init(cfg.OnBurst, opt, discSPS)
	if cfg.OnBurst == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+16)
	}
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Signed, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		r.inChunk = max(2*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), 2)
	}
	r.df.init(cfg.BasebandHz == discRate)
	r.iq, r.center = &r.df, r.df.center()
	if opt.SoftExt {
		r.ext = softExtOf(&opt, discSPS)
	}
	return r
}

// init sets up r for matched filter samples at sps samples per symbol with
// the settings opt, to emit bursts to onBurst, or, when it is nil, to
// append them to out while it has room.
func (r *leanCore) init(onBurst func(*Burst), opt options, sps float32) {
	r.onBurst, r.lastAnch, r.softFEC = onBurst, leanNoSlot, opt.SoftFEC
	r.sources, r.outlier = uint8(opt.Sources), float32(opt.DiscOutlier)
	r.clickSpan = float32(opt.ClickSpan)
	r.signMiss = leanSignMiss
	if po := opt.DiscOutlier; po > 0 {
		r.floorC = float32(math.Log(po/basebandRate) - math.Log(1-po) + 0.5*math.Log(2*math.Pi))
	}
	r.mfStep = int64(samplesPerSy / sps)
	r.acqGain, r.trackGain = leanAcqGain*(sps/discSPS), leanTrackGain*(sps/discSPS)
	r.tm.step = sps / 2
	r.tm.cubic = sps <= leanCubicSPS
	r.mmOK = sps <= leanMMSPS
	r.tm.t, r.tm.gain = r.tm.step, r.acqGain
	r.resetChans()
}

// resetChans clears the superframe state of the stream.
func (r *leanCore) resetChans() {
	for i := range r.ch {
		r.ch[i] = leanChan{sf: leanNoSlot, hdr: leanNoSlot}
	}
	r.dataAnch = [2]bool{}
	r.votes = [16]uint8{}
}

// write processes the I/Q bytes of Receiver.Write.
func (r *leanRx) write(iq []byte) []Burst {
	r.out = r.out[:0]
	pair, iq := r.carry.split(iq)
	r.bytes(pair)
	r.bytes(iq)
	return r.out
}

// bytes processes whole I/Q pairs of bytes.
func (r *leanRx) bytes(iq []byte) {
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
		return
	}
	for len(iq) > 0 {
		n := min(len(iq), r.inChunk) &^ 1
		if n == 0 {
			break
		}
		w := r.fe.process(iq[:n], r.wide)
		iq = iq[n:]
		for _, v := range r.wide[:w] {
			r.sample(v)
		}
	}
}

// byteSample converts one I/Q pair of bytes at the baseband rate to a
// sample, with the DC offset removal of frontEnd.
func (r *leanRx) byteSample(i, q byte) complex64 {
	var v complex64
	if r.signed {
		v = complex(float32(int8(i))/127.5, float32(int8(q))/127.5)
	} else {
		v = complex((float32(i)-127.5)/127.5, (float32(q)-127.5)/127.5)
	}
	if r.dcN < math.MaxInt32 {
		r.dcN++
	}
	g := max(1/(dcSeconds*discRate*2), 1/(float32(r.dcN)+dcStartSeconds*discRate*2))
	r.dc += (v - r.dc) * complex(g, 0)
	return v - r.dc
}

// writeBaseband processes the samples of Receiver.WriteBaseband.
func (r *leanRx) writeBaseband(x []complex64) []Burst {
	r.out = r.out[:0]
	for _, v := range x {
		r.sample(v)
	}
	return r.out
}

// flush finishes the stream at its end and returns the remaining bursts.
func (r *leanRx) flush() []Burst {
	r.out = r.out[:0]
	for range slotSamplesN {
		r.sample(0)
	}
	if r.locked {
		r.unlock()
	}
	return r.out
}

// sample runs one baseband sample through the front end and the symbol
// timing, and handles each symbol.
func (r *leanRx) sample(v complex64) {
	s, p, ok := r.df.push(v)
	if !ok {
		return
	}
	r.power(p)
	if !r.locked {
		r.block(p)
	}
	if !r.locked || r.hunt {
		r.keep(s)
	}
	r.push(s)
}

// power counts one matched filter sample, whose prefilter power is p.
func (r *leanCore) power(p float32) {
	r.n++
	r.eAcc += p
	r.eSyms++
}

// keep stores matched filter sample s, in Hz, in the ring of samples while
// the receiver searches, so that searchSym can sample the start of a burst
// again at the timing of its SYNC.
func (r *leanCore) keep(s float32) {
	r.keepQ(int32(roundf(s * (1.0 / discStepHz))))
}

// keepQ stores a matched filter sample in units of discStepHz, as keep
// does, without a float conversion, which TinyGo checks for range.
func (r *leanCore) keepQ(q int32) {
	buf, an := r.acqBuf()
	v := int16(max(min(q, math.MaxInt8), math.MinInt8))
	h := r.acqHead
	if h&1 == 0 {
		r.acqLow = v
	} else if i := h >> 1; i < int32(len(buf)) {
		buf[i] = int16(uint16(uint8(r.acqLow)) | uint16(uint8(v))<<8)
	}
	if h++; h == an {
		h = 0
	}
	r.acqHead = h
}

// acqBuf returns the ring of kept samples, two to an element, and its
// length in samples: the FEC scratch, or the longer ring of a stage that
// decodes the slots before a lock.
func (r *leanCore) acqBuf() ([]int16, int32) {
	if r.ext != nil {
		if b := r.ext.kept(); b != nil {
			return b, int32(2 * len(b))
		}
	}
	return r.fec.s[:leanAcqSamples/2], leanAcqSamples
}

// rescale multiplies the symbols of the ring, the kept samples and the
// samples and power of the timing loop by g, as the front end scales its
// input while the receiver searches.
func (r *leanCore) rescale(g float32) {
	// In integers, Q8: TinyGo checks each float conversion for range.
	g8 := int32(g*256 + 0.5)
	for i, v := range &r.ring {
		r.ring[i] = int8(scale8(int32(v), g8, 0))
	}
	r.rebuildSigns()
	r.rescaleKept(g8, 0)
	tm := &r.tm
	tm.s0, tm.s1, tm.s2, tm.s3 = tm.s0*g, tm.s1*g, tm.s2*g, tm.s3*g
	tm.last, tm.half, tm.pow = tm.last*g, tm.half*g, tm.pow*g*g
}

// rescaleKept sets each kept sample x to x*g8/256+a8/256, rounded.
func (r *leanCore) rescaleKept(g8, a8 int32) {
	buf, _ := r.acqBuf()
	for i, w := range buf {
		lo, hi := scale8(int32(int8(uint8(w))), g8, a8), scale8(int32(int8(uint8(uint16(w)>>8))), g8, a8)
		buf[i] = int16(uint16(uint8(lo)) | uint16(uint8(hi))<<8)
	}
	r.acqLow = int16(scale8(int32(r.acqLow), g8, a8))
}

// scale8 returns x*g8/256+a8/256, rounded and saturated to int8. An x at
// a limit of int8 was clipped and stays there.
func scale8(x, g8, a8 int32) int32 {
	if x == math.MaxInt8 || x == math.MinInt8 {
		return x
	}
	return max(min((x*g8+a8+128)>>8, math.MaxInt8), math.MinInt8)
}

// acqKept returns the number of samples in the ring of samples.
func (r *leanCore) acqKept() int32 {
	_, an := r.acqBuf()
	return int32(min(r.n-r.acqFrom, int64(an)))
}

// kept returns the stored sample back samples before the last one.
func (r *leanCore) kept(back int32) float32 {
	buf, an := r.acqBuf()
	h := r.acqHead - 1 - back
	if h < 0 {
		h += an
	}
	if h == r.acqHead-1 && h&1 == 0 {
		// Its pair is not stored yet.
		return float32(r.acqLow)
	}
	w := uint16(buf[h>>1])
	if h&1 != 0 {
		w >>= 8
	}
	return float32(int8(uint8(w)))
}

// keptAt returns the stored samples interpolated by Catmull-Rom at t
// samples after the last one, t at most 0, in units of discStepHz less
// off, as the ring holds the symbols.
func (r *leanCore) keptAt(t float32) float32 {
	b := -t
	i := int32(b)
	mu := 1 - (b - float32(i))
	// p1 and p2 are the samples i+1 and i back, around t.
	p0, p1, p2, p3 := r.kept(i+2), r.kept(i+1), r.kept(i), r.kept(max(i-1, 0))
	if i+3 >= r.acqKept() {
		p0 = p1
	}
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))) - r.off*(1.0/discStepHz)
}

// coarseSync reports whether the kept samples of win nearest the SYNC
// symbols syms, which ends end samples after the last sample, one sample
// early, on time or one sample late, correlate with them as a SYNC may,
// a cheap test, in integers, before the interpolations of resample, which
// noise would otherwise run often.
func coarseSync(win *syncWindow, syms *[syncSymbols]int8, end, sps float32) bool {
	// Sample positions back from the last, in Q8.
	e, step := int32(-end*256+0.5), int32(sps*256+0.5)
	// The sums of the best offset: of the samples, their squares and
	// their products with the SYNC symbols. sp is the sum of the symbols.
	var best, bsv, bsvv, sp int32
	for _, s := range syms {
		sp += int32(s)
	}
	for d := int32(-1); d <= 1; d++ {
		if end+float32(d) > 0 {
			// Its last symbol is after the last sample.
			break
		}
		var c, sv, svv int32
		for i, s := range syms {
			b := (e-d*256+int32(syncSymbols-1-i)*step+128)>>8 - win.b0
			v := int32(win.w[min(max(b, 0), leanSyncWindow-1)])
			c += int32(s) * v
			sv += v
			svv += v * v
		}
		if c > best {
			best, bsv, bsvv = c, sv, svv
		}
	}
	if best <= 0 {
		return false
	}
	// rho² from the sums, with the SYNC symbols ±3.
	n := float32(syncSymbols)
	cov := float32(best) - float32(sp)*float32(bsv)/n
	vv := float32(bsvv) - float32(bsv)*float32(bsv)/n
	pp := 9*n - float32(sp)*float32(sp)/n
	return cov > 0 && cov*cov >= leanCoarse*leanCoarse*vv*pp
}

// fillWindow copies the kept samples from b0 back from the last into win.
func (r *leanCore) fillWindow(win *syncWindow, b0 int32) {
	win.b0, win.off = b0, r.off*(1.0/discStepHz)
	buf, an := r.acqBuf()
	h := r.acqHead - 1 - b0
	for k := range win.w {
		if h < 0 {
			h += an
		}
		if h == r.acqHead-1 && h&1 == 0 {
			// Its pair is not stored yet.
			win.w[k] = int8(r.acqLow)
		} else if i := h >> 1; i < int32(len(buf)) {
			w := uint16(buf[i])
			if h&1 != 0 {
				w >>= 8
			}
			win.w[k] = int8(uint8(w))
		}
		h--
	}
}

// syncWindow holds kept samples around a SYNC for resample: w[k] is the
// sample b0+k back from the last, and off the offset of the ring in units
// of discStepHz. It is a type and not a closure, which TinyGo would
// allocate.
type syncWindow struct {
	w   [leanSyncWindow]int8
	b0  int32
	off float32
}

// at returns the samples of w interpolated by Catmull-Rom at t samples
// after the last one, in units of discStepHz less off, as keptAt does.
func (win *syncWindow) at(t float32) float32 {
	b := -t
	i := int32(b)
	mu := 1 - (b - float32(i))
	k := min(max(i-win.b0, 0), leanSyncWindow-3)
	p0, p1, p2, p3 := float32(win.w[k+2]), float32(win.w[k+1]), float32(win.w[k]), float32(win.w[max(k-1, 0)])
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))) - win.off
}

// resample samples the SYNC p that ends at the last symbol again from the
// kept samples: it finds the strobe offset that best correlates with it
// and, when the SYNC there passes the lock test, writes its symbols at it
// into the ring, with the ones they replace in saved, and returns the
// offset and true; resampleStart then samples the start of the burst. It
// reports false when the SYNC fails or the ring holds too few samples: for
// the burst from its start, or while locked for the SYNC alone.
func (r *leanCore) resample(p enums.SyncPattern, saved *[syncSymbols]int8) (float32, bool) {
	sps := 2 * r.tm.step
	// The SYNC end is lastAt samples after the last sample.
	end := r.lastAt
	if !r.burstKept() && (!r.locked || max(int32(-end-sps/2)-2, 0)+leanSyncWindow > r.acqKept()) {
		return 0, false
	}
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	// The kept samples around the SYNC, so that the tests below read an
	// array.
	var win syncWindow
	r.fillWindow(&win, max(int32(-end-sps/2)-2, 0))
	if !coarseSync(&win, &syms, end, sps) {
		return 0, false
	}
	// The correlations from half a symbol early to half a symbol late in
	// quarter symbols, and the peak of the parabola around the best.
	var c [5]float32
	best := 2
	for k := range c {
		d := float32(k-2) * sps / 4
		if end+d > 0 {
			// Its last symbol is after the last sample.
			c[k] = float32(math.Inf(-1))
			continue
		}
		for i, s := range syms {
			c[k] += float32(s) * win.at(end+d-float32(syncSymbols-1-i)*sps)
		}
		if c[k] > c[best] {
			best = k
		}
	}
	bestD := float32(best-2) * sps / 4
	lo, hi := c[max(best-1, 0)], c[min(best+1, len(c)-1)]
	if den := lo - 2*c[best] + hi; best > 0 && best < len(c)-1 && den < 0 && !math.IsInf(float64(lo+hi), 0) {
		bestD += max32(min32(0.5*(lo-hi)/den, 0.5), -0.5) * sps / 4
	}
	bestD = min(bestD, -end)
	// rho² from the sums of the samples, their squares and their products
	// with the symbols; the samples are interpolated again for the ring
	// only when the SYNC passes.
	var sv, svv, sxv, sp float32
	for i, s := range syms {
		v := win.at(end + bestD - float32(syncSymbols-1-i)*sps)
		sv += v
		svv += v * v
		sxv += float32(s) * v
		sp += float32(s)
	}
	const n = syncSymbols
	sxy := sxv - sp*sv/n
	sxx := svv - sv*sv/n
	spp := 9*n - sp*sp/n
	if sxy <= 0 || sxy*sxy < discLock*discLock*sxx*spp {
		return 0, false
	}
	for i := range int64(syncSymbols) {
		v := win.at(end + bestD - float32(syncSymbols-1-i)*sps)
		j := uint32(r.ns-syncSymbols+i) % leanRing
		saved[i], r.ring[j] = r.ring[j], int8(max(min(roundf(v), math.MaxInt8), math.MinInt8))
	}
	return bestD, true
}

// burstKept reports whether the kept samples hold the burst whose SYNC
// ends at the last symbol from its start.
func (r *leanCore) burstKept() bool {
	return float32(leanAcqSymbols)*2*r.tm.step+4-r.lastAt <= float32(r.acqKept())
}

// resampleStart writes the symbols of the burst before the SYNC that
// resample sampled again at the strobe offset d into the ring, when the
// kept samples hold them, and moves the timing loop by d.
func (r *leanCore) resampleStart(d float32) {
	sps := 2 * r.tm.step
	if !r.burstKept() {
		r.tm.t += d
		return
	}
	for i := range int64(leanAcqSymbols - syncSymbols) {
		x := r.keptAt(r.lastAt + d - float32(leanAcqSymbols-1-i)*sps)
		r.ring[uint32(r.ns-leanAcqSymbols+i)%leanRing] = int8(max(min(roundf(x), math.MaxInt8), math.MinInt8))
	}
	r.tm.t += d
}

// push runs matched filter sample s through the symbol timing and handles
// each symbol.
func (r *leanCore) push(s float32) {
	if x, at, ok := r.tm.push(s); ok {
		r.symbol(x, at)
	}
}

// roundBits returns v rounded to the nearest integer, ties to even, for
// |v| below 2^22: v plus 1.5·2^23 holds it in the low bits of its
// mantissa. It needs no float conversion, which TinyGo checks for range.
func roundBits(v float32) int32 {
	return int32(math.Float32bits(v+0x1.8p23)) - 0x4B400000
}

// symbol handles symbol value x, at samples after the last matched filter
// sample. It is not inlined, so that the code of every sample stays small.
//
//go:noinline
func (r *leanCore) symbol(x, at float32) {
	r.lastAt = at
	q := int8(max(min(roundBits((x-r.off)/discStepHz), math.MaxInt8), math.MinInt8))
	h := uint32(r.ns) % leanRing
	r.ring[h] = q
	old := h + leanRing - syncSymbols
	if old >= leanRing {
		old -= leanRing
	}
	r.ringSum += int32(q) - int32(r.ring[old])
	r.signs <<= 1
	if int32(q)*syncSymbols > r.ringSum {
		r.signs |= 1
	}
	r.ns++
	if !r.locked {
		// The search starts at the first SYNC that can end the first
		// slot, so that a receiver that starts in a call finds it.
		switch {
		case r.ns > leanSyncEnd+1 && r.signsMatch():
			r.searchSym(false)
		case r.ns >= leanRing && r.ext != nil:
			if src, data, back, ok := r.ext.scan(r); ok && !r.clipped(r.ns-1-back) {
				r.forceLock(src, data, back)
			}
		}
		return
	}
	if r.hunt && r.ns >= leanRing && r.signsMatch() {
		if !r.offGrid() {
			r.retime()
		} else if r.searchSym(true); r.lockNs == r.ns {
			r.forced = true
		}
	}
	if r.ns == r.slotEnd {
		if r.ext != nil && r.next == 0 {
			r.ext.firstSlot(r)
		}
		r.slot()
	}
}

// block adds the prefilter power p and the last lag product to the
// carrier estimate before lock. At the end of a block whose power stands
// above the noise floor, it moves the oscillator to the frequency of the
// mean lag product less that of the noise: the noise alone, white before
// the prefilter, has a real lag product that pulls the phase toward 0 Hz.
func (r *leanRx) block(p float32) {
	r.bPow += p
	r.bLag += r.df.lag
	if r.bN++; r.bN < leanBlock {
		return
	}
	pw, lag := r.bPow/leanBlock, r.bLag/leanBlock
	r.bPow, r.bLag, r.bN = 0, 0, 0
	if r.floor == 0 || pw < r.floor {
		r.floor = pw
	} else {
		r.floor *= leanFloorRise
	}
	if pw <= leanPresent*r.floor {
		r.noiseLag = 0.9*r.noiseLag + 0.1*lag
		return
	}
	lag -= r.noiseLag
	if hz := atan2f(imag(lag), real(lag)) * (discRate / (2 * math.Pi)); abs32(hz) > leanSearchHz {
		r.retune(hz)
	}
}

// sym returns symbol k in Hz.
func (r *leanCore) sym(k int64) float32 {
	return float32(r.ring[uint32(k)%leanRing])*discStepHz + r.off
}

// syncAt returns the correlation of the SYNC field that ends at symbol end
// with the voice SYNC of each source, as discAt.
func (r *leanCore) syncAt(end int64) (rho2 float32, src int, data bool) {
	// The sums run over the ring values: the offset and the step of the
	// ring cancel in the correlation, and the voice SYNC of a source less
	// its mean is syncSyms8 less syncMean.
	var mx, xx int32
	var sr [numSources]int32
	j := uint32(end-syncSymbols+1) % leanRing
	for k := range syncSymbols {
		q := int32(r.ring[j])
		if j++; j == leanRing {
			j = 0
		}
		mx += q
		xx += q * q
		for i := range r.sources {
			sr[i] += int32(syncSyms8[i][k]) * q
		}
	}
	fm := float32(mx)
	sxx := float32(xx) - fm*fm/syncSymbols
	if sxx <= 0 {
		return 0, 0, false
	}
	var best, sxyBest float32
	for i := range int(r.sources) {
		sxy := float32(sr[i]) - syncMean[i]*fm
		if q := sxy * sxy / syncDevSq[i]; q > best {
			best, src, sxyBest = q, i, sxy
		}
	}
	return best / sxx, src, sxyBest < 0
}

// signsMatch reports whether the signs of the last syncSymbols symbols
// differ from the voice or the data SYNC of a source in at most
// leanSignMiss symbols.
func (r *leanCore) signsMatch() bool {
	for i := range r.sources {
		// A byte table: TinyGo counts the bits of a word in 11
		// instructions on the M4.
		x := r.signs ^ syncSigns[i]
		m := ones8[x&0xff] + ones8[x>>8&0xff] + ones8[x>>16&0xff]
		if m <= r.signMiss || m >= syncSymbols-r.signMiss {
			return true
		}
	}
	return false
}

// searchSym tests whether the last symbols, whose signs match a SYNC, end
// a SYNC and locks to it: when strict, only to one that the discriminator
// shows clearly, in place of a lock.
//
//go:noinline
func (r *leanCore) searchSym(strict bool) {
	if r.clipped(r.ns - 1) {
		return
	}
	rho2, src, data := r.syncAt(r.ns - 1)
	// The timing loop of a lock samples a stream at another symbol
	// timing poorly, so that the search while locked samples a SYNC again
	// from a weaker correlation.
	gate := float32(leanResample)
	if strict {
		gate = leanExtRho
	}
	weak := rho2 < gate*gate
	if weak && (strict || r.ext == nil || rho2 < leanExtRho*leanExtRho) {
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	// The timing loop may not have settled on the burst yet: sample its
	// start again at the timing of the SYNC, or else test the symbols as
	// they are.
	ok := false
	var d float32
	if !weak && (strict || r.ns >= r.acqNext) {
		var saved [syncSymbols]int8
		var sampled bool
		if d, sampled = r.resample(p, &saved); sampled {
			if ok = r.fitLevels(r.ns-1, p, true, true); ok {
				r.resampleStart(d)
			} else {
				// Back to the SYNC as the loop sampled it.
				for i, q := range &saved {
					r.ring[uint32(r.ns-syncSymbols+int64(i))%leanRing] = q
				}
			}
		}
		if !ok {
			if !strict {
				r.acqNext = r.ns + leanAcqWait
			}
			d = 0
		}
	}
	if !ok && (rho2 < discLock*discLock || !r.fitLevels(r.ns-1, p, true, true)) {
		// A stage may find the SYNC that the discriminator does not, but
		// not in place of a lock.
		if strict || r.ext == nil || !r.ext.verify(r, p) {
			return
		}
		r.fitLevels(r.ns-1, p, true, false)
	}
	r.lockStream(src, d, 0)
}

// clipped reports whether most symbols of the SYNC field that ends at
// symbol end lie at the limits of the ring. A DMR SYNC that the front end
// scales to discInputRMSHz lies well within them, and noise clips only a
// few: the front end scaled the field for a weaker input, such as the noise
// of an unmodulated carrier, and its levels are lost. A lock to it would
// keep that scale, as the front end keeps it for a locked stream.
func (r *leanCore) clipped(end int64) bool {
	n := 0
	for k := end - syncSymbols + 1; k <= end; k++ {
		if q := r.ring[uint32(k)%leanRing]; q == math.MaxInt8 || q == math.MinInt8 {
			n++
		}
	}
	return 2*n > syncSymbols
}

// retime samples a SYNC of the stream that ends at the last symbol, on
// the slots of a stale lock, again from the kept samples at its timing, as
// the search does before a lock, and moves the timing loop to it: a call
// that starts on the slots of the lock but between its symbols needs it.
func (r *leanCore) retime() {
	rho2, src, data := r.syncAt(r.ns - 1)
	if src != int(r.src) || rho2 < leanExtRho*leanExtRho || r.clipped(r.ns-1) {
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	var saved [syncSymbols]int8
	if d, ok := r.resample(p, &saved); ok {
		r.resampleStart(d)
	}
}

// staleSlots is the number of slots of a BS stream without a SYNC after
// which a tier with a stage searches again while it stays locked.
const staleSlots = 6

// stale reports whether the lock may be lost: a lock that a stage or a
// search while locked forced and nothing confirmed yet, a suspect lock, a
// lock whose first slot did not anchor, or a BS stream without a SYNC for
// staleSlots, whose repeater may have keyed up again with a new timing.
func (r *leanCore) stale() bool {
	if r.forced || r.suspect {
		return true
	}
	if r.lastAnch == leanNoSlot {
		return r.next > 0
	}
	return r.bs && r.next-r.lastAnch > staleSlots
}

// offGrid reports whether the last symbol ends a SYNC field off the slots
// of the stream by more than the anchors of slot test: for an MS, which
// sends in one slot of two, off the slots of its bursts.
func (r *leanCore) offGrid() bool {
	k, n := r.ns-1-leanSyncEnd-r.first, int64(slotSymbols)
	if !r.bs && r.lastAnch != leanNoSlot {
		k, n = k-int64(r.lastAnch)*slotSymbols, 2*slotSymbols
	}
	if k %= n; k < 0 {
		k += n
	}
	return k > 1 && k < n-1
}

// forceLock locks to the SYNC of source src, data or voice, that ends
// back symbols before the last, which a stage found coherently: it fits
// the levels to it however they spread.
func (r *leanCore) forceLock(src int, data bool, back int64) {
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	r.fitLevels(r.ns-1-back, p, true, false)
	r.lockStream(src, 0, back)
	r.forced = true
}

// lockStream locks to a stream of source src whose SYNC ends back symbols
// before the last, which the timing loop sampled d samples early.
func (r *leanCore) lockStream(src int, d float32, back int64) {
	r.locked, r.forced, r.suspect, r.hunt, r.src, r.bs = true, false, false, false, uint8(src), sourceAt(src) == layer2.SyncSourceBS
	r.lockNs = r.ns
	r.streams++
	r.first = r.ns - 1 - back - leanSyncEnd
	r.next, r.lastAnch, r.lost = 0, leanNoSlot, 0
	r.slotEnd = r.first + slotSymbols
	r.tm.hold, r.tm.gain = false, r.trackGain
	if r.mmOK && !r.tm.mm {
		// The next strobe of the Gardner loop after a symbol is a midpoint,
		// which Mueller and Muller skips.
		r.tm.mm, r.tm.mid = true, false
		r.tm.t += r.tm.step
		r.tm.half = 0
	}
	r.resetChans()
	r.eAcc, r.eSyms = 0, 0
	if r.ext != nil {
		r.ext.locked(r, d)
	}
}

// fitLevels fits the discriminator offset, the deviation scale and the
// noise variance to the SYNC field that ends at symbol end with pattern p,
// and reports whether they make sense for the first anchor of a stream
// when first; with check false it takes them as they come.
func (r *leanCore) fitLevels(end int64, p enums.SyncPattern, first, check bool) bool {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	k0 := end - syncSymbols + 1
	var mp, mx float32
	for k := range syncSymbols {
		mx += r.sym(k0 + int64(k))
		mp += float32(syms[k])
	}
	mp /= syncSymbols
	mx /= syncSymbols
	var sxy, spp float32
	for k := range syncSymbols {
		sxy += (float32(syms[k]) - mp) * (r.sym(k0+int64(k)) - mx)
		spp += (float32(syms[k]) - mp) * (float32(syms[k]) - mp)
	}
	g := sxy / spp / deviationHz
	dc := mx - g*deviationHz*mp
	var res float32
	for k := range syncSymbols {
		e := r.sym(k0+int64(k)) - dc - g*deviationHz*float32(syms[k])
		res += e * e
	}
	res /= syncSymbols
	if first {
		if check && (g <= 0 || sqrtf(res) > discFirstSpread*2*g*deviationHz) {
			return false
		}
		g = max(g, 0.1)
		r.devScale, r.dcHz, r.discVar = g, dc, res
		r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
		return true
	}
	r.devScale = levelForget*r.devScale + (1-levelForget)*g
	r.dcHz = levelForget*r.dcHz + (1-levelForget)*dc
	r.discVar = levelForget*r.discVar + (1-levelForget)*res
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
	return true
}

// slot handles the slot that just ended: it tests its SYNC field, updates
// the levels at an anchor, and detects, places and decodes its burst.
//
//go:noinline
func (r *leanCore) slot() {
	j := r.next
	r.next++
	r.slotEnd += slotSymbols
	base := r.first + int64(j)*slotSymbols
	energy := r.eAcc / float32(max(r.eSyms, 1))
	r.eAcc, r.eSyms = 0, 0
	voice, dp, _ := sourceAt(int(r.src)).SyncPatterns()
	var anchor enums.SyncPattern
	best, shift := float32(0), int64(0)
	for d := int64(-1); d <= 1; d++ {
		rho2, src, data := r.syncAt(base + leanSyncEnd + d)
		if src == int(r.src) && rho2 > best {
			best, shift = rho2, d
			anchor = voice
			if data {
				anchor = dp
			}
		}
	}
	isAnchor := best >= discAnchor*discAnchor && r.anchors(base+leanSyncEnd+shift, anchor)
	// An MS sends in one slot of two: the slots between hold no burst of
	// the stream, so a SYNC there is noise.
	ms := !r.bs && r.lastAnch != leanNoSlot
	if ms && (j-r.lastAnch)&1 != 0 {
		isAnchor = false
	}
	c := &r.ch[j&1]
	sf := c.sf
	if isAnchor {
		r.fitLevels(base+leanSyncEnd+shift, anchor, false, true)
		if abs32(r.dcHz) > leanRetuneHz && !r.replaying {
			r.retune(r.dcHz)
		}
		r.first += shift
		r.slotEnd += shift
		base += shift
		if r.amp == 0 {
			r.amp, r.noise = energy, energy/4
		} else {
			r.amp = levelForget*r.amp + (1-levelForget)*energy
		}
		r.lastAnch = j
		if base > r.lockNs {
			r.forced = false
		}
	}
	present := isAnchor || r.noise == 0 || energy > 1.5*r.noise
	// Nor do they carry a burst, and the timing loop holds through them.
	if ms && (j-r.lastAnch)&1 != 0 {
		present = false
	}
	r.tm.hold = !r.bs && !present
	if ms {
		r.tm.hold = (j+1-r.lastAnch)&1 != 0
	}
	switch {
	case present && (isAnchor || r.lastAnch != leanNoSlot && j-r.lastAnch < lostSlots):
		r.lost = 0
	default:
		r.lost++
	}
	if present {
		if !isAnchor {
			anchor = 0
		}
		r.burst(j, base, anchor)
	}
	if !r.replaying {
		switch {
		case isAnchor:
			r.suspect = false
		case r.dataAnch[j&1], sf != leanNoSlot && j-sf == 2*layer2.VoiceBurstsPerSuperframe && c.sf != j:
			r.suspect = true
		}
		r.dataAnch[j&1] = isAnchor && anchor == dp
	}
	if hunt := r.stale(); hunt {
		if !r.hunt || r.ext == nil {
			// Tier 0 keeps the samples in the FEC scratch, which the
			// decode of the burst overwrote.
			r.acqFrom = r.n
		}
		r.hunt = true
	} else {
		r.hunt = false
	}
	if !r.replaying && r.ended(j, base) {
		r.unlock()
	}
}

// anchors reports whether the SYNC field of pattern p that ends at symbol
// end, which correlates as an anchor, anchors the stream: its sliced
// symbols differ from p in at most discAnchorBits bits, and in a slot
// before the lock that a stage decodes again, the front end did not clip
// it. Those slots hold the samples that the receiver kept while it
// searched, which can be at the scale before the last rescale, and the
// signs of a clipped carrier match a SYNC as well as a weak one does.
func (r *leanCore) anchors(end int64, p enums.SyncPattern) bool {
	if r.syncErrors(end, p) > discAnchorBits {
		return false
	}
	return !r.replaying || !r.clipped(end)
}

// ended reports whether the stream ends at slot j, whose first symbol is
// base. lostSlots without a SYNC end it: the carrier of a BS stream stays
// on, and the noise of a discriminator keeps the energy of the slots up,
// so that a repeater that keys up again with a new timing, or an MS that
// stops, would keep it. A forced lock that no later SYNC confirms is
// false.
func (r *leanCore) ended(j int32, base int64) bool {
	return r.lost >= lostSlots || r.lastAnch == leanNoSlot && j >= 6 || j-r.lastAnch >= lostSlots ||
		r.forced && base-r.lockNs > 6*slotSymbols
}

// retune moves the oscillator by d Hz and the discriminator values that
// the receiver holds by -d, so that they read as if the oscillator had
// been there.
func (r *leanCore) retune(d float32) {
	r.tuneHz += d
	if r.audio != nil {
		r.audio.retuneFront(r.tuneHz, d)
	} else {
		r.iq.retuneFront(r.tuneHz, d)
	}
	r.shift(d)
}

// shift moves the discriminator values that the core holds by -d, as a
// front end that removes d Hz more gives them.
func (r *leanCore) shift(d float32) {
	if r.locked {
		r.off -= d
	} else {
		if r.acqKept() > 0 {
			r.rescaleKept(256, int32(roundf(-d*(256.0/discStepHz))))
		}
		// The moves of a search add up, and the ring would clip its
		// symbols against an offset that drifts: it moves the symbols by
		// whole steps instead, and off keeps the rest.
		off := r.off - d
		k := roundf(off / discStepHz)
		r.off = off - k*discStepHz
		if k != 0 {
			a8 := int32(k) * 256
			for i, v := range &r.ring {
				r.ring[i] = int8(scale8(int32(v), 256, a8))
			}
			r.rebuildSigns()
		}
	}
	r.dcHz -= d
	r.tm.s1 -= d
	r.tm.s2 -= d
	r.tm.s0 -= d
	r.tm.s3 -= d
	r.tm.last -= d
	r.tm.dc -= d
	if !r.tm.mm {
		r.tm.half -= d
	}
}

// syncErrors returns the number of bits where the sliced symbols of the
// SYNC field that ends at symbol end differ from pattern p.
//
// It is not inlined, so that its locals are on the stack only while it
// runs.
//
//go:noinline
func (r *leanCore) syncErrors(end int64, p enums.SyncPattern) int {
	// The slicer thresholds at dcHz and ±2 steps, in ring units, so that
	// each symbol takes compares and no division.
	step := r.devScale * deviationHz
	t0 := (r.dcHz - r.off) / discStepHz
	hi, lo := t0+2*step/discStepHz, t0-2*step/discStepHz
	n := 0
	j := uint32(end-syncSymbols+1) % leanRing
	for k := range syncSymbols {
		q := float32(r.ring[j])
		if j++; j == leanRing {
			j = 0
		}
		// SYNC symbols are ±3: a symbol of the same sign off by one level
		// and one of the other sign beyond it is one bit error, the other
		// level of the other sign two.
		want := uint64(p)>>(2*(syncSymbols-1-k))&2 == 0
		switch {
		case q >= hi:
			if !want {
				n++
			}
		case q >= t0:
			if want {
				n++
			} else {
				n += 2
			}
		case q >= lo:
			if want {
				n += 2
			} else {
				n++
			}
		default:
			if want {
				n++
			}
		}
	}
	return n
}

// unlock ends the stream.
//
// It is not inlined, so that its locals are on the stack only while it
// runs.
//
//go:noinline
func (r *leanCore) unlock() {
	if r.ext != nil {
		r.ext.end(r)
	}
	r.locked, r.forced, r.hunt = false, false, false
	r.acqFrom = r.n
	if a := r.audio; a != nil {
		// The input summed before the lock is of the signal before it.
		a.sum, a.sq, a.n = 0, 0, 0
	}
	r.rebuildSigns()
	r.tm.hold, r.tm.gain = false, r.acqGain
	if r.tm.mm {
		// The next strobe is a symbol, so the Gardner loop takes the
		// midpoint before it.
		r.tm.mm, r.tm.mid = false, true
		r.tm.t -= r.tm.step
	}
	r.offsetHz = int16(max32(min32(roundf(r.tuneHz+r.dcHz), math.MaxInt16), math.MinInt16))
	r.devHz = int16(max32(min32(roundf(3*deviationHz*r.devScale), math.MaxInt16), math.MinInt16))
}

// rebuildSigns sets ringSum and signs from the last symbols of the ring.
func (r *leanCore) rebuildSigns() {
	r.ringSum, r.signs = 0, 0
	for k := int64(syncSymbols); k > 0; k-- {
		r.ringSum += int32(r.ring[uint32(r.ns-k)%leanRing])
	}
	for k := int64(syncSymbols); k > 0; k-- {
		r.signs <<= 1
		if int32(r.ring[uint32(r.ns-k)%leanRing])*syncSymbols > r.ringSum {
			r.signs |= 1
		}
	}
}

// symLLR returns the log-likelihood ratios of the two bits of symbol k for
// the soft decoders: in units of 1/8, rounded to an even number, saturated
// at 254. A ratio of 0, as a symbol gets when both levels of a pair are at
// the outlier floor, becomes ±1 with the sign of the hard decision that
// burst makes: the decoders take the hard decisions from the signs, so
// that they start from the bits of Data, and ±1 is below every other
// magnitude, so that such a bit still counts as the least reliable.
func (r *leanCore) symLLR(k int64) (int16, int16) {
	// x is the symbol less the DC offset in units of llrStepHz. The
	// log-likelihood of level 2d-3 is max(e*e*ll, lfloor), with e the
	// distance of x to it, and each ratio is the best of two levels less
	// the best of the other two. By the symmetry of the levels, the best
	// of each pair is the nearer level, and three distances cover them.
	x := int32(r.ring[uint32(k)%leanRing])*(discStepHz/llrStepHz) + r.llrOff
	ax := x
	if ax < 0 {
		ax = -ax
	}
	d := r.llrDev
	near, far, opp := r.llrTerm(ax-d), r.llrTerm(ax-3*d), r.llrTerm(ax+d)
	same := near
	if ax > 2*d {
		same = far
	}
	b0 := same - opp
	if x < 0 {
		b0 = -b0
	}
	l0, l1 := evenLLR(b0), evenLLR(near-far)
	if l0 == 0 {
		l0 = 1
		if x < 0 {
			l0 = -1
		}
	}
	if l1 == 0 {
		l1 = 1
		if ax > 2*d {
			l1 = -1
		}
	}
	return l0, l1
}

// llrTerm returns the log-likelihood, in units of 1/8, of a symbol at
// distance e, in units of llrStepHz, from a level: max(e*e*ll, lfloor).
func (r *leanCore) llrTerm(e int32) int32 {
	return max(-int32(int64(e*e)*r.llrK>>llrShift), r.llrFloor)
}

// evenLLR returns the log-likelihood ratio v, in units of 1/8, rounded to
// an even value and limited to ±254.
func evenLLR(v int32) int16 {
	h := (v + 1) >> 1
	if v < 0 {
		h = -((1 - v) >> 1)
	}
	return int16(max(min(h, 127), -127)) * 2
}

// setLLR sets the fixed point settings of symLLR from dcHz, off, dev, ll
// and lfloor.
func (r *leanCore) setLLR() {
	r.llrOff = int32(roundf((r.off - r.dcHz) / llrStepHz))
	r.llrDev = int32(roundf(r.dev / llrStepHz))
	// ll is at least -1/2, so int32 holds llrK, and the conversion needs
	// no 64-bit helper in TinyGo.
	r.llrK = int64(int32(-r.ll*8*llrStepHz*llrStepHz*(1<<llrShift) + 0.5))
	r.llrFloor = math.MinInt32 / 2
	if r.lfloor >= -math.MaxFloat32 {
		r.llrFloor = int32(roundf(r.lfloor * 8))
	}
}

// llrAt returns the log-likelihood ratio of burst bit p of the slot at
// r.base.
func (r *leanCore) llrAt(p int) int16 {
	l0, l1 := r.symLLR(r.base + cachSymbols + int64(p>>1))
	if p&1 == 0 {
		return l0
	}
	return l1
}

// burst detects the burst of slot j, whose first symbol is base and whose
// SYNC is anchor, an anchor pattern or 0, places it and emits it.
//
// It is not inlined, so that its locals are on the stack only while it
// runs.
//
//go:noinline
func (r *leanCore) burst(j int32, base int64, anchor enums.SyncPattern) {
	r.base, r.anchor = base, anchor
	v := max(r.discVar, 1)
	r.ll = -1 / (2 * v)
	r.dev = r.devScale * deviationHz
	r.lfloor = float32(math.Inf(-1))
	if r.outlier > 0 {
		r.lfloor = r.floorC + 0.5*log32(v)
	}
	r.setLLR()
	b := &r.cb
	// Field by field: the hard decisions below write all of Data and
	// CACH, and TinyGo clears a whole Burst with a slow memset.
	b.Distance, b.BS, b.HasCACH, b.Stream, b.Slot = -1, r.bs, r.bs, int(r.streams), int64(j)
	b.Sync, b.Voice, b.HasFixed = 0, 0, false
	b.Fixed, b.FixedCACH = layer2.RawBurst{}, [3]byte{}
	// The time in 1/256 baseband samples, in integers but for the place of
	// the last symbol, so that the MCU build needs one float64 multiply.
	ticks := (r.mfStep*(r.n-1)+r.center-(r.ns-1-(base+cachSymbols))*samplesPerSy)<<8 +
		int64(int32(roundf(float32(int32(r.mfStep))*r.lastAt*256)))
	b.Seconds = float64(ticks) * (1.0 / (basebandRate * 256))
	// The hard decisions are the signs of the ratios of symLLR: the first
	// bit is set below 0, the second beyond the midpoint between the
	// levels, ±2 dev.
	thr, out := 2*r.llrDev, r.llrDev*leanClickOut/2
	h := uint32(base) % leanRing
	var acc byte
	beyond := 0
	for k := range slotSymbols {
		x := int32(r.ring[h])*(discStepHz/llrStepHz) + r.llrOff
		if h++; h == leanRing {
			h = 0
		}
		acc <<= 2
		if x < 0 {
			acc |= 2
			x = -x
		}
		if x > thr {
			acc |= 1
		}
		if x > out {
			beyond++
		}
		// Four symbols fill a byte; the CACH is three bytes.
		switch {
		case k&3 != 3:
		case k < cachSymbols:
			b.CACH[k/4] = acc
		default:
			b.Data[(k-cachSymbols)/4] = acc
		}
	}
	if r.clickSpan > 0 {
		r.clickFloor(beyond, v)
	}
	if !r.classify(b, j) && (r.ext == nil || !r.ext.place(r, b, j)) {
		if r.ext != nil && !r.replayDry {
			r.ext.unplaced(r, b)
		}
		return
	}
	if r.ext != nil {
		if !r.replayDry {
			r.ext.burst(r, b)
		}
		return
	}
	if r.softFEC {
		cc, ok := r.colorCode()
		r.repair(&r.fec, b, cc, ok)
	}
	r.emit(b)
}

// emit hands burst b to onBurst, or appends it to out while it has room.
func (r *leanCore) emit(b *Burst) {
	if r.onBurst != nil {
		r.onBurst(b)
	} else if len(r.out) < cap(r.out) {
		r.out = r.out[:len(r.out)+1]
		r.out[len(r.out)-1] = *b
	}
}

// clickFloor sets the floor of the symbol likelihoods from the share of
// clicks: beyond symbols of the slot lie beyond the outer levels, and v is
// the noise variance. Below the FM threshold the discriminator turns a
// slip of the carrier phase into a pulse that moves a symbol by more than
// the span of the levels, so most clicks land beyond them, and their share
// measures the density of the clicks that land between them. A fixed
// floor makes the ratios of a weak signal too confident there: the error
// rate of a large ratio then rises instead of falling.
func (r *leanCore) clickFloor(beyond int, v float32) {
	r.clicks += leanClickGain * (float32(beyond)/slotSymbols - r.clicks)
	po := max(r.clicks, leanMinClicks)
	lf := log32(po/r.clickSpan) - log32(1-po) + 0.5*log32(2*math.Pi*v)
	r.llrFloor = int32(roundf(lf * 8))
}

// colorCode returns the color code most clean data bursts carried.
func (r *leanCore) colorCode() (enums.ColorCode, bool) {
	best := 0
	for i, v := range &r.votes {
		if v > r.votes[best] {
			best = i
		}
	}
	return enums.ColorCode(best), r.votes[best] > 0
}

// classify places burst b of slot j by its SYNC and the superframe of its
// channel, as tracker.classify does but without later slots: a burst B to
// F without a burst A before it is placed by its EMB alone. It reports
// whether the burst is kept.
//
// It is not inlined, so that its locals are on the stack only while it
// runs.
//
//go:noinline
func (r *leanCore) classify(b *Burst, j int32) bool {
	c := &r.ch[j&1]
	voice, data, _ := sourceAt(int(r.src)).SyncPatterns()
	dv, dd := syncDist(b.Data, voice), syncDist(b.Data, data)
	switch {
	case r.anchor == data || r.anchor == 0 && dd <= damagedSyncDistance && dd < dv:
		b.Distance, b.Sync = dd, data
		c.sf = leanNoSlot
		r.noteData(b, j, c)
		return true
	case r.anchor == enums.SyncMSSourcedRC:
		b.Distance, b.Sync = syncDist(b.Data, enums.SyncMSSourcedRC), enums.SyncMSSourcedRC
		return true
	case r.anchor == voice || dv <= layer2.DefaultSyncThreshold:
		b.Distance, b.Sync = dv, voice
		c.sf = j
		return true
	}
	if c.sf != leanNoSlot {
		switch n := (j - c.sf) / 2; {
		case n >= 1 && n <= 5 && (j-c.sf)%2 == 0:
			b.Voice = enums.VoiceBurstA + enums.VoiceBurstType(n)
			if !r.replaying {
				// The EMB tests the timing of a lock as a SYNC does.
				ok := r.embOK(b)
				r.suspect = r.suspect || !ok
				r.forced = r.forced && !ok
			}
			return true
		case n == 6 && dv <= voiceSyncDistance:
			b.Distance, b.Sync, b.Voice = dv, voice, enums.VoiceBurstA
			c.sf = j
			return true
		}
		c.sf = leanNoSlot
	}
	if c.hdr == j-2 && dv <= voiceSyncDistance || dv <= damagedSyncDistance {
		b.Distance, b.Sync, b.Voice = dv, voice, enums.VoiceBurstA
		c.sf = j
		return true
	}
	if pos, ok := r.embPosition(b); ok {
		b.Voice = pos
		c.sf = j - 2*int32(pos-enums.VoiceBurstA)
		return true
	}
	if dd <= damagedSyncDistance {
		b.Distance, b.Sync = dd, data
		return true
	}
	return false
}

// embPosition returns the superframe position of burst b, which has no
// SYNC, when its EMB decodes with at most one correction and the color code
// of the stream: B for the first fragment of the embedded LC and E for the
// last.
func (r *leanCore) embPosition(b *Burst) (enums.VoiceBurstType, bool) {
	cc, ok := r.colorCode()
	if !ok {
		return 0, false
	}
	emb := b.Data.EMB()
	if emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > 1 || emb.ColorCode != cc {
		return 0, false
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		return enums.VoiceBurstB, true
	case enums.LCSSLastFragment:
		return enums.VoiceBurstE, true
	default:
		return 0, false
	}
}

// embOK reports whether the EMB of burst b decodes, with the color code
// of the stream when it is known.
func (r *leanCore) embOK(b *Burst) bool {
	emb := b.Data.EMB()
	cc, ok := r.colorCode()
	return !emb.FEC.Uncorrectable && (!ok || emb.ColorCode == cc)
}

// noteData records the color code of data burst b when its slot type has
// at most leanVoteErrors errors, and the slot j of a voice LC or PI
// header.
func (r *leanCore) noteData(b *Burst, j int32, c *leanChan) {
	st := b.Data.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	if st.FEC.ErrorsCorrected <= leanVoteErrors && r.votes[st.ColorCode&0xF] < math.MaxUint8 {
		r.votes[st.ColorCode&0xF]++
	}
	if dt := st.DataType; dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader {
		c.hdr = j
	}
}
