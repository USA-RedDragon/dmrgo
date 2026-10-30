package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	discAnchor          = 0.6
	levelForget         = 0.8
	discAnchorBits      = 12
	discFirstSpread     = 0.35
	leanRing            = slotSymbols + 2
	leanChunk           = 6
	leanSyncEnd         = syncFirstSym + syncSymbols - 1
	leanAcqSymbols      = leanSyncEnd + 1 - cachSymbols
	leanReplaySymbols   = leanSyncEnd + 1
	leanAcqSamples      = (leanReplaySymbols*5/2 + 7 + 2) &^ 1
	leanResample        = 0.7
	leanCoarse          = 0.75
	leanOverScale       = 2
	leanMinScale        = 0.1
	leanAcqWait         = 48
	leanVoteErrors      = 2
	leanSyncWindow      = (syncSymbols-1)*5/2 + 3 + 6
	leanHuntSamples     = (leanAcqSymbols*5/2 + 7) &^ 1
	leanAcqGain         = 0.15
	leanTrackGain       = 0.02
	leanPowerForget     = 0.99
	leanRetuneHz        = 100
	leanBlock           = slotSymbols * discSPS / 8
	leanFloorRise       = 1.01
	leanPresent         = 2
	leanSearchHz        = 200
	leanSignMiss        = 5
	leanExtSignMiss     = 9
	leanExtRho          = 0.3
	llrStepHz           = 4
	llrShift            = 24
	leanClickGain       = 0.1
	leanMinClicks       = 1e-3
	leanClickOut        = 9
	leanNoCarrierClicks = 36
	leanNoCarrierSpread = 1.2
	leanGapSpread       = 4
	leanGapFloor        = 0.9
	leanGapClicks       = 16
	leanCubicSPS        = 2.5
	leanSixSPS          = 2
	leanMMSPS           = 2.5
	leanMMSign          = 1
	leanMMGain          = 2
	leanClockGain       = 0.0002
	leanClockMax        = 0.002
	leanStretchMin      = 0.0002
	replayFlatLevel     = 0.3
	replaySyncRho       = 0.95
	leanClockDead       = 0.000075
	leanClockBlock      = 64
	leanClockSig        = 4
	leanClockSpan       = 4
	leanClockLag        = 120
	leanMidGate         = 0
	leanSyncUnroll      = true
	leanRawFuse         = true
	leanHuntMid         = true
	leanHuntFuse        = true
)

const (
	mid96Cubic = iota
	mid96Linear
	mid96Raw
	mid96RawLock
	mid96Tab = 4
)

type leanTiming struct {
	step           float32
	s1, s2         float32
	sa, sb, s0, s3 float32
	t              float32
	last           float32
	half           float32
	pow            float32
	gain           float32
	dc, dev        float32
	cubic, six     bool
	mid            bool
	hold           bool
	mm             bool
	rate, rateK    float32
	drift, corr    float32
	corr2          float32
	nc, nall       int32
	lastStep       float32
	acorr, acorr2  float32
	anc, anall     int32
	aweak          float32
}

func (tm *leanTiming) push(s float32) (v, at float32, ok bool) {
	if tm.cubic {
		tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3 = tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, s
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
		at--
	}
	return v, at, ok
}

func (tm *leanTiming) settle() {
	if tm.nc < leanClockBlock {
		return
	}
	n := float32(tm.nc)
	step := tm.rateK * tm.corr
	if v := tm.corr2/n - tm.corr*tm.corr/(n*n); tm.rateK > 0 && tm.corr*tm.corr > leanClockSig*leanClockSig*n*v {
		step = (tm.corr - tm.lastStep*leanClockLag) / float32(tm.nall)
		tm.acorr, tm.acorr2, tm.anc, tm.anall, tm.aweak = 0, 0, 0, 0, 0
	} else if tm.rateK > 0 {
		tm.acorr, tm.acorr2, tm.anc, tm.anall, tm.aweak = tm.acorr+tm.corr, tm.acorr2+tm.corr2, tm.anc+tm.nc, tm.anall+tm.nall, tm.aweak+step
		an := float32(tm.anc)
		switch v := tm.acorr2/an - tm.acorr*tm.acorr/(an*an); {
		case tm.acorr*tm.acorr > leanClockSig*leanClockSig*an*v:
			step = tm.acorr/float32(tm.anall) - (tm.aweak - step)
			tm.acorr, tm.acorr2, tm.anc, tm.anall, tm.aweak = 0, 0, 0, 0, 0
		case tm.anc >= leanClockSpan*slotSymbols:
			tm.acorr, tm.acorr2, tm.anc, tm.anall, tm.aweak = 0, 0, 0, 0, 0
		}
	}
	lim := leanClockMax * 2 * tm.step
	old := tm.rate
	tm.rate = max32(min32(tm.rate+step, lim), -lim)
	tm.lastStep = tm.rate - old
	tm.corr, tm.corr2, tm.nc, tm.nall, tm.drift = 0, 0, 0, 0, tm.rate
	if abs32(tm.rate) < leanClockDead*2*tm.step {
		tm.drift = 0
	}
}

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
		tm.t += 2*tm.step + tm.drift
		tm.nall++
		if !tm.hold && tm.pow > 0 {
			c := max32(min32(leanMMGain*tm.gain*e/tm.pow, 0.5), -0.5)
			tm.t += c
			tm.corr, tm.corr2, tm.nc = tm.corr+c, tm.corr2+c*c, tm.nc+1
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

func (tm *leanTiming) at(t float32) float32 {
	if !tm.cubic {
		return tm.s2 + t*(tm.s2-tm.s1)
	}
	return cubicAt(tm.six, tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, 1+t)
}

func cubicAt(six bool, a, b, p0, p1, p2, p3, mu float32) float32 {
	if six {
		return lagrange6[site1](a, b, p0, p1, p2, p3, mu)
	}
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0)))
}

//go:section .text.linearAt
func (tm *leanTiming) linearAt(t float32) float32 {
	switch {
	case t >= -1:
		return tm.s3 + t*(tm.s3-tm.s2)
	case t >= -2:
		return tm.s2 + (t+1)*(tm.s2-tm.s1)
	}
	return tm.s1 + (t+2)*(tm.s1-tm.s0)
}

func (tm *leanTiming) midAt(t float32) float32 {
	if tm.six && t >= -3 && t <= -2 {
		return lagrange6[site1](tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, t+2)
	}
	return tm.linearAt(t)
}

func lagrange6[_ any](a, b, p0, p1, p2, p3, mu float32) float32 {
	d0, d1, d2, d3, d4, d5 := mu+3, mu+2, mu+1, mu, mu-1, mu-2
	l01 := d0 * d1
	l012 := l01 * d2
	r45 := d4 * d5
	r345 := d3 * r45
	return (-a*(d1*d2*r345)+5*b*(d0*d2*r345))*(1.0/120) +
		(-p0*(l01*r345)+p1*(l012*r45))*(1.0/12) +
		(-p2*(l012*d3*d5)+p3*(l012*d3*d4)*(1.0/5))*(1.0/24)
}

type leanChan struct {
	sf   int32
	hdr  int32
	data bool
}

const leanNoSlot = math.MinInt32

type leanTuner interface {
	retuneFront(hz, d float32)
}

type leanCore struct {
	center   int64
	mfStep   int64
	n        int64
	first    int64
	lockNs   int64
	base     int64
	anchor   enums.SyncPattern
	onBurst  func(*Burst)
	iq       leanTuner
	ext      leanExt
	sink     *burstSink
	audio    *audioFront
	tm       leanTiming
	ns       int64
	ring     [leanRing]int8
	head     uint8
	locked   bool
	acqHead  int32
	acqFrom  int64
	acqLow   int16
	offsetHz int16
	off      float32
	acqNext  int64
	slotEnd  int64
	ringSum  int32
	signs    uint32
	cb       Burst

	acqGain        float32
	trackGain      float32
	tuneHz         float32
	tunePrev       float32
	tuneNs         uint32
	lastAt         float32
	next           int32
	lastAnch       int32
	lost           int32
	lagTicks       int32
	dcHz           float32
	devScale       float32
	fitHz          float32
	discVar        float32
	amp            float32
	noise          float32
	eAcc           float32
	eSyms          int32
	ch             [2]leanChan
	streams        int32
	midSigns       uint32
	ll             float32
	lfloor         float32
	floorC         float32
	llrOff, llrDev int32
	llrFloor       int32
	llrK           int32
	dev            float32
	outlier        float32
	clickSpan      float32
	clicks         float32
	fec            leanFEC
	huntBuf        [leanHuntSamples / 2]int16
	devHz          int16
	devAtHz        int16

	signMiss                           uint8
	suspectRun, bsSuspectRun           uint8
	suspect, hunt                      bool
	dataAnch                           [2]bool
	again, midTiming, front96, slotDue bool
	mid96                              uint8
	replaying, replayDry               bool
	iqLow, linHunt                     bool
	mmOK                               bool
	chase                              uint8
	forced                             bool
	softFEC                            bool
	src                                uint8
	sources                            uint8
	bs                                 bool
	votes                              [16]uint8
	ms                                 leanSlots
}

type leanLevels struct {
	devScale, dcHz, discVar, amp, noise float32
	fitHz                               float32
}

