package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	ibits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/golay"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	discAnchor          = 0.6
	levelForget         = 0.8
	leanSyncClip        = 2
	leanSyncClipMax     = 4
	levelRefit          = 5
	levelRefitFloor     = discStepHz * discStepHz / 12
	levelJumpZ          = 5
	leanClipSyms        = 2
	leanClipDrop        = 0.25
	discAnchorBits      = 12
	discFirstSpread     = 0.35
	leanRing            = slotSymbols + 2
	leanChunk           = 6
	leanSyncEnd         = syncFirstSym + syncSymbols - 1
	leanAcqSymbols      = leanSyncEnd + 1 - cachSymbols
	leanReplaySymbols   = leanSyncEnd + 1
	leanAcqSamples      = (leanReplaySymbols*5/2 + 7 + 2) &^ 1
	leanResample        = 0.7
	leanPairLock        = 0.84
	leanPairSlots       = 6
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
	leanLockMMStep      = 0.1
	leanSignMiss        = 5
	leanBurstSec        = float64(slotSymbols-cachSymbols) / symbolRate
	leanFadeBursts      = 2
	leanFlatSyms        = 1
	leanInBurst         = cachSymbols + syncSymbols - 1
	leanDeferSkew       = 0.125
	leanProofErrors     = 0
	leanExtSignMiss     = 9
	leanExtRho          = 0.3
	llrStepHz           = 4
	llrShift            = 24
	leanClickGain       = 0.1
	leanMinClicks       = 1e-3
	leanClickOut        = 9
	leanNoCarrierClicks = 36
	leanNoCarrierSpread = 1.2
	leanSyncScaleMin    = 0.25
	leanGapSpread       = 4
	leanGapFloor        = 0.9
	leanGapClicks       = 16
	leanCubicSPS        = 2.5
	leanSixSPS          = 2
	leanMMSPS           = 2.5
	leanMMSign          = 1
	leanMMGain          = 2
	leanClockGain       = leanClockBudget / (slotSymbols * leanPhaseRatio * leanPhaseRatio)
	leanClockBudget     = 0.01
	leanPhaseRatio      = 0.88
	leanClockAvg        = leanClockSig * leanClockSig / (leanClockGain * leanClockBlock)
	leanClockMax        = 0.002
	replayFlatLevel     = 0.3
	replaySyncRho       = 0.95
	leanClockBlock      = 64
	leanClockSpan       = 4
	leanClockSig        = 5
	leanRefitVar        = 0.02
	leanClockLag        = 120
	leanClockLagBS      = leanClockLag / 2
)

const (
	mid96Cubic = iota
	mid96Linear
	mid96Raw
	mid96RawLock
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
	rateK          float32
	lastC          float32
	stale          bool
	leanClock
	baseN        int32
	baseC        float32
	mK           int32
	mS, mC, mVar float32
	mOK          bool
}

type leanClock struct {
	rate, corr, corr2       float32
	owed, estS, estW, fresh float32
	nc, nall, estN          int32
	learned                 bool
}

