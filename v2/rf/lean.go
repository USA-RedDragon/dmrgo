package rf

import (
	"math"
	"math/bits"

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
	leanSignMiss = 6
	// llrStepHz is the resolution of the symbols in symLLR, and llrShift
	// the fixed point of its likelihood scale.
	llrStepHz = 4
	llrShift  = 24
	// leanCubicSPS is the most samples per symbol at which the symbol
	// timing interpolates by Catmull-Rom instead of linearly.
	leanCubicSPS = 2.5
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

// leanTuner is the front end of a leanCore: it moves the carrier that it
// removes to hz, d Hz from the last, and adds -d to the discriminator
// samples that it holds.
type leanTuner interface {
	retuneFront(hz, d float32)
}

// leanCore is the streaming receiver after the discriminator: it takes the
// matched filter samples at discSPS samples per symbol, keeps the symbols of
// one slot and the state of the stream, and decodes each burst as its slot
// ends, without waiting for later slots.
type leanCore struct {
	onBurst func(Burst)
	out     []Burst
	front   leanTuner
	// center is the input sample at the center of matched filter sample 0,
	// in baseband samples, and mfScale the baseband samples per matched
	// filter sample.
	center  int64
	mfScale float64
	// acqGain and trackGain are leanAcqGain and leanTrackGain for the
	// samples per symbol of the matched filter.
	acqGain, trackGain float32
	tm                 leanTiming
	// n counts the matched filter samples and ns the symbols. ring holds
	// the last symbols in units of discStepHz, plus off.
	n    int64
	ns   int64
	ring [leanRing]int8
	off  float32
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

	locked  bool
	softFEC bool
	src     uint8
	sources uint8
	bs      bool
	// first is the symbol of the start of slot 0 of the stream, next the
	// slot that ends next, lastAnch the last slot with a SYNC, lost the
	// slots since the last slot with carrier.
	first    int64
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

	offsetHz int16
	devHz    int16
	// cb holds the burst under detection and fec the scratch of its
	// decode, which as locals TinyGo puts on the heap.
	cb  Burst
	fec leanFEC
}

// leanRx is the receiver of TierDisc without lookback: the complex front
// end leanFront before a leanCore.
type leanRx struct {
	leanCore
	// fe and wide convert the bytes of Write at another rate than the
	// baseband, and dc and dcN remove the DC offset of the bytes of Write
	// at the baseband rate.
	fe      *frontEnd
	wide    []complex64
	inChunk int
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
	r.leanCore.init(cfg.OnBurst, opt, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+16, discSPS)
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Signed, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		r.inChunk = max(2*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), 2)
	}
	r.df.init(cfg.BasebandHz == discRate)
	r.front, r.center = &r.df, r.df.center()
	return r
}

// init sets up c for matched filter samples at sps samples per symbol with
// the settings opt, to emit bursts to onBurst, or, when it is nil, to hold
// up to keep bursts for the methods to return.
func (c *leanCore) init(onBurst func(Burst), opt options, keep int, sps float32) {
	c.onBurst, c.lastAnch, c.softFEC = onBurst, leanNoSlot, opt.SoftFEC
	c.sources, c.outlier = uint8(opt.Sources), float32(opt.DiscOutlier)
	if po := opt.DiscOutlier; po > 0 {
		c.floorC = float32(math.Log(po/basebandRate) - math.Log(1-po) + 0.5*math.Log(2*math.Pi))
	}
	c.mfScale = float64(samplesPerSy) / float64(sps)
	c.acqGain, c.trackGain = leanAcqGain*(sps/discSPS), leanTrackGain*(sps/discSPS)
	c.tm.step = sps / 2
	c.tm.cubic = sps <= leanCubicSPS
	c.tm.t, c.tm.gain = c.tm.step, c.acqGain
	c.resetChans()
	if onBurst == nil {
		c.out = make([]Burst, 0, keep)
	}
}

// resetChans clears the superframe state of the stream.
func (r *leanCore) resetChans() {
	for i := range r.ch {
		r.ch[i] = leanChan{sf: leanNoSlot, hdr: leanNoSlot}
	}
	r.votes = [16]uint8{}
}

// write processes the I/Q bytes of Receiver.Write.
func (r *leanRx) write(iq []byte) []Burst {
	r.out = r.out[:0]
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
		return r.out
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
	return r.out
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
	r.push(s)
}

// power counts one matched filter sample, whose prefilter power is p.
func (r *leanCore) power(p float32) {
	r.n++
	r.eAcc += p
	r.eSyms++
}

// push runs matched filter sample s through the symbol timing and handles
// each symbol.
func (r *leanCore) push(s float32) {
	if x, at, ok := r.tm.push(s); ok {
		r.symbol(x, at)
	}
}