type leanSlots struct {
	anch  [2]int32
	t     [2]float32
	n, ns [2]uint16
	off   [2]int16
	lv    [2]leanLevels
	moved bool
	seek  bool
	retry bool
	late  bool
	in    uint8
	out   uint8
	fresh [2]uint8
	rate  float32
}

type leanRx struct {
	leanCore
	fe       *frontEnd
	wide     []complex64
	inChunk  int
	dc       dcBlock
	signed   bool
	df       leanFront
	bPow     float32
	bLag     complex64
	bN       int32
	floor    float32
	noiseCoh complex64
}

func (f *leanFront) retuneFront(hz, d float32) {
	f.turn(2 * math.Pi * d / discRate)
	f.tune(float64(hz))
	f.shift(-d)
}

func (f *leanFront) turn(w float32) {
	for r := range preTaps {
		j := (f.preHead - 1 - r + preTaps) % preTaps
		v := complex(float32(f.preHist[j][0]), float32(f.preHist[j][1])) * turnBy(w, r+1)
		f.preHist[j] = [2]int8{sat8(real(v)), sat8(imag(v))}
	}
	f.prev *= turnBy(w, preTaps-preTaps/2)
}

func newLean(cfg rxSetup, opt options) *leanRx {
	r := &leanRx{signed: cfg.Signed}
	r.dc.init(float64(cfg.BasebandHz))
	r.init(cfg.OnBurst, opt, float32(discSPS)/2)
	r.sink = cfg.sink
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Format, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		r.inChunk = max(r.fe.size*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), r.fe.size)
	}
	r.df.init(cfg.BasebandHz == discRate)
	r.iq, r.center = &r.df, r.df.center()
	r.lagTicks = r.df.lagTicks()
	if cfg.OffsetHz != 0 {
		r.retune(float32(cfg.OffsetHz))
	}
	if opt.SoftExt {
		r.ext = softExtOf(&opt, float32(discSPS)/2)
	}
	return r
}

func (r *leanCore) init(onBurst func(*Burst), opt options, sps float32) {
	r.onBurst, r.lastAnch, r.softFEC = onBurst, leanNoSlot, opt.SoftFEC
	r.sources, r.outlier = uint8(opt.Sources), float32(opt.DiscOutlier)
	r.clickSpan = float32(opt.ClickSpan)
	r.midTiming = opt.MidTiming && sps <= leanCubicSPS
	r.linHunt = opt.HuntLinMid
	r.chase = leanChase
	if opt.LeanChase > 0 {
		r.chase = min(opt.LeanChase, leanChase)
	}
	r.signMiss = leanSignMiss
	r.bsSuspectRun = opt.BSSuspectRun
	if po := opt.DiscOutlier; po > 0 {
		r.floorC = float32(math.Log(po/basebandRate) - math.Log(1-po) + 0.5*math.Log(2*math.Pi))
	}
	r.mfStep = int64(samplesPerSy / sps)
	r.acqGain, r.trackGain = leanAcqGain*(sps/discSPS), leanTrackGain*(sps/discSPS)
	r.tm.step = sps / 2
	r.tm.cubic, r.tm.six = sps <= leanCubicSPS, sps <= leanSixSPS || sps <= leanCubicSPS && !opt.SoftExt
	r.mmOK = sps <= leanMMSPS
	r.tm.t, r.tm.gain = r.tm.step, r.acqGain
	r.resetChans()
}

func (r *leanCore) resetChans() {
	for i := range r.ch {
		r.ch[i] = leanChan{sf: leanNoSlot, hdr: leanNoSlot}
	}
	r.dataAnch = [2]bool{}
	r.votes = [16]uint8{}
}

func (r *leanRx) write(iq []byte) {
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
		return
	}
	for len(iq) > 0 {
		n := min(len(iq), r.inChunk)
		n -= n % r.fe.size
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

func (r *leanRx) byteSample(i, q byte) complex64 {
	var v complex64
	if r.signed {
		v = complex(float32(int8(i))/127.5, float32(int8(q))/127.5)
	} else {
		v = complex((float32(i)-127.5)/127.5, (float32(q)-127.5)/127.5)
	}
	return r.dc.remove(v)
}

func (r *leanRx) writeBaseband(x []complex64) {
	for _, v := range x {
		r.sample(v)
	}
}

func (r *leanRx) flush() {
	for range slotSamplesN {
		r.sample(0)
	}
	if r.locked {
		r.unlock()
	}
}

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

func (r *leanCore) power(p float32) {
	r.n++
	r.eAcc += p
	r.eSyms++
}

func (r *leanCore) keep(s float32) {
	r.keepQ(int32(roundf(s * (1.0 / discStepHz))))
}

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

func (r *leanCore) acqBuf() ([]int16, int32) {
	if r.hasExt() {
		if b := r.ext.kept(); b != nil {
			return b, int32(2 * len(b))
		}
	}
	if r.locked {
		return r.huntBuf[:], leanHuntSamples
	}
	return r.fec.s[:leanAcqSamples/2], leanAcqSamples
}

func (r *leanCore) rescale(g float32) {
	g8 := int32(g*256 + 0.5)
	for i, v := range &r.ring {
		r.ring[i] = int8(scale8(int32(v), g8, 0))
	}
	r.rebuildSigns()
	r.rescaleKept(g8, 0)
	tm := &r.tm
	tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3 = tm.sa*g, tm.sb*g, tm.s0*g, tm.s1*g, tm.s2*g, tm.s3*g
	tm.last, tm.half, tm.pow = tm.last*g, tm.half*g, tm.pow*g*g
}

func (r *leanCore) rescaleKept(g8, a8 int32) {
	buf, _ := r.acqBuf()
	for i, w := range buf {
		lo, hi := scale8(int32(int8(uint8(w))), g8, a8), scale8(int32(int8(uint8(uint16(w)>>8))), g8, a8)
		buf[i] = int16(uint16(uint8(lo)) | uint16(uint8(hi))<<8)
	}
	r.acqLow = int16(scale8(int32(r.acqLow), g8, a8))
}

func scale8(x, g8, a8 int32) int32 {
	if x == math.MaxInt8 || x == math.MinInt8 {
		return x
	}
	return max(min((x*g8+a8+128)>>8, math.MaxInt8), math.MinInt8)
}

func (r *leanCore) acqKept() int32 {
	return acqKeptAt[site1](r)
}

func acqKeptAt[_ any](r *leanCore) int32 {
	_, an := r.acqBuf()
	return int32(min(r.n-r.acqFrom, int64(an)))
}

func (r *leanCore) kept(back int32) float32 {
	buf, an := r.acqBuf()
	h := r.acqHead - 1 - back
	if h < 0 {
		h += an
	}
	if h == r.acqHead-1 && h&1 == 0 {
		return float32(r.acqLow)
	}
	w := uint16(buf[h>>1])
	if h&1 != 0 {
		w >>= 8
	}
	return float32(int8(uint8(w)))
}

func (r *leanCore) keptAt(t float32) float32 {
	b := -t
	i := int32(b)
	mu := 1 - (b - float32(i))
	var p [4]float32
	for j := range int32(4) {
		b := i + 2 - j
		if j == 3 {
			b = max(b, 0)
		}
		p[j] = r.kept(b)
	}
	p0, p1, p2, p3 := p[0], p[1], p[2], p[3]
	if i+3 >= r.acqKept() {
		p0 = p1
	}
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))) - r.off*(1.0/discStepHz)
}

func coarseSync(win *syncWindow, syms *[syncSymbols]int8, end, sps float32, reach int32) bool {
	e, step := int32(-end*256+0.5), int32(sps*256+0.5)
	var best, bsv, bsvv int32
	sp := symSum(syms)
	for d := -reach; d <= reach; d++ {
		if end+float32(d) > 0 {
			break
		}
		c, sv, svv := win.coarseAt(syms, e-d*256, step)
		if c > best {
			best, bsv, bsvv = c, sv, svv
		}
	}
	if best <= 0 {
		return false
	}
	n := float32(syncSymbols)
	cov := float32(best) - float32(sp)*float32(bsv)/n
	vv := float32(bsvv) - float32(bsv)*float32(bsv)/n
	pp := 9*n - float32(sp)*float32(sp)/n
	return cov > 0 && cov*cov >= leanCoarse*leanCoarse*vv*pp
}

func symSum(syms *[syncSymbols]int8) int32 {
	var sp int32
	for _, s := range syms {
		sp += int32(s)
	}
	return sp
}

//go:noinline
func (win *syncWindow) coarseAt(syms *[syncSymbols]int8, e, step int32) (c, sv, svv int32) {
	for i, s := range syms {
		b := (e+int32(syncSymbols-1-i)*step+128)>>8 - win.b0
		v := int32(win.w[min(max(b, 0), leanSyncWindow-1)])
		c += int32(s) * v
		sv += v
		svv += v * v
	}
	return c, sv, svv
}

//go:noinline
func (r *leanCore) midNear(p enums.SyncPattern) bool {
	back := int32(r.tm.step - r.lastAt + 0.5)
	if back+2*syncSymbols > r.acqKept() {
		return true
	}
	var sv, svv, c, sp float32
	for k := range syncSymbols {
		v := r.kept(back + 2*int32(syncSymbols-1-k))
		s := float32(3)
		if uint64(p)>>(2*(syncSymbols-1-k))&2 != 0 {
			s = -3
		}
		c += s * v
		sv += v
		svv += v * v
		sp += s
	}
	const n = syncSymbols
	cov := c - sp*sv/n
	return cov > 0 && cov*cov >= leanMidGate*leanMidGate*(svv-sv*sv/n)*(9*n-sp*sp/n)
}