func (tm *leanTiming) newTalker() {
	tm.corr, tm.corr2, tm.nc, tm.nall = 0, 0, 0, 0
	tm.estS, tm.estW, tm.estN = 0, 0, 0
	tm.owed, tm.fresh, tm.learned, tm.mOK = 0, 1, false, false
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

func (tm *leanTiming) measure(s, v float32) {
	n, c := tm.baseN+tm.nall, tm.baseC+tm.corr
	if tm.mOK && n > tm.mK {
		k := float32(n - tm.mK)
		res := (c - tm.mC + s - tm.mS) / k
		w := k * k / (tm.mVar + v)
		tm.estS, tm.estW = tm.estS+w*res, tm.estW+w
	}
	tm.mK, tm.mC, tm.mS, tm.mVar, tm.mOK = n, c, s, v, true
}

func (tm *leanTiming) settle(lagN float32) {
	if tm.nc < leanClockBlock {
		return
	}
	n := float32(tm.nc)
	v := tm.corr2/n - tm.corr*tm.corr/(n*n)
	nall := float32(tm.nall)
	q, keep := 1-1/lagN, float32(1)
	for k := tm.nall; k > 0; k >>= 1 {
		if k&1 != 0 {
			keep *= q
		}
		q *= q
	}
	share := tm.owed * (1 - keep)
	lag := lagN * tm.fresh * (1 - keep)
	span := nall - lag
	res := (tm.corr - share + lag*tm.rate) / span
	tm.owed -= share
	tm.fresh *= keep
	tm.baseN, tm.baseC = tm.baseN+tm.nall, tm.baseC+tm.corr
	tm.corr, tm.corr2, tm.nc, tm.nall = 0, 0, 0, 0
	if tm.rateK == 0 || v <= 0 {
		return
	}
	w := span * span / (n * v)
	if tm.estN >= leanClockAvg {
		tm.estS, tm.estW = tm.estS*(leanClockAvg-1)/leanClockAvg, tm.estW*(leanClockAvg-1)/leanClockAvg
	} else {
		tm.estN++
	}
	tm.estS, tm.estW = tm.estS+w*res, tm.estW+w
	if e := tm.estS / tm.estW; e*e*tm.estW > leanClockSig*leanClockSig {
		lim := leanClockMax * 2 * tm.step
		old := tm.rate
		tm.rate = max32(min32(tm.rate+e, lim), -lim)
		tm.owed += (tm.rate - old) * lagN
		tm.mOK = false
		tm.estS, tm.estW, tm.estN, tm.learned = 0, 0, 0, true
	} else if tm.estN*leanClockBlock >= leanClockSpan*slotSymbols {
		tm.learned = true
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
		if tm.stale {
			tm.stale, e = false, 0
		}
		tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*y*y
		tm.last, tm.half = x, d
		tm.t += 2*tm.step + tm.rate
		tm.nall++
		tm.lastC = 0
		if !tm.hold && tm.pow > 0 {
			c := max32(min32(leanMMGain*tm.gain*e/tm.pow, 0.5), -0.5)
			tm.t += c
			tm.lastC = c
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
	if tm.stale {
		tm.stale, e = false, 0
	}
	tm.last, tm.mid = x, true
	tm.t += tm.step
	tm.lastC = 0
	if !tm.hold && tm.pow > 0 {
		tm.lastC = max32(min32(tm.gain*e/tm.pow, 0.5), -0.5)
		tm.t += tm.lastC
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
	lastGood int32
	termAt   int32
	pendEnd  int64
	pendWait uint8
	pendSrc  uint8
	pendData bool
	unproven bool
	rcLone   bool
	pow      leanPow
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
	off      float32
	acqNext  int64
	slotEnd  int64
	ringSum  int32
	signs    uint32
	cb       Burst

	acqGain        float32
	trackGain      float32
	pairNs         int64
	pairSrc        uint8
	pairLv         bool
	refit          bool
	tuneHz         float32
	offsetHz       float32
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
	jumpZ          float32
	preDev, preOff float32
	stepHold       bool
	preStep        bool
	preCut         bool

	signMiss                           uint8
	suspectRun, bsSuspectRun           uint8
	suspect, hunt, cohRefit            bool
	reEntry                            bool
	dataAnch                           [2]bool
	again, midTiming, front96, slotDue bool
	lockDue                            bool
	mid96                              uint8
	replaying, replayDry, replayDeep   bool
	lateTime                           bool
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
	fresh [2]uint8
	clk   leanClock
	clkNs uint16
	clkAt uint8
}

func (r *leanCore) init(onBurst func(*Burst), opt options, sps float32) {
	r.onBurst, r.lastAnch, r.softFEC = onBurst, leanNoSlot, opt.SoftFEC
	r.cohRefit = opt.CohExt
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

func (r *leanCore) resample(p enums.SyncPattern, saved *[syncSymbols]int8, wide bool, lim float32) (d, c2, z float32, ok bool) {
	sps := 2 * r.tm.step
	end := r.lastAt
	if !r.burstKept() && (!r.locked || max(int32(-end-sps/2)-2, 0)+leanSyncWindow > r.acqKept()) {
		return 0, 0, 0, false
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
		return 0, 0, 0, false
	}
	ninf := float32(math.Inf(-1))
	var c [7]float32
	best := win.peak(&syms, &c, end, sps, wide)
	if r.locked && best < len(c)-1 && c[best+1] == ninf {
		r.again = true
		return 0, 0, 0, false
	}
	bestD := float32(best-3)*sps/4 + edgeVertex(&c, best)*sps/4
	if best == 0 || best == len(c)-1 {
		k := float32(-4)
		if best > 0 {
			k = 4
		}
		if end+k*sps/4 > 0 {
			return 0, 0, 0, false
		}
		out, in := win.corr(&syms, end+k*sps/4, sps), c[best-int(k)/4]
		if out >= c[best] {
			return 0, 0, 0, false
		}
		lo, hi := out, in
		if k > 0 {
			lo, hi = in, out
		}
		if den := lo - 2*c[best] + hi; den < 0 {
			bestD += max32(min32(0.5*(lo-hi)/den, 0.5), -0.5) * sps / 4
		}
	}
	if !r.locked || p == enums.SyncMSSourcedRC || r.cohRefit {
		bestD = win.mmZero(&syms, end, bestD, sps)
		z = bestD
	} else if z = bestD; r.hasExt() {
		z = win.mmZero(&syms, end, bestD, sps)
	}
	bestD = min(bestD, -end)
	sv, svv, sxv, sp := win.sums(&syms, end+bestD, sps)
	const n = syncSymbols
	sxy := sxv - sp*sv/n
	sxx := svv - sv*sv/n
	spp := 9*n - sp*sp/n
	if sxy <= 0 || sxy*sxy < lim*lim*sxx*spp {
		return 0, 0, 0, false
	}
	r.ringSync(&win, end+bestD, sps, saved)
	return bestD, sxy * sxy / (sxx * spp), min(z, -end), true
}

func (win *syncWindow) mmZero(syms *[syncSymbols]int8, end, d, sps float32) float32 {
	const h = leanLockMMStep
	lo, hi := d-h, d+h
	if end+hi > 0 {
		lo, hi = d-2*h, d
	}
	if end+hi > 0 {
		return d
	}
	a, b := win.mmSum(syms, end+lo, sps), win.mmSum(syms, end+hi, sps)
	if a == b {
		return d
	}
	z := lo - a*(hi-lo)/(b-a)
	if z <= d-2*h || z >= d+2*h {
		return d
	}
	return min(z, -end)
}

func (win *syncWindow) mmSum(syms *[syncSymbols]int8, end, sps float32) float32 {
	var e float32
	prev := win.at(end - float32(syncSymbols-1)*sps)
	m := prev
	for k := 1; k < syncSymbols; k++ {
		y := win.at(end - float32(syncSymbols-1-k)*sps)
		e += float32(syms[k-1])*y - float32(syms[k])*prev
		m += y
		prev = y
	}
	return e - m/syncSymbols*float32(syms[0]-syms[syncSymbols-1])
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
		r.shiftStrobe(d)
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
	r.shiftStrobe(d)
}

func (r *leanCore) shiftStrobe(d float32) {
	r.tm.t += d - r.tm.lastC
	r.tm.lastC, r.tm.stale = 0, true
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
	if r.pendEnd != 0 && r.ns >= r.pendEnd+int64(r.pendWait) {
		r.decidePend()
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
	case r.ns >= leanAcqSymbols && sym:
		r.searchSym(false)
	case r.ns >= leanAcqSymbols && r.rcSigns(r.signs) && r.searchRC():
	case r.ns >= leanRing && r.hasExt():
		if src, data, back, ok := r.ext.scan(r); ok && !r.clipped(r.ns-1-back) {
			r.forceLock(src, data, back)
			r.unproven = true
			r.ext.weakAt(r.ns - 1 - back)
		}
	}
	if mid && !r.locked && !r.dueLock() && r.ns > leanAcqSymbols {
		r.searchMid()
	}
	if !r.locked && !r.dueLock() && r.ns > leanAcqSymbols && r.ns >= r.acqNext && r.rcSigns(r.midSigns) && r.lockMid(1, enums.SyncMSSourcedRC) {
		r.rcLone = true
	}
}

//go:noinline
//go:section .text.rawSym
func (r *leanCore) rawSym(x float32) (hit, sym, mid bool) {
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
	hit = r.ns >= leanAcqSymbols && (sym || r.rcSigns(a)) || r.ns >= leanRing && r.hasExt() || mid && r.ns > leanAcqSymbols
	return hit, sym, mid
}

//go:noinline
//go:section .text.lockSym
func (r *leanCore) lockSym(x, at float32) (again, mid, match bool) {
	r.lastAt = at
	r.pushSym(x)
	if r.hunt && !r.hasExt() && r.midTiming && r.linHunt {
		return r.lockHunt(at)
	}
	if !r.hunt || !r.huntMid(r.lastAt) {
		return false, false, false
	}
	again = r.again
	r.again = false
	sym, mid := r.signsMatch2(r.signs, r.midSigns)
	return again, mid, mid || sym || r.rcHit()
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
	return again, mid, mid || sym || r.rcHit()
}

func (r *leanCore) paired(src int) bool {
	k := r.ns - r.pairNs
	return r.pairSrc == uint8(src)+1 && k >= slotSymbols-1 && k <= leanPairSlots*slotSymbols+1 && (k+1)%slotSymbols <= 2
}

func (r *leanCore) rcHit() bool {
	return r.rcSource() && bits.OnesCount32((r.signs^leanRCSigns)&(1<<syncSymbols-1)) <= int(r.signMiss)
}

func (r *leanCore) rcSigns(signs uint32) bool {
	return r.sources > 1 && bits.OnesCount32((signs^leanRCSigns)&(1<<syncSymbols-1)) <= int(r.signMiss)
}

//go:noinline
func (r *leanCore) searchRC() bool {
	if r.clipped(r.ns-1) || r.ns < r.acqNext {
		return false
	}
	rho2 := r.rcAt(r.ns - 1)
	if rho2 < leanResample*leanResample {
		return false
	}
	p := enums.SyncMSSourcedRC
	var saved [syncSymbols]int8
	d, c2, _, ok := r.resample(p, &saved, false, discLock)
	if ok {
		if ok = c2 >= discLock*discLock && r.fitLevels(r.ns-1, p, true, true); ok {
			r.resampleStart(d)
		} else {
			r.restoreSync(&saved)
		}
	}
	if !ok {
		if rho2 < discLock*discLock || !r.fitLevels(r.ns-1, p, true, true) {
			return false
		}
		d, c2 = 0, 0
	}
	r.lockStream(1, d, c2, 0)
	r.rcLone = true
	return true
}

//go:noinline
func (r *leanCore) rcProof(d *layer2.RawBurst) bool {
	emb := d.StandaloneRCEMB()
	if emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > leanProofErrors {
		return false
	}
	cc, ok := r.colorCode()
	return !ok || emb.ColorCode == cc
}

func (r *leanCore) rcSource() bool {
	return !r.bs && r.src == 1
}

func (r *leanCore) lockPush(x, at float32) (more bool) {
	r.lastAt = at
	r.pushSym(x)
	return r.ns != r.slotEnd
}

func (r *leanCore) endSlot() {
	lagN := float32(leanClockLag)
	if r.bs {
		lagN = leanClockLagBS
	}
	r.tm.settle(lagN)
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
	if !r.huntMid(at) {
		return false, false, false
	}
	again = r.again
	r.again = false
	sym, mid := r.signsMatch2(r.signs, r.midSigns)
	return again, mid, mid || sym || r.rcHit()
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

func (r *leanCore) holding() bool {
	return !r.bs && r.lastGood != r.termAt && r.next-1-r.lastGood <= 2*leanFadeBursts+1
}

func (r *leanCore) held(d float32, src int, data bool, p enums.SyncPattern) bool {
	base := r.slotEnd - slotSymbols
	if !r.locked || !r.holding() || (r.next-r.lastGood)&1 != 0 || r.ns-1 < base+leanInBurst ||
		abs32(d) > leanDeferSkew*2*r.tm.step || !r.fitOK(r.ns-1, p) {
		return false
	}
	if r.ns >= base+leanSyncEnd+1 {
		return r.slotHolds(base)
	}
	if r.pendEnd == 0 {
		r.pendEnd, r.pendWait, r.pendSrc, r.pendData = r.ns-1, uint8(base+leanSyncEnd+2-r.ns), uint8(src), data
	}
	return true
}

func (r *leanCore) fitOK(end int64, p enums.SyncPattern) bool {
	g, _, res := r.fitSync(end, p)
	return tightSync(g, res)
}

//go:noinline
func (r *leanCore) decidePend() {
	end := r.pendEnd
	r.pendEnd = 0
	if r.slotHolds(end + int64(r.pendWait) - leanSyncEnd - 1) {
		return
	}
	src, back := int(r.pendSrc), r.ns-1-end
	if r.hasExt() {
		r.forceLock(src, r.pendData, back)
		return
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if r.pendData {
		p = dp
	}
	r.fitLevels(end, p, true, false)
	r.lockStream(src, 0, 0, back)
	r.forced, r.hunt = true, true
}

func (r *leanCore) slotHolds(base int64) bool {
	for d := int64(-1); d <= 1 && base+leanSyncEnd+d < r.ns; d++ {
		end := base + leanSyncEnd + d
		if rho2, src, _ := r.syncAt(end); src == int(r.src) && rho2 >= discAnchor*discAnchor || r.rcAt(end) >= discAnchor*discAnchor {
			return true
		}
	}
	off := int32(roundf((r.off - r.dcHz) / llrStepHz))
	thr := 2 * int32(roundf(r.devScale*deviationHz/llrStepHz))
	var cw uint16
	for i := range int64(8) {
		k := base + syncFirstSym + i
		if i >= 4 {
			k += syncSymbols - 8
		}
		x := int32(r.ring[ringPos(k)])*(discStepHz/llrStepHz) + off
		cw <<= 2
		if x < 0 {
			cw |= 2
			x = -x
		}
		if x > thr {
			cw |= 1
		}
	}
	e := embOfCW(cw)
	return r.embOK(&e)
}

func (r *leanCore) fitScale() {
	r.relevel(1 / r.devScale)
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
func (r *leanCore) searchFit(p enums.SyncPattern, src int, data, strict bool) (d, c2 float32, ok, hold bool) {
	var saved [syncSymbols]int8
	pairing := !strict && !r.cohVerify()
	lim := float32(discLock)
	if pairing {
		lim = leanPairLock
	}
	if d, c2, _, sampled := r.resample(p, &saved, r.locked, lim); sampled {
		if strict && r.held(d, src, data, p) {
			r.restoreSync(&saved)
			return 0, 0, false, true
		}
		strong := c2 >= discLock*discLock
		ok := strong && r.fitLevels(r.ns-1, p, true, true)
		if !ok && strong && pairing && r.paired(src) {
			ok = r.fitLevels(r.ns-1, p, true, false)
			r.pairLv = ok
		}
		if ok && r.wantRetune(p, src, false) {
			r.restoreSync(&saved)
			r.lockDue = true
			return 0, 0, false, false
		}
		if ok {
			r.resampleStart(d)
			return d, c2, true, false
		}
		r.restoreSync(&saved)
		if pairing {
			r.pairNs, r.pairSrc = r.ns, uint8(src)+1
		}
	}
	if !strict {
		r.acqNext = r.ns + leanAcqWait
	}
	return 0, 0, false, false
}

func (r *leanCore) searchSym(strict bool) {
	if r.dueLock() {
		return
	}
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
	ok, verified := false, false
	var d, c2 float32
	if !weak && (strict || r.ns >= r.acqNext) {
		var hold bool
		if d, c2, ok, hold = r.searchFit(p, src, data, strict); hold {
			return
		}
	}
	if !ok && strict && rho2 >= discLock*discLock && r.held(0, src, data, p) {
		return
	}
	if !ok && r.ns <= leanSyncEnd+1 {
		return
	}
	if r.dueLock() {
		return
	}
	if !ok && (rho2 < discLock*discLock || !r.fitLevels(r.ns-1, p, true, true)) {
		if d, verified = r.weakLock(p, src, rho2, strict); !verified {
			return
		}
		c2 = 0
	}
	if !ok && !verified && r.wantRetune(p, src, false) {
		r.lockDue = true
		return
	}
	r.lockStream(src, d, c2, 0)
	if verified {
		r.unproven = true
		r.ext.weakAt(r.ns - 1)
	}
}

func (r *leanCore) weakLock(p enums.SyncPattern, src int, rho2 float32, strict bool) (float32, bool) {
	if strict || !r.hasExt() || !r.ext.verify(r, p, rho2) {
		return 0, false
	}
	r.fitLevels(r.ns-1, p, true, false)
	if r.wantRetune(p, src, true) {
		r.lockDue = true
		return 0, false
	}
	var d float32
	if t, ok := r.ext.lockShift(r, p, 0); ok {
		d = t * 2 * r.tm.step
		r.shiftStrobe(d)
	}
	return d, true
}

//go:noinline
func (r *leanCore) searchMid() {
	src, data := r.signsSource(r.midSigns)
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	r.lockMid(src, p)
}

func (r *leanCore) lockMid(src int, p enums.SyncPattern) bool {
	at := r.lastAt
	r.lastAt = at - r.tm.step
	var saved [syncSymbols]int8
	d, c2, _, ok := r.resample(p, &saved, false, discLock)
	r.lastAt = at
	if !ok {
		return false
	}
	d -= r.tm.step
	if r.clipped(r.ns-1) || !r.fitLevels(r.ns-1, p, true, true) {
		for i, q := range &saved {
			r.ring[ringPos(r.ns-syncSymbols+int64(i))] = q
		}
		return false
	}
	r.resampleStart(d)
	r.lockStream(src, d, c2, 0)
	return r.locked
}

//go:noinline
func (r *leanCore) clipped(end int64) bool {
	return 2*r.clips(end) > syncSymbols
}

func (r *leanCore) clips(end int64) int {
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
	return n
}

func (r *leanCore) overload(best float32, end int64) bool {
	n := r.clips(end)
	return n >= leanClipSyms && best >= discAnchor*discAnchor || 2*n >= syncSymbols
}

func (r *leanCore) relevel(g float32) {
	g = r.scaleAudio(g)
	r.devScale, r.dcHz, r.discVar = r.devScale*g, r.dcHz*g, r.discVar*g*g
	for i := range r.ms.lv {
		lv := &r.ms.lv[i]
		lv.devScale, lv.dcHz, lv.discVar = lv.devScale*g, lv.dcHz*g, lv.discVar*g*g
	}
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
}

func (r *leanCore) retime(mid bool) {
	rho2, src, data := r.syncAt(r.ns - 1)
	if mid {
		src, data = r.signsSource(r.midSigns)
	}
	voice, dp, _ := sourceAt(src).SyncPatterns()
	p := voice
	if data {
		p = dp
	}
	if !mid && (rho2 < leanExtRho*leanExtRho || src != int(r.src)) {
		if !r.rcSource() || r.rcAt(r.ns-1) < leanExtRho*leanExtRho {
			return
		}
		src, p = int(r.src), enums.SyncMSSourcedRC
	}
	if src != int(r.src) || r.clipped(r.ns-1) {
		return
	}
	if r.reEntry && r.ns-1 != r.slotEnd-slotSymbols+leanSyncEnd {
		return
	}
	var saved [syncSymbols]int8
	if d, c2, z, ok := r.resample(p, &saved, true, discLock); ok {
		if r.hasExt() && r.ns-1 == r.slotEnd-slotSymbols+leanSyncEnd {
			r.ext.liveFit(r, r.ns-1, r.strobeTicks(r.lastAt+z), c2, 1)
		}
		if r.reEntry && !r.cohRefit && p != enums.SyncMSSourcedRC {
			r.restoreSync(&saved)
			sps := 2 * r.tm.step
			r.tm.measure(d-r.tm.lastC, leanRefitVar*(1-c2)/c2*sps*sps)
			if s, ok := r.lineShift(z); ok {
				r.tm.corr += s
				r.shiftStrobe(s + r.tm.lastC)
			}
			return
		}
		if r.reEntry {
			s := d - r.tm.lastC
			sps := 2 * r.tm.step
			r.tm.corr, r.tm.corr2, r.tm.fresh = r.tm.corr+s, r.tm.corr2+leanRefitVar*(1-c2)/c2*sps*sps, 0
		}
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
	d, _, _, ok := r.resample(p, &saved, true, discLock)
	if !ok {
		return
	}
	if r.held(d, src, data, p) {
		r.restoreSync(&saved)
		return
	}
	if !r.clipped(r.ns-1) && r.fitLevels(r.ns-1, p, true, true) {
		r.resampleStart(d)
		r.lockStream(src, d, 0, 0)
		r.forced = true
		return
	}
	r.restoreSync(&saved)
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
	leanReplayFalse  = 4
	leanReplayNear   = 8
	leanSeekRho      = 0.5
	leanRCSigns      = 0xa7c91a
)

func (r *leanCore) seekMatch(d int32, mid bool) {
	rho2, _, _ := r.syncAt(r.ns - 1)
	if !mid && r.rcSource() {
		rho2 = max(rho2, r.rcAt(r.ns-1))
	}
	if !mid && rho2 < leanSeekRho*leanSeekRho {
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
		r.shiftStrobe(d)
	}
	r.lockStream(src, d, 0, back)
	r.forced, r.hunt = true, true
}

func (r *leanCore) lockStream(src int, d, q float32, back int64) {
	if r.dueLock() {
		return
	}
	if r.unproven && r.hasExt() {
		r.ext.end(r)
	}
	r.locked, r.forced, r.suspect, r.hunt, r.src, r.bs = true, false, false, false, uint8(src), sourceAt(src) == layer2.SyncSourceBS
	r.suspectRun = 0
	r.pairSrc = 0
	r.lockNs = r.ns
	r.unproven, r.rcLone = false, false
	r.lastGood, r.termAt, r.pendEnd = 0, leanNoSlot, 0
	r.streams++
	r.first = r.ns - 1 - back - leanSyncEnd
	r.next, r.lastAnch, r.lost = 0, leanNoSlot, 0
	r.ms.anch, r.ms.off, r.ms.moved, r.ms.seek = [2]int32{leanNoSlot, leanNoSlot}, [2]int16{}, false, false
	r.ms.fresh, r.ms.clkAt = [2]uint8{}, 0
	r.slotEnd = r.first + slotSymbols
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.owed, r.tm.fresh = false, r.trackGain, 0, leanClockGain, 0, 1
	r.tm.learned = false
	r.tm.baseN, r.tm.baseC, r.tm.mOK = 0, 0, false
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.estS, r.tm.estW, r.tm.estN = 0, 0, 0
	if r.mmOK && !r.tm.mm {
		r.tm.mm, r.tm.mid = true, false
		r.tm.t += r.tm.step
		r.tm.half = 0
	}
	r.resetChans()
	r.eAcc, r.eSyms = 0, 0
	if r.hasExt() {
		r.ext.locked(r, d)
		r.ext.lockFit(r, r.ns-1-back, r.strobeTicks(r.lastAt+d-float32(back)*2*r.tm.step), q)
	}
}

func (r *leanCore) fitLevels(end int64, p enums.SyncPattern, first, check bool) bool {
	g, dc, res := r.fitSync(end, p)
	r.stepHold = false
	if first && check && !tightSync(g, res) {
		return false
	}
	var step float32
	if !first && g > 0 {
		se := sqrtf(res/(syncSymbols-2)/9) / deviationHz
		z := (g - r.devScale) / max32(se, 1e-6)
		jump := abs32(z) > levelJumpZ
		twice := jump && r.jumpZ*z > 0
		r.jumpZ = 0
		if jump && !twice {
			r.jumpZ = z
		}
		if first = twice || r.heldOff(end, p, res); first {
			r.preDev, r.preOff, r.preStep = r.devScale, r.off-r.dcHz, !r.isReplaying()
			k := max32(g, leanMinScale) / r.devScale
			gk := max32(g, leanMinScale) * deviationHz
			r.tm.pow = 5*gk*gk + res
			step = k
		}
	}
	if first {
		g = max32(g, leanMinScale)
		r.devScale, r.dcHz, r.discVar, r.refit = g, dc, res, false
		if a := r.audio; a != nil {
			t := r.tuneHz
			if int32(uint32(end)-r.tuneNs) < 0 {
				t = r.tunePrev
			}
			r.fitHz = (t + dc) / a.gain
		}
		r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
		if step > 0 && r.hasExt() && !r.isReplaying() {
			r.ext.relevel(step)
		}
		return true
	}
	if check {
		g, dc = r.inlierLevels(end, p, g, dc)
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

func (r *leanCore) syncRes(end int64, p enums.SyncPattern, g, dc float32) float32 {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	j := ringPos(end - syncSymbols + 1)
	var res float32
	for k := range syncSymbols {
		e := float32(r.ring[j])*discStepHz + r.off - dc - g*deviationHz*float32(syms[k])
		if j++; j == leanRing {
			j = 0
		}
		res += e * e
	}
	return res / syncSymbols
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

func (r *leanCore) inlierLevels(end int64, p enums.SyncPattern, g, dc float32) (float32, float32) {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	ref := max(r.devScale, leanMinScale) * deviationHz
	lim := leanSyncClip * ref
	var n, sp, spp, sx, spx float32
	j := ringPos(end - syncSymbols + 1)
	out := 0
	for k := range syncSymbols {
		v := float32(r.ring[j])*discStepHz + r.off
		if j++; j == leanRing {
			j = 0
		}
		s := float32(syms[k])
		if abs32(v-r.dcHz-ref*s) > lim && abs32(v-dc-g*deviationHz*s) > lim {
			out++
			continue
		}
		n++
		sp += s
		spp += s * s
		sx += v
		spx += s * v
	}
	den := n*spp - sp*sp
	if out == 0 || out > leanSyncClipMax || den <= 0 {
		return g, dc
	}
	gd := (n*spx - sp*sx) / den
	return gd / deviationHz, (sx - gd*sp) / n
}

//go:noinline
func (r *leanCore) slot() {
	j := r.next
	r.next++
	base := r.first + int64(j)*slotSymbols + int64(r.ms.off[j&1])
	energy := r.eAcc / float32(max(r.eSyms, 1))
	known := r.eSyms > 0
	r.eAcc, r.eSyms = 0, 0
	measured := false
	if !r.bs {
		var e float32
		if e, measured = r.slotPower(base); measured {
			energy = e
		}
	}
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
	present := isAnchor || !known && !measured || r.noise == 0 && !measured || energy > 1.5*r.noise
	if seek && !isAnchor {
		present = false
		if known || measured {
			r.noise = min(r.noise, energy)
		}
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
	if a := r.audio; audioOn() && a != nil && a.maxGain == 0 && !r.isReplaying() && r.overload(best, base+leanSyncEnd+shift) {
		r.relevel(leanClipDrop)
		r.stepHold = true
		return false, 0, 0
	}
	isAnchor := best >= discAnchor*discAnchor && r.anchors(base+leanSyncEnd+shift, anchor)
	ms := !r.bs && (r.lastAnch != leanNoSlot || base+leanSyncEnd == r.lockNs-1)
	if d, ok := r.rcAnchor(base, ms && !isAnchor); ok {
		isAnchor, shift, anchor = true, d, enums.SyncMSSourcedRC
	}
	if isAnchor && r.isReplaying() {
		if e := r.syncErrors(base+leanSyncEnd+shift, anchor, r.devScale, r.dcHz); r.replayDeep && e > leanReplayFalse || seek && e > leanReplayErrors {
			isAnchor = false
		} else if g, _, res := r.fitSync(base+leanSyncEnd+shift, anchor); g <= 0 || sqrtf(res) > r.ext.replaySpread()*2*g*deviationHz {
			isAnchor = false
		}
	}
	return isAnchor, shift, anchor
}

func (r *leanCore) holdNext(j int32, present bool) {
	r.tm.hold = !r.bs && !present
	if !r.bs && r.lastAnch != leanNoSlot {
		r.tm.hold = r.ms.anch[(j+1)&1] == leanNoSlot
	}
	r.tm.hold = r.tm.hold || r.stepHold
}

func (r *leanCore) slotFirst(j int32, base int64, anchor enums.SyncPattern) {
	o := (j + 1) & 1
	r.ms.lv[o] = r.levels()
	r.fitLevels(base+leanSyncEnd, anchor, true, false)
	r.amp = 0
	if !r.isReplaying() {
		r.ms.fresh[j&1] = 2
		r.saveClock(o)
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
			r.savePhase(p)
		}
		m.lv[p] = r.levels()
	}
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
	if m.anch[p] != leanNoSlot {
		r.swapClock(p, q)
	}
	lo := float32(0)
	if !r.tm.mm && r.tm.mid {
		lo = r.tm.step
	}
	u := m.t[q] + float32(int16(m.n[q]-uint16(r.n))) + float32(int16(uint16(r.ns)-m.ns[q]))*(sps+r.tm.rate)
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
}

func (r *leanCore) saveClock(i int32) {
	m := &r.ms
	m.clk, m.clkAt, m.clkNs = r.tm.leanClock, uint8(i)+1, uint16(r.ns)
	r.tm.newTalker()
}

func (r *leanCore) swapClock(p, q int32) {
	m := &r.ms
	if m.clkAt != uint8(q)+1 {
		r.saveClock(p)
		return
	}
	gap := int32(int16(uint16(r.ns) - m.clkNs))
	r.tm.leanClock, m.clk = m.clk, r.tm.leanClock
	m.clkAt, m.clkNs = uint8(p)+1, uint16(r.ns)
	r.tm.nall += gap
	r.tm.mOK = false
}

func (r *leanCore) anchorAt(j int32, base, shift int64, anchor enums.SyncPattern, energy float32) {
	if (r.pairLv || r.refit) && r.fitLevels(base+leanSyncEnd, anchor, true, true) {
		r.pairLv = false
	} else {
		r.fitLevels(base+leanSyncEnd, anchor, false, true)
	}
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
	r.rcLone = r.rcLone && anchor == enums.SyncMSSourcedRC
	if r.unproven && anchor != enums.SyncMSSourcedRC && r.ext.anchorProves(base+leanSyncEnd) &&
		r.syncErrors(base+leanSyncEnd, anchor, r.devScale, r.dcHz) <= layer2.DefaultSyncThreshold {
		r.unproven = false
	}
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
	r.reEntry = false
	if !r.stale() {
		r.hunt = false
		if !r.ms.seek {
			if r.bs || r.tm.learned && (!r.hasExt() || r.ext.rateSettled(r)) || r.isReplaying() || r.ms.anch[0] != leanNoSlot && r.ms.anch[1] != leanNoSlot {
				return
			}
			r.reEntry = true
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
	if r.syncErrors(end, p, r.devScale, r.dcHz) > discAnchorBits && !r.stepped(end, p) || r.noCarrier(end, p) {
		return false
	}
	return !r.isReplaying() || !r.clipped(end)
}

func (r *leanCore) flatSync(g float32) bool {
	return g < leanSyncScaleMin*r.devScale
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
	if r.flatSync(g) {
		return true
	}
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
	center := float32(0)
	if r.locked {
		center = r.dcHz - d
	} else if r.acqKept() > 0 {
		r.rescaleKept(256, int32(roundf(-d*(256.0/discStepHz))))
	}
	off := r.off - d
	k := roundf((off - center) / discStepHz)
	r.off = off - k*discStepHz
	if k != 0 {
		a8 := int32(k) * 256
		for i, v := range &r.ring {
			r.ring[i] = int8(scale8(int32(v), 256, a8))
		}
		r.rebuildSigns()
		r.preOff -= k * discStepHz
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

func (r *leanCore) stepped(end int64, p enums.SyncPattern) bool {
	g, dc, res := r.fitSync(end, p)
	return tightSync(g, res) && r.heldOff(end, p, res) && r.syncErrors(end, p, g, dc) <= discAnchorBits
}

func tightSync(g, res float32) bool {
	return g > 0 && sqrtf(res) <= discFirstSpread*2*g*deviationHz
}

func (r *leanCore) heldOff(end int64, p enums.SyncPattern, res float32) bool {
	return r.syncRes(end, p, r.devScale, r.dcHz) > levelRefit*max32(res, levelRefitFloor)
}

//go:noinline
func (r *leanCore) syncErrors(end int64, p enums.SyncPattern, g, dc float32) int {
	step := g * deviationHz
	t0 := (dc - r.off) / discStepHz
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
	r.locked, r.forced, r.hunt, r.stepHold, r.preStep = false, false, false, false, false
	r.snap().ok = false
	if a := r.audio; a != nil {
		a.sum, a.sq, a.n, a.qsum, a.qsq = 0, 0, 0, 0, 0
	}
	r.rebuildSigns()
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.owed = false, r.acqGain, 0, 0, 0
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.estS, r.tm.estW, r.tm.estN = 0, 0, 0
	if r.tm.mm {
		r.tm.mm, r.tm.mid = false, true
		r.tm.t -= r.tm.step
	}
	r.offsetHz = r.tuneHz + r.dcHz
	r.devHz = sat16(3 * deviationHz * r.devScale)
	r.devAtHz = sat16(r.fitHz)
}

func sat16(x float32) int16 {
	return int16(max32(min32(roundf(x), math.MaxInt16), math.MinInt16))
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
func (r *leanCore) unreported(b *Burst, anchor enums.SyncPattern, lo, hi int8) bool {
	return anchor == 0 && int32(hi)-int32(lo) <= leanFlatSyms || r.rcLone && anchor == enums.SyncMSSourcedRC && !r.rcProof(&b.Data)
}

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
	b.Distance, b.BS, b.Stream, b.Slot = -1, r.bs, int(r.streams), int64(j)
	b.Sync, b.Voice, b.HasFixed = 0, 0, false
	b.Fixed, b.FixedCACH = layer2.RawBurst{}, [3]byte{}
	ticks := r.burstTicks(base)
	b.Seconds = tickSeconds(ticks)
	b.HasCACH = r.bs && ticks >= cachSymbols*samplesPerSy<<8
	if r.iqLow && iqLowISI > 0 {
		r.cancelISI(base)
	}
	thr, out := 2*r.llrDev, r.llrDev*leanClickOut/2
	split, preOff, preThr := r.stepSplit(base)
	r.preCut = split > 0
	h := ringPos(base)
	var acc byte
	beyond := 0
	lo, hi := int8(math.MaxInt8), int8(math.MinInt8)
	for k := range slotSymbols {
		if k >= cachSymbols {
			lo, hi = min(lo, r.ring[h]), max(hi, r.ring[h])
		}
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
	if split > 0 {
		r.sliceHead(b, base, split, preOff, preThr)
	}
	if r.clickSpan > 0 {
		r.clickFloor(v)
	}
	if r.unreported(b, anchor, lo, hi) {
		return
	}
	if !r.classify(b, j) && (!r.hasExt() || !r.ext.place(r, b, j)) && !r.dataSync(b) {
		if r.hasExt() && !r.isReplayDry() {
			r.ext.unplaced(r, b)
		}
		return
	}
	if r.isReplayDry() {
		r.ext.replayLock(j, b)
	}
	r.lastGood = j
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

func (r *leanCore) burstTicks(base int64) int64 {
	rate := float32(0)
	if r.bs || r.hasExt() && r.ext.rateAgrees(r, base) {
		rate = r.tm.rate
	}
	back := float32(r.ns-1-(base+cachSymbols)) * (2*r.tm.step + rate)
	ticks := (r.mfStep*(r.n-1)+r.center)<<8 +
		int64(int32(roundf(float32(int32(r.mfStep))*(r.lastAt-back)*256))) - int64(r.lagTicks)
	if r.hasExt() && !r.bs {
		if t, ok := r.ext.grid(r, base+cachSymbols); ok {
			ticks = t - int64(r.lagTicks)
		}
	}
	return ticks
}

func (r *leanCore) stepSplit(base int64) (int, int32, int32) {
	if !r.preStep {
		return 0, 0, 0
	}
	r.preStep = false
	off := int32(roundf(r.preOff / llrStepHz))
	dev := int32(roundf(r.preDev * deviationHz / llrStepHz))
	var c, best int32
	split := 0
	h := ringPos(base)
	for k := range syncFirstSym {
		v := int32(r.ring[h]) * (discStepHz / llrStepHz)
		if h++; h == leanRing {
			h = 0
		}
		if c += symRes(v+off, dev) - symRes(v+r.llrOff, r.llrDev); c < best {
			best, split = c, k+1
		}
	}
	return split, off, 2 * dev
}

//go:noinline
func (r *leanCore) sliceHead(b *Burst, base int64, split int, off, thr int32) {
	h := ringPos(base)
	for k := range split {
		x := int32(r.ring[h])*(discStepHz/llrStepHz) + off
		if h++; h == leanRing {
			h = 0
		}
		var d byte
		if x < 0 {
			d = 2
			x = -x
		}
		if x > thr {
			d |= 1
		}
		sh := uint(6 - 2*(k&3))
		if k < cachSymbols {
			b.CACH[k/4] = b.CACH[k/4]&^(3<<sh) | d<<sh
		} else {
			j := (k - cachSymbols) / 4
			b.Data[j] = b.Data[j]&^(3<<sh) | d<<sh
		}
	}
}

func symRes(x, d int32) int32 {
	if x < 0 {
		x = -x
	}
	return min(abs32i(x-d), abs32i(x-3*d))
}

func abs32i(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
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
				if ok && !r.unproven && emb.FEC.ErrorsCorrected == 0 && r.votes[emb.ColorCode&0xF] < math.MaxUint8 {
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
	if pos, ok := r.embPosition(b); ok && !c.data && (!r.isReplaying() || r.ext.replayFits(r, j, pos)) {
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

func (r *leanCore) dataSync(b *Burst) bool {
	_, data, _ := sourceAt(int(r.src)).SyncPatterns()
	dd := syncDist(&b.Data, data)
	if dd > discAnchorBits || r.unproven || r.isReplaying() {
		return false
	}
	rho2, src, neg := r.syncAt(r.base + leanSyncEnd)
	if !neg || src != int(r.src) || rho2 < leanSeekRho*leanSeekRho {
		return false
	}
	if g, _, _ := r.fitSync(r.base+leanSyncEnd, data); r.flatSync(g) {
		return false
	}
	st, res := golay.Decode2087(slotTypeWord(&b.Data))
	if cc, ok := r.colorCode(); !ok || res.Uncorrectable || enums.ColorCode(st>>16) != cc || enums.DataType(st>>12&0xF).IsReserved() {
		return false
	}
	b.Distance, b.Sync = dd, data
	return true
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
	return embOfCW(uint16(d[13]&0xf)<<12 | uint16(d[14]>>4)<<8 | uint16(d[18]&0xf)<<4 | uint16(d[19]>>4))
}

func embOfCW(cw uint16) pdu.EmbeddedSignalling {
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
	if !r.unproven && st.FEC.ErrorsCorrected <= leanVoteErrors && r.votes[st.ColorCode&0xF] < math.MaxUint8 {
		r.votes[st.ColorCode&0xF]++
	}
	if isHeader(st.DataType) {
		c.hdr = j
	}
	if st.DataType == enums.DataTypeTerminatorWithLC {
		r.termAt = j
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

const (
	syncSymbols = framer.SyncBits / 2
	syncEdge    = 2
	numSources  = 4
)

const (
	damagedSyncDistance = 8
	voiceSyncDistance   = 16
)

func sourceAt(i int) layer2.SyncSource {
	switch i {
	case 0:
		return layer2.SyncSourceBS
	case 1:
		return layer2.SyncSourceMS
	case 2:
		return layer2.SyncSourceDirectTS1
	}
	return layer2.SyncSourceDirectTS2
}

func syncSyms(p enums.SyncPattern, out *[syncSymbols]int8) {
	v := uint64(p) << (64 - 2*syncSymbols)
	for k := range syncSymbols {
		d := uint8(v >> 62)
		v <<= 2
		out[k] = symbolOf(unpacked.Bit(d>>1), unpacked.Bit(d&1))
	}
}

// ETSI TS 102 361-1 Table 10.3
func symbolOf(b0, b1 unpacked.Bit) int8 {
	switch b0<<1 | b1 {
	case 0b01:
		return 3
	case 0b00:
		return 1
	case 0b10:
		return -1
	}
	return -3
}

func syncDist(b *layer2.RawBurst, p enums.SyncPattern) int {
	return bits.OnesCount64(ibits.Field(b[:], framer.SyncOffset, framer.SyncBits) ^ uint64(p))
}

func (r *leanCore) strobeTicks(at float32) int64 { return r.ticksAt(r.n, at) }

func (r *leanCore) ticksAt(n int64, at float32) int64 {
	return (r.mfStep*(n-1)+r.center)<<8 + int64(int32(roundf(float32(int32(r.mfStep))*at*256)))
}

func (r *leanCore) lineShift(z float32) (float32, bool) {
	if !r.hasExt() {
		return 0, false
	}
	return r.ext.lineShift(r, r.ns-1, r.strobeTicks(r.lastAt+z))
}