// symbol handles symbol value x, at samples after the last matched filter
// sample. It is not inlined, so that the code of every sample stays small.
//
//go:noinline
func (r *leanCore) symbol(x, at float32) {
	r.lastAt = at
	q := sat8((x - r.off) / discStepHz)
	r.ring[uint32(r.ns)%leanRing] = q
	r.ringSum += int32(q) - int32(r.ring[uint32(r.ns-syncSymbols+leanRing)%leanRing])
	r.signs <<= 1
	if int32(q)*syncSymbols > r.ringSum {
		r.signs |= 1
	}
	r.ns++
	if !r.locked {
		r.searchSym()
		return
	}
	if r.ns == r.slotEnd {
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
	var mx, xx float32
	var sxy [numSources]float32
	for k := range syncSymbols {
		v := r.sym(end - syncSymbols + 1 + int64(k))
		mx += v
		xx += v * v
		for i := range r.sources {
			sxy[i] += syncDev[i][k] * v
		}
	}
	sxx := xx - mx*mx/syncSymbols
	if sxx <= 0 {
		return 0, 0, false
	}
	var best float32
	for i := range int(r.sources) {
		if q := sxy[i] * sxy[i] / syncDevSq[i]; q > best {
			best, src = q, i
		}
	}
	return best / sxx, src, sxy[src] < 0
}

// signsMatch reports whether the signs of the last syncSymbols symbols
// differ from the voice or the data SYNC of a source in at most
// leanSignMiss symbols.
func (r *leanCore) signsMatch() bool {
	for i := range r.sources {
		m := bits.OnesCount32((r.signs ^ syncSigns[i]) & (1<<syncSymbols - 1))
		if m <= leanSignMiss || m >= syncSymbols-leanSignMiss {
			return true
		}
	}
	return false
}

// searchSym tests whether the last symbols end a SYNC and locks to it.
//
//go:noinline
func (r *leanCore) searchSym() {
	if r.ns < leanRing || !r.signsMatch() {
		return
	}
	rho2, src, data := r.syncAt(r.ns - 1)
	if rho2 < discLock*discLock {
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	if !r.fitLevels(r.ns-1, p, true) {
		return
	}
	r.locked, r.src, r.bs = true, uint8(src), sourceAt(src) == layer2.SyncSourceBS
	r.streams++
	r.first = r.ns - 1 - leanSyncEnd
	r.next, r.lastAnch, r.lost = 0, leanNoSlot, 0
	r.slotEnd = r.first + slotSymbols
	r.tm.hold, r.tm.gain = false, r.trackGain
	r.resetChans()
	r.eAcc, r.eSyms = 0, 0
}

// fitLevels fits the discriminator offset, the deviation scale and the
// noise variance to the SYNC field that ends at symbol end with pattern p,
// and reports whether they make sense for the first anchor of a stream
// when first.
func (r *leanCore) fitLevels(end int64, p enums.SyncPattern, first bool) bool {
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
		if g <= 0 || sqrtf(res) > discFirstSpread*2*g*deviationHz {
			return false
		}
		r.devScale, r.dcHz, r.discVar = g, dc, res
		return true
	}
	r.devScale = levelForget*r.devScale + (1-levelForget)*g
	r.dcHz = levelForget*r.dcHz + (1-levelForget)*dc
	r.discVar = levelForget*r.discVar + (1-levelForget)*res
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
	isAnchor := best >= discAnchor*discAnchor
	if isAnchor && r.syncErrors(base+leanSyncEnd+shift, anchor) > discAnchorBits {
		isAnchor = false
	}
	if isAnchor {
		r.fitLevels(base+leanSyncEnd+shift, anchor, false)
		if abs32(r.dcHz) > leanRetuneHz {
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
	}
	present := isAnchor || r.noise == 0 || energy > 1.5*r.noise
	r.tm.hold = !r.bs && !present
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
	if r.lost >= lostSlots || r.lastAnch == leanNoSlot && j >= 6 {
		r.unlock()
	}
}

// retune moves the oscillator by d Hz and the discriminator values that
// the receiver holds by -d, so that they read as if the oscillator had
// been there.
func (r *leanCore) retune(d float32) {
	r.tuneHz += d
	r.front.retuneFront(r.tuneHz, d)
	r.off -= d
	r.dcHz -= d
	r.tm.s1 -= d
	r.tm.s2 -= d
	r.tm.s0 -= d
	r.tm.s3 -= d
	r.tm.last -= d
	r.tm.half -= d
}

// syncErrors returns the number of bits where the sliced symbols of the
// SYNC field that ends at symbol end differ from pattern p.
func (r *leanCore) syncErrors(end int64, p enums.SyncPattern) int {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	step := r.devScale * deviationHz
	n := 0
	for k, want := range syms {
		if got := sliceSymbol(float64((r.sym(end-syncSymbols+1+int64(k)) - r.dcHz) / step)); got != want {
			n += symbolBitErrors(got, want)
		}
	}
	return n
}

// unlock ends the stream.
func (r *leanCore) unlock() {
	r.locked = false
	r.tm.hold, r.tm.gain = false, r.acqGain
	r.offsetHz = int16(max32(min32(roundf(r.tuneHz+r.dcHz), math.MaxInt16), math.MinInt16))
	r.devHz = int16(max32(min32(roundf(3*deviationHz*r.devScale), math.MaxInt16), math.MinInt16))
}

// symLLR returns the log-likelihood ratios of the two bits of symbol k, as
// discDetect and llr8 make them for the soft decoders: in units of 1/8,
// rounded to an even number, saturated at 254.
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
	return evenLLR(b0), evenLLR(near - far)
}

// llrTerm returns the log-likelihood, in units of 1/8, of a symbol at
// distance e, in units of llrStepHz, from a level: max(e*e*ll, lfloor).
func (r *leanCore) llrTerm(e int32) int32 {
	return max(-int32(int64(e*e)*r.llrK>>llrShift), r.llrFloor)
}

// evenLLR returns the log-likelihood ratio v, in units of 1/8, as even8
// stores it.
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
	r.llrK = int64(-r.ll*8*llrStepHz*llrStepHz*(1<<llrShift) + 0.5)
	r.llrFloor = math.MinInt32 / 2
	if !math.IsInf(float64(r.lfloor), -1) {
		r.llrFloor = int32(roundf(r.lfloor * 8))
	}
}