func (r *leanCore) fillWindow(win *syncWindow, b0 int32) {
	win.b0, win.off = b0, r.off*(1.0/discStepHz)
	buf, an := r.acqBuf()
	h := r.acqHead - 1 - b0
	for k := range win.w {
		if h < 0 {
			h += an
		}
		if h == r.acqHead-1 && h&1 == 0 {
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

type syncWindow struct {
	w   [leanSyncWindow]int8
	b0  int32
	off float32
}

func (win *syncWindow) at(t float32) float32 {
	return winAt[site1](win, t)
}

func winAt[_ any](win *syncWindow, t float32) float32 {
	b := -t
	i := int32(b)
	mu := 1 - (b - float32(i))
	k := min(max(i-win.b0, 0), leanSyncWindow-3)
	p0, p1, p2, p3 := float32(win.w[k+2]), float32(win.w[k+1]), float32(win.w[k]), float32(win.w[max(k-1, 0)])
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))) - win.off
}

//go:noinline
func (win *syncWindow) corr(syms *[syncSymbols]int8, t, sps float32) float32 {
	var c float32
	for i, s := range syms {
		c += float32(s) * win.at(t-float32(syncSymbols-1-i)*sps)
	}
	return c
}

func (win *syncWindow) sums(syms *[syncSymbols]int8, t, sps float32) (sv, svv, sxv, sp float32) {
	for i, s := range syms {
		v := win.at(t - float32(syncSymbols-1-i)*sps)
		sv += v
		svv += v * v
		sxv += float32(s) * v
		sp += float32(s)
	}
	return sv, svv, sxv, sp
}

const leanEdgeMargin = 9

func (win *syncWindow) peak(syms *[syncSymbols]int8, c *[7]float32, end, sps float32, wide bool) int {
	ninf := float32(math.Inf(-1))
	span := 2
	if wide {
		span = 3
	}
	best := 3
	for k := range c {
		d := float32(k-3) * sps / 4
		if end+d > 0 || k < 3-span || k > 3+span {
			c[k] = ninf
			continue
		}
		c[k] = win.corr(syms, end+d, sps)
		if c[k] > c[best] {
			best = k
		}
	}
	if wide || best != 3-span && best != 3+span {
		return best
	}
	k := best - 1
	if best > 3 {
		k = best + 1
	}
	d := float32(k-3) * sps / 4
	if end+d > 0 {
		return best
	}
	sv, svv, sxv, sp := win.sums(syms, end+d, sps)
	dc := sxv - c[best]
	if dc <= 0 {
		return best
	}
	const n = syncSymbols
	sxy := sxv - sp*sv/n
	spp := 9*n - sp*sp/n
	res := svv - sv*sv/n - sxy*sxy/spp
	if dc*dc*(n-2) <= leanEdgeMargin*2*9*n*res {
		return best
	}
	c[k] = sxv
	return k
}

func edgeVertex(c *[7]float32, best int) float32 {
	ninf := float32(math.Inf(-1))
	if best <= 0 || best >= len(c)-1 {
		return 0
	}
	lo, mid, hi := c[best-1], c[best], c[best+1]
	switch {
	case lo != ninf && hi != ninf:
		if den := lo - 2*mid + hi; den < 0 {
			return max32(min32(0.5*(lo-hi)/den, 0.5), -0.5)
		}
	case lo == ninf && hi != ninf && peakAt(c, best+2) != ninf:
		if den := mid - 2*hi + peakAt(c, best+2); den < 0 {
			return max32(min32(1+0.5*(mid-peakAt(c, best+2))/den, 0.5), -0.5)
		}
	case hi == ninf && lo != ninf && peakAt(c, best-2) != ninf:
		if den := peakAt(c, best-2) - 2*lo + mid; den < 0 {
			return max32(min32(0.5*(peakAt(c, best-2)-mid)/den-1, 0.5), -0.5)
		}
	}
	return 0
}

func peakAt(c *[7]float32, k int) float32 {
	if k < 0 || k >= len(c) {
		return float32(math.Inf(-1))
	}
	return c[k]
}

func (r *leanCore) resample(p enums.SyncPattern, saved *[syncSymbols]int8, wide bool) (float32, bool) {
	sps := 2 * r.tm.step
	end := r.lastAt
	if !r.burstKept() && (!r.locked || max(int32(-end-sps/2)-2, 0)+leanSyncWindow > r.acqKept()) {
		return 0, false
	}
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	var win syncWindow
	r.fillWindow(&win, max(int32(-end-sps/2)-2, 0))
	reach := int32(1)
	if r.locked {
		reach = 2
	}
	if !coarseSync(&win, &syms, end, sps, reach) {
		return 0, false
	}
	ninf := float32(math.Inf(-1))
	var c [7]float32
	best := win.peak(&syms, &c, end, sps, wide)
	if r.locked && best < len(c)-1 && c[best+1] == ninf {
		r.again = true
		return 0, false
	}
	bestD := float32(best-3)*sps/4 + edgeVertex(&c, best)*sps/4
	bestD = min(bestD, -end)
	sv, svv, sxv, sp := win.sums(&syms, end+bestD, sps)
	const n = syncSymbols
	sxy := sxv - sp*sv/n
	sxx := svv - sv*sv/n
	spp := 9*n - sp*sp/n
	if sxy <= 0 || sxy*sxy < discLock*discLock*sxx*spp {
		return 0, false
	}
	r.ringSync(&win, end+bestD, sps, saved)
	return bestD, true
}

func (r *leanCore) ringSync(win *syncWindow, t, sps float32, saved *[syncSymbols]int8) {
	j := ringPos(r.ns - syncSymbols)
	for i := range syncSymbols {
		v := win.at(t - float32(syncSymbols-1-i)*sps)
		saved[i], r.ring[j] = r.ring[j], int8(max(min(roundf(v), math.MaxInt8), math.MinInt8))
		if j++; j == leanRing {
			j = 0
		}
	}
}

func (r *leanCore) burstKept() bool {
	return r.keptSymbols(leanAcqSymbols)
}

func (r *leanCore) keptSymbols(n int) bool {
	return float32(n)*2*r.tm.step+4-r.lastAt <= float32(r.acqKept())
}

func (r *leanCore) resampleStart(d float32) {
	sps := 2 * r.tm.step
	if !r.burstKept() {
		r.tm.t += d
		return
	}
	n := leanAcqSymbols
	if r.keptSymbols(leanReplaySymbols) {
		n = leanReplaySymbols
	}
	j := ringPos(r.ns - int64(n))
	for i := range n - syncSymbols {
		x := r.keptAt(r.lastAt + d - float32(n-1-i)*sps)
		r.ring[j] = int8(max(min(roundf(x), math.MaxInt8), math.MinInt8))
		if j++; j == leanRing {
			j = 0
		}
	}
	r.tm.t += d
}

func (r *leanCore) push(s float32) {
	if x, at, ok := r.tm.push(s); ok {
		r.symbol(x, at)
	}
}

func roundBits(v float32) int32 {
	return int32(math.Float32bits(v+0x1.8p23)) - 0x4B400000
}

//go:noinline
func (r *leanCore) symbol(x, at float32) {
	r.lastAt = at
	r.pushSym(x)
	if !r.locked {
		r.searchHit(r.searchSigns())
		return
	}
	if r.hunt {
		r.huntSym(at)
	}
	if r.ns == r.slotEnd {
		if r.front96 {
			r.slotDue = true
			return
		}
		r.endSlot()
	}
}

func symQ(x, off float32) int8 {
	return int8(max(min(roundBits((x-off)/discStepHz), math.MaxInt8), math.MinInt8))
}

//go:section .text.pushSym
func (r *leanCore) pushSymGo(x float32) {
	pushSymAt[site1](r, x)
}

func pushSymAt[T any](r *leanCore, x float32) {
	q := int8(max(min(roundBits((x-r.off)/discStepHz), math.MaxInt8), math.MinInt8))
	h := uint32(r.head)
	nh := h + 1
	if nh == leanRing {
		nh = 0
	}
	r.ring[h] = q
	r.head = uint8(nh)
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
}

func isiLevel(q int8, off, thr int32) int32 {
	x := int32(q)*(discStepHz/llrStepHz) + off
	l := int32(1)
	if x > thr || x < -thr {
		l = 3
	}
	if x < 0 {
		l = -l
	}
	return l
}

//go:noinline
//go:section .text.cancelISI
func (r *leanCore) cancelISI(base int64) {
	k := roundBits(iqLowISI * llrStepHz / discStepHz * (1 << 16))
	thr, off, dev := 2*r.llrDev, r.llrOff, r.llrDev*k
	h := ringPos(base)
	hp := h + leanRing - 1
	if hp >= leanRing {
		hp -= leanRing
	}
	lp, x := isiLevel(r.ring[hp], off, thr), int32(r.ring[h])
	l := isiLevel(int8(x), off, thr)
	start := int32(max(r.ns-syncSymbols-base, 0))
	var d int32
	for i := range int32(slotSymbols - 1) {
		hn := h + 1
		if hn == leanRing {
			hn = 0
		}
		xn := r.ring[hn]
		xl := int32(xn)*(discStepHz/llrStepHz) + off
		ln := int32(1)
		if xl > thr || xl < -thr {
			ln = 3
		}
		if xl < 0 {
			ln = -ln
		}
		y := max(min(x-((lp+ln)*dev+1<<15)>>16, math.MaxInt8), math.MinInt8)
		r.ring[h] = int8(y)
		if i >= start {
			d += y - x
		}
		h, x, lp, l = hn, int32(xn), l, ln
	}
	r.ringSum += d
}

func (r *leanCore) searchSigns() (sym, mid bool) {
	r.pushMidSign()
	return r.signsMatch2(r.signs, r.midSigns)
}

//go:section .text.pushMidSign
func (r *leanCore) pushMidSign() {
	midSignAt[site1](r)
}

func midSignAt[_ any](r *leanCore) {
	r.midSigns <<= 1
	if (r.tm.half-r.off)*(float32(syncSymbols)/discStepHz) > float32(r.ringSum) {
		r.midSigns |= 1
	}
}

func (r *leanCore) searchHit(sym, mid bool) {
	switch {
	case r.ns > leanSyncEnd+1 && sym:
		r.searchSym(false)
	case r.ns >= leanRing && r.hasExt():
		if src, data, back, ok := r.ext.scan(r); ok && !r.clipped(r.ns-1-back) {
			r.forceLock(src, data, back)
		}
	}
	if mid && !r.locked && r.ns > leanSyncEnd+2 {
		r.searchMid()
	}
}

//go:noinline
//go:section .text.rawSym
func (r *leanCore) rawSym(x float32) (hit, sym, mid bool) {
	if leanRawFuse {
		q := int8(max(min(roundBits((x-r.off)/discStepHz), math.MaxInt8), math.MinInt8))
		h := uint32(r.head)
		nh := h + 1
		if nh == leanRing {
			nh = 0
		}
		r.ring[h] = q
		r.head = uint8(nh)
		old := h + leanRing - syncSymbols
		if old >= leanRing {
			old -= leanRing
		}
		sum := r.ringSum + (int32(q) - int32(r.ring[old]))
		r.ringSum = sum
		a := r.signs << 1
		if int32(q)*syncSymbols > sum {
			a |= 1
		}
		r.signs = a
		r.ns++
		b := r.midSigns << 1
		if (r.tm.half-r.off)*(float32(syncSymbols)/discStepHz) > float32(sum) {
			b |= 1
		}
		r.midSigns = b
		sym, mid = signsMatch[site0](r, a, b)
	} else {
		pushSymAt[site0](r, x)
		midSignAt[site0](r)
		sym, mid = signsMatch[site0](r, r.signs, r.midSigns)
	}
	hit = r.ns > leanSyncEnd+1 && sym || r.ns >= leanRing && r.hasExt() || mid && r.ns > leanSyncEnd+2
	return hit, sym, mid
}

//go:noinline
//go:section .text.lockSym
func (r *leanCore) lockSym(x, at float32) (again, mid, match bool) {
	r.lastAt = at
	r.pushSym(x)
	if !leanHuntMid {
		if !r.hunt || r.ns < leanRing {
			return false, false, false
		}
		r.again = false
		return false, false, r.signsOne(r.signs)
	}
	if leanHuntFuse && r.hunt && !r.hasExt() && r.midTiming && r.linHunt {
		return r.lockHunt(at)
	}
	if !r.hunt || !r.huntMid(r.lastAt) {
		return false, false, false
	}
	again = r.again
	r.again = false
	sym, mid := r.signsMatch2(r.signs, r.midSigns)
	return again, mid, mid || sym
}

func (r *leanCore) lockHunt(at float32) (again, mid, match bool) {
	if r.n-r.acqFrom > 4 {
		b := r.midSigns << 1
		tm := &r.tm
		t := at - tm.step
		var x float32
		switch {
		case t >= -1:
			x = tm.s3 + t*(tm.s3-tm.s2)
		case t >= -2:
			x = tm.s2 + (t+1)*(tm.s2-tm.s1)
		default:
			x = tm.s1 + (t+2)*(tm.s1-tm.s0)
		}
		if (x-r.off)*syncSymbols > float32(r.ringSum)*discStepHz {
			b |= 1
		}
		r.midSigns = b
	}
	if r.ns < leanRing {
		return false, false, false
	}
	again = r.again
	r.again = false
	sym, mid := r.signsMatch2(r.signs, r.midSigns)
	return again, mid, mid || sym
}

func (r *leanCore) lockPush(x, at float32) (more bool) {
	r.lastAt = at
	r.pushSym(x)
	return r.ns != r.slotEnd
}

func (r *leanCore) endSlot() {
	r.tm.settle()
	first := r.next == 0
	if r.hasExt() && first {
		r.ext.firstSlot(r)
	}
	r.slot()
	if first && r.locked && r.audio != nil && r.devScale > leanOverScale {
		r.fitScale()
	}
}

//go:noinline
func (r *leanCore) huntSym(at float32) {
	if again, mid, ok := r.huntSigns(at); ok || r.ms.retry {
		r.huntMatch(again, mid)
	}
}

func (r *leanCore) huntSigns(at float32) (again, mid, ok bool) {
	if !leanHuntMid {
		if r.ns < leanRing {
			return false, false, false
		}
		r.again = false
		return false, false, r.signsOne(r.signs)
	}
	if !r.huntMid(at) {
		return false, false, false
	}
	again = r.again
	r.again = false
	sym, mid := r.signsMatch2(r.signs, r.midSigns)
	return again, mid, mid || sym
}

//go:section .text.huntMid
func (r *leanCore) huntMid(at float32) bool {
	an := int64(leanAcqSamples)
	if r.locked {
		an = leanHuntSamples
	}
	if r.hasExt() {
		if b := r.ext.kept(); b != nil {
			an = int64(2 * len(b))
		}
	}
	if int32(min(r.n-r.acqFrom, an)) > 4 {
		r.midSigns <<= 1
		if r.midTiming {
			t := at - r.tm.step
			var x float32
			if r.linHunt {
				tm := &r.tm
				switch {
				case t >= -1:
					x = tm.s3 + t*(tm.s3-tm.s2)
				case t >= -2:
					x = tm.s2 + (t+1)*(tm.s2-tm.s1)
				default:
					x = tm.s1 + (t+2)*(tm.s1-tm.s0)
				}
			} else {
				x = r.tm.midAt(t)
			}
			if (x-r.off)*syncSymbols > float32(r.ringSum)*discStepHz {
				r.midSigns |= 1
			}
		} else if r.keptAt(at-r.tm.step)*syncSymbols > float32(r.ringSum) {
			r.midSigns |= 1
		}
	}
	return r.ns >= leanRing
}

//go:noinline
func (r *leanCore) huntMatch(again, mid bool) {
	if d := r.gridMiss(); r.ms.seek && d >= -leanSeekSyms && d <= leanSeekSyms {
		r.seekMatch(d, again && mid)
		return
	}
	switch {
	case again && mid && r.offGrid():
		r.midSync()
	case again && mid:
		r.retime(true)
	case !r.offGrid():
		r.retime(false)
	default:
		if r.searchSym(true); r.lockNs == r.ns {
			r.forced = true
		}
	}
}

func (r *leanCore) fitScale() {
	g := 1 / r.devScale
	r.scaleAudio(g)
	r.devScale, r.dcHz, r.discVar = 1, r.dcHz*g, r.discVar*g*g
	r.tm.dc, r.tm.dev = r.dcHz, deviationHz
	for i := range r.ms.lv {
		l := &r.ms.lv[i]
		l.devScale, l.dcHz, l.discVar = l.devScale*g, l.dcHz*g, l.discVar*g*g
	}
}

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
		if pw > 0 {
			r.noiseCoh = 0.9*r.noiseCoh + complex(0.1/pw, 0)*lag
		}
		return
	}
	lag -= r.noiseCoh * complex(r.floor, 0)
	if hz := atan2f(imag(lag), real(lag)) * (discRate / (2 * math.Pi)); abs32(hz) > leanSearchHz {
		r.retune(hz)
	}
}

func ringPos(k int64) uint32 {
	return ringIdx[site2](k)
}

func ringIdx[_ any](k int64) uint32 {
	if uint64(k) < 1<<32 {
		return uint32(k) % leanRing
	}
	return ringPosFar(k)
}

//go:noinline
func ringPosFar(k int64) uint32 {
	hi := uint32(int32(k>>32)%leanRing + leanRing)
	return (hi%leanRing*(1<<32%leanRing) + uint32(k)%leanRing) % leanRing
}

//go:section .text.sym
func (r *leanCore) sym(k int64) float32 {
	j := uint32(k) % leanRing
	if uint64(k) >= 1<<32 {
		j = ringPosFar(k)
	}
	return float32(r.ring[j])*discStepHz + r.off
}