// even8 rounds an LLR in units of 1/8 as llr8 stores it.
func even8(x int16) int16 {
	h := (int32(x) + 1) >> 1
	if x < 0 {
		h = -((1 - int32(x)) >> 1)
	}
	return int16(max(min(h, 127), -127)) * 2
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

// cachAt returns the log-likelihood ratio of CACH bit p of the slot at
// r.base.
func (r *leanCore) cachAt(p int) int16 {
	l0, l1 := r.symLLR(r.base + int64(p>>1))
	if p&1 == 0 {
		return l0
	}
	return l1
}

// burst detects the burst of slot j, whose first symbol is base and whose
// SYNC is anchor, an anchor pattern or 0, places it and emits it.
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
	*b = Burst{Distance: -1, BS: r.bs, HasCACH: r.bs, Stream: int(r.streams), Slot: int64(j)}
	now := r.mfScale*(float64(r.n-1)+float64(r.lastAt)) + float64(r.center)
	b.Seconds = (now - float64(r.ns-1-(base+cachSymbols))*float64(samplesPerSy)) / basebandRate
	// The hard decisions are the signs of the ratios of symLLR: the first
	// bit is set below 0, the second beyond the midpoint between the
	// levels, ±2 dev.
	thr := 2 * r.llrDev
	for k := range slotSymbols {
		x := int32(r.ring[uint32(base+int64(k))%leanRing])*(discStepHz/llrStepHz) + r.llrOff
		var d byte
		if x < 0 {
			d = 2
			x = -x
		}
		if x > thr {
			d |= 1
		}
		if k < cachSymbols {
			b.CACH[k/4] |= d << (6 - 2*(k%4))
		} else {
			i := 2 * (k - cachSymbols)
			b.Data[i/8] |= d << (6 - i%8)
		}
	}
	if !r.classify(b, j) {
		return
	}
	if r.softFEC {
		cc, ok := r.colorCode()
		r.repair(&r.fec, b, cc, ok)
	}
	if r.onBurst != nil {
		r.onBurst(*b)
	} else if len(r.out) < cap(r.out) {
		r.out = r.out[:len(r.out)+1]
		r.out[len(r.out)-1] = *b
	}
}

// colorCode returns the color code most clean data bursts carried.
func (r *leanCore) colorCode() (enums.ColorCode, bool) {
	best := 0
	for i, v := range r.votes {
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

// noteData records the color code of data burst b when it is clean, and
// the slot j of a voice LC or PI header.
func (r *leanCore) noteData(b *Burst, j int32, c *leanChan) {
	st := b.Data.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	if st.FEC.ErrorsCorrected == 0 && r.votes[st.ColorCode&0xF] < math.MaxUint8 {
		r.votes[st.ColorCode&0xF]++
	}
	if dt := st.DataType; dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader {
		c.hdr = j
	}
}