//go:section .text.syncAt
func (r *leanCore) syncAt(end int64) (rho2 float32, src int, data bool) {
	var mx, xx int32
	var sr [numSources]int32
	j := ringPos(end - syncSymbols + 1)
	if leanSyncUnroll {
		var s0, s1, s2, s3 int32
		for k := range syncSymbols {
			q := int32(r.ring[j])
			if j++; j == leanRing {
				j = 0
			}
			mx += q
			xx += q * q
			s0 += int32(syncSyms8[0][k]) * q
			s1 += int32(syncSyms8[1][k]) * q
			s2 += int32(syncSyms8[2][k]) * q
			s3 += int32(syncSyms8[3][k]) * q
		}
		sr = [numSources]int32{s0, s1, s2, s3}
	} else {
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

func (r *leanCore) rcAnchor(base int64, try bool) (int64, bool) {
	if !try || r.src != 1 {
		return 0, false
	}
	for d := int64(-1); d <= 1; d++ {
		if r.rcAt(base+leanSyncEnd+d) >= discAnchor*discAnchor && r.anchors(base+leanSyncEnd+d, enums.SyncMSSourcedRC) {
			return d, true
		}
	}
	return 0, false
}

func (r *leanCore) rcAt(end int64) float32 {
	var syms [syncSymbols]int8
	syncSyms(enums.SyncMSSourcedRC, &syms)
	var mx, xx, sx, ss, sp int32
	j := ringPos(end - syncSymbols + 1)
	for k := range syncSymbols {
		q := int32(r.ring[j])
		if j++; j == leanRing {
			j = 0
		}
		p := int32(syms[k])
		mx, xx, sx, ss, sp = mx+q, xx+q*q, sx+q*p, ss+p, sp+p*p
	}
	fm, fs := float32(mx), float32(ss)
	sxx := float32(xx) - fm*fm/syncSymbols
	spp := float32(sp) - fs*fs/syncSymbols
	sxy := float32(sx) - fs*fm/syncSymbols
	if sxx <= 0 || spp <= 0 || sxy <= 0 {
		return 0
	}
	return sxy * sxy / (sxx * spp)
}

//go:noinline
//go:section .text.signsMatch
func (r *leanCore) signsMatch2(a, b uint32) (bool, bool) {
	return signsMatch[site1](r, a, b)
}

func signsMatch[T any](r *leanCore, a, b uint32) (bool, bool) {
	lo := (uint32(r.signMiss) + 1) * 0x01010101
	hi := (syncSymbols - uint32(r.signMiss)) * 0x01010101
	m := uint32(0x80808080) >> (32 - 8*uint32(r.sources))
	da, db := syncDists2[T](a, b)
	return syncHits(da, lo, hi)&m != 0, syncHits(db, lo, hi)&m != 0
}

func syncDists2[_ any](a, b uint32) (uint32, uint32) {
	return syncNib[0][a&15] + syncNib[1][a>>4&15] + syncNib[2][a>>8&15] +
			syncNib[3][a>>12&15] + syncNib[4][a>>16&15] + syncNib[5][a>>20&15],
		syncNib[0][b&15] + syncNib[1][b>>4&15] + syncNib[2][b>>8&15] +
			syncNib[3][b>>12&15] + syncNib[4][b>>16&15] + syncNib[5][b>>20&15]
}

func (r *leanCore) signsOne(a uint32) bool {
	lo := (uint32(r.signMiss) + 1) * 0x01010101
	hi := (syncSymbols - uint32(r.signMiss)) * 0x01010101
	m := uint32(0x80808080) >> (32 - 8*uint32(r.sources))
	return syncHits(syncDists(a), lo, hi)&m != 0
}

func syncDists(s uint32) uint32 {
	return syncNib[0][s&15] + syncNib[1][s>>4&15] + syncNib[2][s>>8&15] +
		syncNib[3][s>>12&15] + syncNib[4][s>>16&15] + syncNib[5][s>>20&15]
}

func syncHits(c, lo, hi uint32) uint32 {
	c |= 0x80808080
	return (^(c - lo) | (c - hi)) & 0x80808080
}

var syncNib = [syncSymbols / 4][16]uint32{
	{0x02000101, 0x03010002, 0x03010200, 0x04020101, 0x01010202, 0x02020103, 0x02020301, 0x03030202, 0x01010202, 0x02020103, 0x02020301, 0x03030202, 0x00020303, 0x01030204, 0x01030402, 0x02040303},
	{0x03030203, 0x02040102, 0x04020302, 0x03030201, 0x02020304, 0x01030203, 0x03010403, 0x02020302, 0x02020102, 0x01030001, 0x03010201, 0x02020100, 0x01010203, 0x00020102, 0x02000302, 0x01010201},
	{0x02020301, 0x01030202, 0x01010202, 0x00020103, 0x03030200, 0x02040101, 0x02020101, 0x01030002, 0x03010402, 0x02020303, 0x02000303, 0x01010204, 0x04020301, 0x03030202, 0x03010202, 0x02020103},
	{0x03010302, 0x02020203, 0x02020401, 0x01030302, 0x02020201, 0x01030102, 0x01030300, 0x00040201, 0x04000203, 0x03010104, 0x03010302, 0x02020203, 0x03010102, 0x02020003, 0x02020201, 0x01030102},
	{0x00030202, 0x01040103, 0x01020303, 0x02030204, 0x01020301, 0x02030202, 0x02010402, 0x03020303, 0x01020101, 0x02030002, 0x02010202, 0x03020103, 0x02010200, 0x03020101, 0x03000301, 0x04010202},
	{0x02030103, 0x01020202, 0x03040202, 0x02030301, 0x03020204, 0x02010303, 0x04030303, 0x03020402, 0x01020002, 0x00010101, 0x02030101, 0x01020200, 0x02010103, 0x01000202, 0x03020202, 0x02010301},
}

//go:noinline
func (r *leanCore) searchSym(strict bool) {
	if r.clipped(r.ns - 1) {
		return
	}
	rho2, src, data := r.syncAt(r.ns - 1)
	gate := float32(leanResample)
	if strict {
		gate = leanExtRho
	}
	weak := rho2 < gate*gate
	if weak && (strict || !r.hasExt() || rho2 < leanExtRho*leanExtRho) {
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	ok := false
	var d float32
	if !weak && (strict || r.ns >= r.acqNext) {
		var saved [syncSymbols]int8
		var sampled bool
		if d, sampled = r.resample(p, &saved, r.locked); sampled {
			if ok = r.fitLevels(r.ns-1, p, true, true); ok {
				r.resampleStart(d)
			} else {
				r.restoreSync(&saved)
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
		if strict || !r.hasExt() || !r.ext.verify(r, p) {
			return
		}
		if t, ok := r.ext.lockShift(r, p, 0); ok {
			d = r.shiftSync(t * 2 * r.tm.step)
		}
		r.fitLevels(r.ns-1, p, true, false)
	}
	r.lockStream(src, d, 0)
}

//go:noinline
func (r *leanCore) searchMid() {
	src, data := r.signsSource(r.midSigns)
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	if leanMidGate > 0 && !r.midNear(p) {
		return
	}
	at := r.lastAt
	r.lastAt = at - r.tm.step
	var saved [syncSymbols]int8
	d, ok := r.resample(p, &saved, false)
	r.lastAt = at
	if !ok {
		return
	}
	d -= r.tm.step
	if r.clipped(r.ns-1) || !r.fitLevels(r.ns-1, p, true, true) {
		for i, q := range &saved {
			r.ring[ringPos(r.ns-syncSymbols+int64(i))] = q
		}
		return
	}
	r.resampleStart(d)
	r.lockStream(src, d, 0)
}

//go:noinline
func (r *leanCore) clipped(end int64) bool {
	n := 0
	j := ringPos(end - syncSymbols + 1)
	for range syncSymbols {
		if q := r.ring[j]; q == math.MaxInt8 || q == math.MinInt8 {
			n++
		}
		if j++; j == leanRing {
			j = 0
		}
	}
	return 2*n > syncSymbols
}

func (r *leanCore) retime(mid bool) {
	rho2, src, data := r.syncAt(r.ns - 1)
	if mid {
		src, data = r.signsSource(r.midSigns)
	} else if rho2 < leanExtRho*leanExtRho {
		return
	}
	if src != int(r.src) || r.clipped(r.ns-1) {
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	var saved [syncSymbols]int8
	if d, ok := r.resample(p, &saved, true); ok {
		r.resampleStart(d)
		r.ms.moved = true
	}
}

//go:noinline
func (r *leanCore) midSync() {
	src, data := r.signsSource(r.midSigns)
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	var saved [syncSymbols]int8
	d, ok := r.resample(p, &saved, true)
	if !ok {
		return
	}
	if !r.clipped(r.ns-1) && r.fitLevels(r.ns-1, p, true, true) {
		r.resampleStart(d)
		r.lockStream(src, d, 0)
		r.forced = true
		return
	}
	r.restoreSync(&saved)
}

func (r *leanCore) shiftSync(d float32) float32 {
	sps := 2 * r.tm.step
	end := r.lastAt
	b0 := max(int32(-end-sps/2)-2, 0)
	if !r.burstKept() && b0+leanSyncWindow > r.acqKept() {
		return 0
	}
	var win syncWindow
	r.fillWindow(&win, b0)
	d = min(d, -end)
	var saved [syncSymbols]int8
	r.ringSync(&win, end+d, sps, &saved)
	r.resampleStart(d)
	return d
}

//go:noinline
func (r *leanCore) restoreSync(saved *[syncSymbols]int8) {
	j := ringPos(r.ns - syncSymbols)
	for _, q := range saved {
		r.ring[j] = q
		if j++; j == leanRing {
			j = 0
		}
	}
}

func (r *leanCore) signsSource(signs uint32) (src int, data bool) {
	c := syncDists(signs)
	for i := range int(r.sources) {
		m := uint8(c >> (8 * i))
		switch {
		case m <= r.signMiss:
			return i, false
		case m >= syncSymbols-r.signMiss:
			return i, true
		}
	}
	return 0, false
}

const staleSlots = 6

func (r *leanCore) stale() bool {
	if r.forced || r.suspect && (!r.bs || r.suspectRun >= r.bsSuspectRun) {
		return true
	}
	if r.lastAnch == leanNoSlot {
		return r.next > 0
	}
	return r.bs && r.next-r.lastAnch > staleSlots
}

func (r *leanCore) offGrid() bool {
	d := r.gridMiss()
	return d < -1 || d > 1
}

func (r *leanCore) gridMiss() int32 {
	m := int32(mod64(r.ns-1-leanSyncEnd-r.first-int64(r.ms.off[r.next&1]), slotSymbols))
	if m > slotSymbols/2 {
		m -= slotSymbols
	}
	return m
}

const (
	leanSeekSyms     = 4
	leanReplayErrors = 1
	leanRateGain     = 0.5
	leanSeekRho      = 0.5
	leanRateMax      = 2e-3
)

func (r *leanCore) seekMatch(d int32, mid bool) {
	if rho2, _, _ := r.syncAt(r.ns - 1); !mid && rho2 < leanSeekRho*leanSeekRho {
		return
	}
	if !r.ms.late && r.ms.anch[r.next&1] == leanNoSlot {
		r.ms.late = true
		r.slotEnd++
	}
	moved, retry := r.ms.moved, r.ms.retry
	r.ms.moved, r.ms.retry = false, false
	r.retime(mid)
	r.ms.retry = !r.ms.moved && !retry
	if r.ms.moved && d != 0 {
		r.ms.off[r.next&1] += int16(d)
		r.slotEnd += int64(d)
	}
	r.ms.moved = r.ms.moved || moved
}

func mod64(k int64, n uint32) uint32 {
	u := uint64(k)
	if k < 0 {
		u = -u
	}
	w := (0xFFFFFFFF%n + 1) % n
	m := (uint32(u>>32)%n*w + uint32(u)%n) % n
	if k < 0 && m != 0 {
		m = n - m
	}
	return m
}

func (r *leanCore) forceLock(src int, data bool, back int64) {
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	r.fitLevels(r.ns-1-back, p, true, false)
	d := float32(0)
	if t, ok := r.ext.lockShift(r, p, back); ok {
		d = t * 2 * r.tm.step
		r.tm.t += d
	}
	r.lockStream(src, d, back)
	r.forced, r.hunt = true, true
}

func (r *leanCore) lockStream(src int, d float32, back int64) {
	r.locked, r.forced, r.suspect, r.hunt, r.src, r.bs = true, false, false, false, uint8(src), sourceAt(src) == layer2.SyncSourceBS
	r.suspectRun = 0
	r.lockNs = r.ns
	r.streams++
	r.first = r.ns - 1 - back - leanSyncEnd
	r.next, r.lastAnch, r.lost = 0, leanNoSlot, 0
	r.ms.anch, r.ms.off, r.ms.moved, r.ms.seek = [2]int32{leanNoSlot, leanNoSlot}, [2]int16{}, false, false
	r.ms.in, r.ms.out, r.ms.fresh, r.ms.rate = 0, 0, [2]uint8{}, 0
	r.slotEnd = r.first + slotSymbols
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.drift, r.tm.lastStep = false, r.trackGain, 0, leanClockGain, 0, 0
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.acorr, r.tm.acorr2, r.tm.anc, r.tm.anall, r.tm.aweak = 0, 0, 0, 0, 0
	if r.mmOK && !r.tm.mm {
		r.tm.mm, r.tm.mid = true, false
		r.tm.t += r.tm.step
		r.tm.half = 0
	}
	r.resetChans()
	r.eAcc, r.eSyms = 0, 0
	if r.hasExt() {
		r.ext.locked(r, d)
	}
}

func (r *leanCore) fitLevels(end int64, p enums.SyncPattern, first, check bool) bool {
	g, dc, res := r.fitSync(end, p)
	if first {
		if check && (g <= 0 || sqrtf(res) > discFirstSpread*2*g*deviationHz) {
			return false
		}
		g = max(g, leanMinScale)
		r.devScale, r.dcHz, r.discVar = g, dc, res
		if a := r.audio; a != nil {
			t := r.tuneHz
			if int32(uint32(end)-r.tuneNs) < 0 {
				t = r.tunePrev
			}
			r.fitHz = (t + dc) / a.gain
		}
		r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
		return true
	}
	r.devScale = levelForget*r.devScale + (1-levelForget)*g
	r.dcHz = levelForget*r.dcHz + (1-levelForget)*dc
	r.discVar = levelForget*r.discVar + (1-levelForget)*res
	if a := r.audio; a != nil {
		t := r.tuneHz
		if int32(uint32(end)-r.tuneNs) < 0 {
			t = r.tunePrev
		}
		r.fitHz = levelForget*r.fitHz + (1-levelForget)*(t+dc)/a.gain
	}
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
	return true
}

func (r *leanCore) fitSync(end int64, p enums.SyncPattern) (g, dc, res float32) {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	j0 := ringPos(end - syncSymbols + 1)
	var mp, mx float32
	j := j0
	for k := range syncSymbols {
		mx += float32(r.ring[j])*discStepHz + r.off
		if j++; j == leanRing {
			j = 0
		}
		mp += float32(syms[k])
	}
	mp /= syncSymbols
	mx /= syncSymbols
	var sxy, spp float32
	j = j0
	for k := range syncSymbols {
		sxy += (float32(syms[k]) - mp) * (float32(r.ring[j])*discStepHz + r.off - mx)
		if j++; j == leanRing {
			j = 0
		}
		spp += (float32(syms[k]) - mp) * (float32(syms[k]) - mp)
	}
	g = sxy / spp / deviationHz
	dc = mx - g*deviationHz*mp
	j = j0
	for k := range syncSymbols {
		e := float32(r.ring[j])*discStepHz + r.off - dc - g*deviationHz*float32(syms[k])
		if j++; j == leanRing {
			j = 0
		}
		res += e * e
	}
	return g, dc, res / syncSymbols
}

//go:noinline
func (r *leanCore) slot() {
	j := r.next
	r.next++
	base := r.first + int64(j)*slotSymbols + int64(r.ms.off[j&1])
	energy := r.eAcc / float32(max(r.eSyms, 1))
	r.eAcc, r.eSyms = 0, 0
	_, dp, _ := sourceAt(int(r.src)).SyncPatterns()
	seek := !r.bs && r.lastAnch != leanNoSlot && r.ms.anch[j&1] == leanNoSlot
	isAnchor, shift, anchor := r.slotAnchor(base, seek)
	c := &r.ch[j&1]
	sf := c.sf
	if isAnchor {
		base += shift
		if seek {
			r.slotFirst(j, base, anchor)
		}
		r.anchorAt(j, base, shift, anchor, energy)
	}
	present := isAnchor || r.noise == 0 || energy > 1.5*r.noise
	if seek && !isAnchor {
		present = false
		r.noise = min(r.noise, energy)
	}
	r.holdNext(j, present)
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
	if !r.isReplaying() {
		switch {
		case isAnchor:
			r.suspect = false
		case r.dataAnch[j&1], sf != leanNoSlot && j-sf == 2*layer2.VoiceBurstsPerSuperframe && c.sf != j:
			r.suspect = true
		}
		r.dataAnch[j&1] = isAnchor && anchor == dp
	}
	r.slotSwitch(j)
	r.slotEnd = r.first + int64(j+2)*slotSymbols + int64(r.ms.off[(j+1)&1])
	r.setHunt()
	if !r.isReplaying() && r.ended(j, base) {
		r.unlock()
	}
}

func (r *leanCore) slotAnchor(base int64, seek bool) (bool, int64, enums.SyncPattern) {
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
	ms := !r.bs && r.lastAnch != leanNoSlot
	if d, ok := r.rcAnchor(base, ms && !isAnchor); ok {
		isAnchor, shift, anchor = true, d, enums.SyncMSSourcedRC
	}
	if seek && isAnchor && r.isReplaying() && r.syncErrors(base+leanSyncEnd+shift, anchor) > leanReplayErrors {
		isAnchor = false
	}
	return isAnchor, shift, anchor
}

func (r *leanCore) holdNext(j int32, present bool) {
	r.tm.hold = !r.bs && !present
	if !r.bs && r.lastAnch != leanNoSlot {
		r.tm.hold = r.ms.anch[(j+1)&1] == leanNoSlot
	}
}

func (r *leanCore) slotFirst(j int32, base int64, anchor enums.SyncPattern) {
	o := (j + 1) & 1
	r.ms.lv[o] = r.levels()
	r.fitLevels(base+leanSyncEnd, anchor, true, false)
	r.amp = 0
	if !r.isReplaying() {
		r.ms.fresh[j&1] = 2
	}
}

func (r *leanCore) levels() leanLevels {
	return leanLevels{r.devScale, r.dcHz, r.discVar, r.amp, r.noise, r.fitHz}
}

func (r *leanCore) setLevels(l *leanLevels) {
	r.devScale, r.dcHz, r.discVar, r.amp, r.noise, r.fitHz = l.devScale, l.dcHz, l.discVar, l.amp, l.noise, l.fitHz
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
}

func (r *leanCore) symWait() float32 {
	if !r.tm.mm && r.tm.mid {
		return r.tm.t + r.tm.step
	}
	return r.tm.t
}

func (r *leanCore) savePhase(i int32) {
	r.ms.t[i], r.ms.n[i], r.ms.ns[i] = r.symWait(), uint16(r.n), uint16(r.ns)
}

func (r *leanCore) slotSwitch(j int32) {
	if r.bs || r.lastAnch == leanNoSlot {
		return
	}
	m := &r.ms
	p, q := j&1, (j+1)&1
	live := !r.isReplaying()
	sps := 2 * r.tm.step
	if m.anch[p] != leanNoSlot {
		if live {
			r.slotRate(p, sps)
			r.savePhase(p)
			m.out |= 1 << p
		}
		m.lv[p] = r.levels()
	}
	m.in &^= 1 << p
	if m.fresh[p] > 0 {
		m.fresh[p]--
	}
	if m.anch[q] != leanNoSlot && j+1-m.anch[q] >= lostSlots {
		m.anch[q] = leanNoSlot
	}
	moved := m.moved
	m.moved = false
	if m.anch[q] == leanNoSlot {
		m.off[q] = m.off[p]
		r.tm.hold = true
		return
	}
	r.tm.hold = false
	if m.anch[p] == leanNoSlot && !moved {
		return
	}
	if m.anch[p] != leanNoSlot {
		r.setLevels(&m.lv[q])
	}
	if !live {
		return
	}
	lo := float32(0)
	if !r.tm.mm && r.tm.mid {
		lo = r.tm.step
	}
	u := m.t[q] + float32(int16(m.n[q]-uint16(r.n))) + float32(int16(uint16(r.ns)-m.ns[q]))*(sps+m.rate)
	cur := r.symWait()
	for k := 0; (u <= lo || u <= cur-sps/2) && k < 4; k++ {
		u += sps
		m.off[q]--
	}
	for k := 0; u > cur+sps/2 && u-sps > lo && k < 4; k++ {
		u -= sps
		m.off[q]++
	}
	r.tm.t = u - lo
	r.savePhase(q)
	m.in |= 1 << q
}

func (r *leanCore) slotRate(p int32, sps float32) {
	m := &r.ms
	n := int32(int16(uint16(r.ns) - m.ns[p]))
	if n <= 0 || (m.in|m.out)&(1<<p) == 0 {
		return
	}
	e := r.symWait() - m.t[p] - float32(int16(m.n[p]-uint16(r.n))) - float32(n)*sps
	if m.in&(1<<p) != 0 {
		m.rate += leanRateGain * e / float32(2*n)
	} else {
		m.rate += leanRateGain * (e/float32(n) - m.rate)
	}
	m.rate = max32(min32(m.rate, leanRateMax*sps), -leanRateMax*sps)
}

func (r *leanCore) anchorAt(j int32, base, shift int64, anchor enums.SyncPattern, energy float32) {
	r.fitLevels(base+leanSyncEnd, anchor, false, true)
	d := r.dcHz
	if o := (j + 1) & 1; !r.bs && r.ms.anch[o] != leanNoSlot {
		d = (d + r.ms.lv[o].dcHz) / 2
	}
	if abs32(d) > leanRetuneHz && !r.isReplaying() {
		r.retune(d)
	}
	if !r.bs {
		k := shift + int64(r.ms.off[j&1])
		r.first += k
		r.ms.off[(j+1)&1] -= int16(k)
		r.ms.off[j&1] = 0
		r.ms.anch[j&1] = j
	} else {
		r.first += shift
	}
	if r.amp == 0 || energy > levelJump*r.amp || levelJump*energy < r.amp {
		r.amp, r.noise = energy, energy/4
	} else {
		r.amp = levelForget*r.amp + (1-levelForget)*energy
	}
	r.lastAnch = j
	if base > r.lockNs {
		r.forced = false
	}
}

func (r *leanCore) setHunt() {
	switch {
	case !r.suspect:
		r.suspectRun = 0
	case r.suspectRun < math.MaxUint8:
		r.suspectRun++
	}
	r.ms.seek, r.ms.retry, r.ms.late = r.seeking(), false, false
	if !r.stale() {
		r.hunt = false
		if !r.ms.seek {
			return
		}
	}
	if !r.hunt {
		r.acqFrom = r.n
		if !r.hasExt() || r.ext.kept() == nil {
			r.acqHead, r.acqLow = 0, 0
		}
	}
	r.hunt = true
}

func (r *leanCore) seeking() bool {
	p := r.next & 1
	return !r.bs && r.lastAnch != leanNoSlot && (r.ms.anch[p] == leanNoSlot || r.ms.fresh[p] != 0) && !r.isReplaying()
}

func (r *leanCore) anchors(end int64, p enums.SyncPattern) bool {
	if r.syncErrors(end, p) > discAnchorBits || r.noCarrier(end, p) {
		return false
	}
	return !r.isReplaying() || !r.clipped(end)
}

func (r *leanCore) noCarrier(end int64, p enums.SyncPattern) bool {
	out := float32(leanClickOut) / 2 * r.devScale * deviationHz
	top := r.ns
	if r.isReplaying() {
		top = min(r.first+int64(r.next)*slotSymbols+1, r.ns)
	}
	n := int32(0)
	k0, k1 := max(end-leanSyncEnd, top-leanRing), min(end-leanSyncEnd+slotSymbols, top)
	j := ringPos(k0)
	for k := k0; k < k1; k++ {
		if abs32(float32(r.ring[j])*discStepHz+r.off-r.dcHz) > out {
			n++
		}
		if j++; j == leanRing {
			j = 0
		}
	}
	g, _, res := r.fitSync(end, p)
	if r.isReplaying() {
		return n >= leanNoCarrierClicks && (g <= 0 || sqrtf(res) > leanNoCarrierSpread*2*g*deviationHz && r.gapSpread(g, res))
	}
	if n >= leanNoCarrierClicks {
		return g <= 0 || sqrtf(res) > leanNoCarrierSpread*2*g*deviationHz
	}
	if n < leanGapClicks || r.lastAnch == leanNoSlot || g <= 0 || sqrtf(res) <= leanGapFloor*2*g*deviationHz {
		return false
	}
	return r.gapSpread(g, res)
}

func (r *leanCore) gapSpread(g, res float32) bool {
	return res*r.devScale*r.devScale > leanGapSpread*leanGapSpread*r.discVar*g*g
}

func (r *leanCore) ended(j int32, base int64) bool {
	return r.lost >= lostSlots || r.lastAnch == leanNoSlot && j >= 6 || j-r.lastAnch >= lostSlots ||
		r.forced && base-r.lockNs > 6*slotSymbols
}

func (r *leanCore) retune(d float32) {
	r.tuneHz += d
	if r.audio != nil {
		r.audio.retuneFront(r.tuneHz, d)
	} else {
		r.iq.retuneFront(r.tuneHz, d)
	}
	r.shift(d)
}

func (r *leanCore) shift(d float32) {
	if r.locked {
		r.off -= d
	} else {
		if r.acqKept() > 0 {
			r.rescaleKept(256, int32(roundf(-d*(256.0/discStepHz))))
		}
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
	r.ms.lv[0].dcHz -= d
	r.ms.lv[1].dcHz -= d
	r.tm.s1 -= d
	r.tm.s2 -= d
	r.tm.s0 -= d
	r.tm.s3 -= d
	r.tm.sa -= d
	r.tm.sb -= d
	r.tm.last -= d
	r.tm.dc -= d
	if !r.tm.mm {
		r.tm.half -= d
	}
}

//go:noinline
func (r *leanCore) syncErrors(end int64, p enums.SyncPattern) int {
	step := r.devScale * deviationHz
	t0 := (r.dcHz - r.off) / discStepHz
	hi, lo := t0+2*step/discStepHz, t0-2*step/discStepHz
	n := 0
	j := ringPos(end - syncSymbols + 1)
	for k := range syncSymbols {
		q := float32(r.ring[j])
		if j++; j == leanRing {
			j = 0
		}
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

//go:noinline
func (r *leanCore) unlock() {
	if r.hasExt() {
		r.ext.end(r)
	}
	if !r.hunt || !r.hasExt() || r.ext.kept() == nil {
		r.acqFrom = r.n
	}
	r.locked, r.forced, r.hunt = false, false, false
	if a := r.audio; a != nil {
		a.sum, a.sq, a.n, a.qsum, a.qsq = 0, 0, 0, 0, 0
	}
	r.rebuildSigns()
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.drift, r.tm.lastStep = false, r.acqGain, 0, 0, 0, 0
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.acorr, r.tm.acorr2, r.tm.anc, r.tm.anall, r.tm.aweak = 0, 0, 0, 0, 0
	if r.tm.mm {
		r.tm.mm, r.tm.mid = false, true
		r.tm.t -= r.tm.step
	}
	r.offsetHz = int16(max32(min32(roundf(r.tuneHz+r.dcHz), math.MaxInt16), math.MinInt16))
	r.devHz = int16(max32(min32(roundf(3*deviationHz*r.devScale), math.MaxInt16), math.MinInt16))
	r.devAtHz = int16(max32(min32(roundf(r.fitHz), math.MaxInt16), math.MinInt16))
}

func (r *leanCore) rebuildSigns() {
	r.ringSum, r.signs = 0, 0
	for k := int64(syncSymbols); k > 0; k-- {
		r.ringSum += int32(r.ring[ringPos(r.ns-k)])
	}
	for k := int64(syncSymbols); k > 0; k-- {
		r.signs <<= 1
		if int32(r.ring[ringPos(r.ns-k)])*syncSymbols > r.ringSum {
			r.signs |= 1
		}
	}
}

//go:section .text.symLLR
func (r *leanCore) symLLR(k int64) (int16, int16) {
	j := uint32(k) % leanRing
	if uint64(k) >= 1<<32 {
		j = ringPosFar(k)
	}
	x := int32(r.ring[j])*(discStepHz/llrStepHz) + r.llrOff
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
	l0, l1 := evenLLR[site0](b0), evenLLR[site1](near-far)
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

func (r *leanCore) llrTerm(e int32) int32 {
	return max(-int32(int64(e*e)*int64(r.llrK)>>llrShift), r.llrFloor)
}

func evenLLR[_ any](v int32) int16 {
	h := (v + 1) >> 1
	if v < 0 {
		h = -((1 - v) >> 1)
	}
	return int16(max(min(h, 127), -127)) * 2
}

func (r *leanCore) setLLR() {
	r.llrOff = int32(roundf((r.off - r.dcHz) / llrStepHz))
	r.llrDev = int32(roundf(r.dev / llrStepHz))
	r.llrK = int32(-r.ll*8*llrStepHz*llrStepHz*(1<<llrShift) + 0.5)
	r.llrFloor = math.MinInt32 / 2
	if r.lfloor >= -math.MaxFloat32 {
		r.llrFloor = int32(roundf(r.lfloor * 8))
	}
}

func (r *leanCore) llrAt(p int) int16 {
	return llrAtS[site1](r, p)
}

func llrAtS[_ any](r *leanCore, p int) int16 {
	l0, l1 := r.symLLR(r.base + cachSymbols + int64(p>>1))
	if p&1 == 0 {
		return l0
	}
	return l1
}

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
	b.Distance, b.BS, b.HasCACH, b.Stream, b.Slot = -1, r.bs, r.bs, int(r.streams), int64(j)
	b.Sync, b.Voice, b.HasFixed = 0, 0, false
	b.Fixed, b.FixedCACH = layer2.RawBurst{}, [3]byte{}
	ticks := (r.mfStep*(r.n-1)+r.center-(r.ns-1-(base+cachSymbols))*samplesPerSy)<<8 +
		int64(int32(roundf(float32(int32(r.mfStep))*r.lastAt*256))) - int64(r.lagTicks)
	b.Seconds = tickSeconds(ticks)
	if r.iqLow && iqLowISI > 0 {
		r.cancelISI(base)
	}
	thr, out := 2*r.llrDev, r.llrDev*leanClickOut/2
	h := ringPos(base)
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
		switch {
		case k&3 != 3:
		case k < cachSymbols:
			b.CACH[k/4] = acc
		default:
			b.Data[(k-cachSymbols)/4] = acc
		}
	}
	if r.clickSpan > 0 {
		r.clickFloor(v)
	}
	if !r.classify(b, j) && (!r.hasExt() || !r.ext.place(r, b, j)) {
		if r.hasExt() && !r.isReplayDry() {
			r.ext.unplaced(r, b)
		}
		return
	}
	if r.bs {
		r.sink.noteLean(r, b)
	}
	if r.clickSpan > 0 {
		r.clicks += leanClickGain * (float32(beyond)/slotSymbols - r.clicks)
		r.clickFloor(v)
	}
	if r.hasExt() {
		r.ext.burst(r, b)
		return
	}
	if r.softFEC {
		cc, ok := r.colorCode()
		r.repair(&r.fec, b, cc, ok)
	}
	r.emit(b)
}

func (r *leanCore) emit(b *Burst) {
	if a := r.audio; a != nil {
		a.observe(b)
	}
	if r.onBurst != nil {
		r.onBurst(b)
	} else {
		r.sink.put(b)
	}
}

func (r *leanCore) clickFloor(v float32) {
	po := max(r.clicks, leanMinClicks)
	lf := log32(po/r.clickSpan) - log32(1-po) + 0.5*log32(2*math.Pi*v)
	r.llrFloor = int32(roundf(lf * 8))
}

func (r *leanCore) colorCode() (enums.ColorCode, bool) {
	best := 0
	for i, v := range &r.votes {
		if v > r.votes[best] {
			best = i
		}
	}
	return enums.ColorCode(best), r.votes[best] > 0
}

//go:noinline
func (r *leanCore) classify(b *Burst, j int32) bool {
	c := &r.ch[j&1]
	voice, data, _ := sourceAt(int(r.src)).SyncPatterns()
	dv, dd := syncDist(&b.Data, voice), syncDist(&b.Data, data)
	switch {
	case r.anchor == data || r.anchor == 0 && dd <= damagedSyncDistance && dd < dv:
		b.Distance, b.Sync = dd, data
		c.sf, c.data = leanNoSlot, false
		r.noteData(b, j, c)
		return true
	case r.anchor == enums.SyncMSSourcedRC:
		b.Distance, b.Sync = syncDist(&b.Data, enums.SyncMSSourcedRC), enums.SyncMSSourcedRC
		return true
	case r.anchor == voice || dv <= layer2.DefaultSyncThreshold:
		b.Distance, b.Sync = dv, voice
		c.sf, c.data = j, false
		return true
	}
	if c.sf != leanNoSlot {
		switch n := (j - c.sf) / 2; {
		case n >= 1 && n <= 5 && (j-c.sf)%2 == 0:
			b.Voice = enums.VoiceBurstA + enums.VoiceBurstType(n)
			if !r.isReplaying() {
				emb := embOf(&b.Data)
				ok := r.embOK(&emb)
				r.suspect = r.suspect || !ok
				r.forced = r.forced && !ok
				if ok && emb.FEC.ErrorsCorrected == 0 && r.votes[emb.ColorCode&0xF] < math.MaxUint8 {
					r.votes[emb.ColorCode&0xF]++
				}
			}
			return true
		case n == 6 && dv <= voiceSyncDistance:
			b.Distance, b.Sync, b.Voice = dv, voice, enums.VoiceBurstA
			c.sf, c.data = j, false
			return true
		}
		c.sf = leanNoSlot
	}
	if c.hdr == j-2 && dv <= voiceSyncDistance || dv <= damagedSyncDistance {
		b.Distance, b.Sync, b.Voice = dv, voice, enums.VoiceBurstA
		c.sf, c.data = j, false
		return true
	}
	if pos, ok := r.embPosition(b); ok && !c.data {
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

func (r *leanCore) embPosition(b *Burst) (enums.VoiceBurstType, bool) {
	cc, ok := r.colorCode()
	if !ok {
		return 0, false
	}
	emb := embOf(&b.Data)
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

func (r *leanCore) embOK(emb *pdu.EmbeddedSignalling) bool {
	cc, ok := r.colorCode()
	return !emb.FEC.Uncorrectable && (!ok || emb.ColorCode == cc)
}

func embOf(d *layer2.RawBurst) pdu.EmbeddedSignalling {
	cw := uint16(d[13]&0xf)<<12 | uint16(d[14]>>4)<<8 | uint16(d[18]&0xf)<<4 | uint16(d[19]>>4)
	info, res := pdu.DecodeEMBCodeword(cw)
	e := info.EmbeddedSignalling()
	e.FEC = res
	return e
}

func (r *leanCore) noteData(b *Burst, j int32, c *leanChan) {
	st := b.Data.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	c.data = !r.bs && !isHeader(st.DataType)
	if st.FEC.ErrorsCorrected <= leanVoteErrors && r.votes[st.ColorCode&0xF] < math.MaxUint8 {
		r.votes[st.ColorCode&0xF]++
	}
	if isHeader(st.DataType) {
		c.hdr = j
	}
}

func isHeader(dt enums.DataType) bool {
	return dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader
}

const (
	tickMant = 0x15d867c3ece2a5
	tickExp  = -76
)

//go:noinline
func tickSeconds(t int64) float64 {
	if t == 0 {
		return 0
	}
	var sign uint64
	u := uint64(t)
	if t < 0 {
		sign, u = 1<<63, -u
	}
	hi, lo := bits.Mul64(u, tickMant)
	n := 64 - bits.LeadingZeros64(lo)
	if hi != 0 {
		n = 128 - bits.LeadingZeros64(hi)
	}
	s := n - 53
	var m uint64
	if s <= 0 {
		m = lo << uint(-s)
	} else {
		m = lo>>uint(s) | hi<<uint(64-s)
		rem, half := lo&(1<<uint(s)-1), uint64(1)<<uint(s-1)
		if rem > half || rem == half && m&1 != 0 {
			if m++; m == 1<<53 {
				m, s = m>>1, s+1
			}
		}
	}
	return math.Float64frombits(sign | uint64(s+tickExp+52+1023)<<52 | m&(1<<52-1))
}

// Each site type makes TinyGo build a separate copy of a generic helper per
// call site, so that -opt z inlines it.
type (
	site0 struct{}
	site1 struct{}
	site2 struct{}
)
