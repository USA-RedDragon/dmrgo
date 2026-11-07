package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fmath"
	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

const (
	iqTaps         = 31
	iqUnitsPerHz   = 2
	iqAFCBlock     = basebandRate / 20
	iqAFCLockHz    = 100
	iqAFCPullHz    = 300
	iqAFCFineHz    = 60
	iqAFCAvgGain   = 0.25
	iqAFCJumpHz    = 1000
	iqHeldHz       = 2000
	iqAFCCoherence = 0.48
	iqZeroFloor    = 8
	iqZeroRun      = 8
	iqZeroSeconds  = 250e-6
	iqZeroQuiet    = int(basebandRate * iqZeroSeconds)
	iqStopTaps     = 33
	iqStopKaiser   = 8
	iqQuickErrHz   = 235
	iqNarrowTaps   = 89
	iqNarrowKaiser = 3.4
	iqNarrowHz     = 4900
)

type iqDisc struct {
	decim   int
	step    int
	taps    []float32
	hist    []complex64
	head    int
	phase   int
	half    *iqHalf
	prev    complex64
	ncoStep complex64
	ncoRot  complex64
	ncoN    int
	hz      float32
	fc      float32
	post    complex64
	postW   float32
	postRot complex64
	acc     complex64
	pw      float32
	accAvg  complex64
	pwAvg   float32
	halfOn  bool
	afc     []complex64
	afcSum  complex64
	afcPrev complex64
	afcHead int
	coh     *iqCohFIR
	floor   float32
	zeros   int
	span    int
	zrun    int
}

type iqCohFIR struct {
	taps []float32
	hist []complex64
	head int
	y    complex64
}

func newIQCohFIR(n int, beta, cutHz float64) *iqCohFIR {
	f := &iqCohFIR{taps: make([]float32, n), hist: make([]complex64, 2*n)}
	var sum float64
	w := windowTaps(n, cutHz/basebandRate, beta)
	for _, v := range w {
		sum += v
	}
	for i, v := range w {
		f.taps[i] = float32(v / sum)
	}
	return f
}

func (f *iqCohFIR) push(w complex64) {
	n := len(f.taps)
	f.hist[f.head], f.hist[f.head+n] = w, w
	if f.head++; f.head == n {
		f.head = 0
	}
}

func (f *iqCohFIR) out() {
	h := f.hist[f.head : f.head+len(f.taps)]
	var re, im float32
	for k, t := range f.taps {
		re += t * real(h[k])
		im += t * imag(h[k])
	}
	f.y = complex(re, im)
}

type iqFilter struct {
	taps  int
	beta  float64
	cutHz float64
	half  int
	afc   int
	stop  float32
}

type iqHalf struct {
	center float32
	side   []float32
	hist   []complex64
	head   int
	phase  int
}

func (h *iqHalf) init(n int) {
	*h = iqHalf{}
	if n == 0 {
		return
	}
	h.side = make([]float32, (n+1)/4)
	h.hist = make([]complex64, 2*n)
	w := windowTaps(n, 0.25, 7)
	var sum float64
	for _, v := range w {
		sum += v
	}
	h.center = float32(w[n/2] / sum)
	for j := range h.side {
		h.side[j] = float32(w[n/2+2*j+1] / sum)
	}
}

func (h *iqHalf) push(v complex64) (complex64, bool) {
	n := len(h.hist) / 2
	h.hist[h.head], h.hist[h.head+n] = v, v
	if h.head++; h.head == n {
		h.head = 0
	}
	if h.phase ^= 1; h.phase == 1 {
		return 0, false
	}
	x := h.hist[h.head : h.head+n]
	c := n / 2
	y := complex(h.center, 0) * x[c]
	for j, t := range h.side {
		o := 2*j + 1
		y += complex(t, 0) * (x[c-o] + x[c+o])
	}
	return y, true
}

func windowTaps(n int, fc, beta float64) []float64 {
	h := make([]float64, n)
	for i := range h {
		k := float64(i - n/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		if beta > 0 {
			r := 2*float64(i)/float64(n-1) - 1
			w = resample.Bessel0(beta*math.Sqrt(fmath.Max(0, 1-r*r))) / resample.Bessel0(beta)
		}
		h[i] = s * w
	}
	return h
}

func (d *iqDisc) init(decim int, f iqFilter) {
	*d = iqDisc{decim: decim, step: decim, ncoRot: 1, ncoStep: 1, post: 1, postRot: 1}
	n, fc, rate := iqTaps, discCutHz, float64(basebandRate)
	if f.taps > 0 {
		n = f.taps
	}
	if f.cutHz > 0 {
		fc = f.cutHz
	}
	if f.half > 0 && decim%2 == 0 {
		d.half, d.halfOn = new(iqHalf), true
		d.half.init(f.half)
		d.step, rate = decim/2, rate/2
	}
	if f.afc > 1 {
		d.afc = make([]complex64, f.afc)
	}
	w := windowTaps(n, fc/rate, f.beta)
	if f.stop > 0 {
		w = convolveTaps(w, windowTaps(iqStopTaps, float64(f.stop)/rate, iqStopKaiser))
		n = len(w)
	}
	d.taps = make([]float32, n)
	d.hist = make([]complex64, 2*n)
	var sum float64
	for i, v := range w {
		d.taps[i] = float32(v)
		sum += v
	}
	for i := range d.taps {
		d.taps[i] /= float32(sum)
	}
	d.span = len(d.taps)
	if d.halfOn {
		d.span = len(d.half.hist)/2 + 2*len(d.taps)
	}
}

// narrowTaps convolves the wide channel filter with a Kaiser low-pass, so the
// narrow filter passes the same response up to 4 kHz and stops 52 dB at
// 5.5 kHz.
func narrowTaps(wide []float32) []float32 {
	w := make([]float64, len(wide))
	for i, t := range wide {
		w[i] = float64(t)
	}
	n := convolveTaps(w, windowTaps(iqNarrowTaps, float64(iqNarrowHz)/basebandRate, iqNarrowKaiser))
	var sum float64
	for _, v := range n {
		sum += v
	}
	t := make([]float32, len(n))
	for i, v := range n {
		t[i] = float32(v / sum)
	}
	return t
}

func convolveTaps(a, b []float64) []float64 {
	out := make([]float64, len(a)+len(b)-1)
	for i, x := range a {
		for j, y := range b {
			out[i+j] += x * y
		}
	}
	return out
}

func (d *iqDisc) lagTicks() int32 {
	if d.taps == nil {
		return 0
	}
	per, hb := 1, 0
	if d.halfOn {
		per, hb = 2, len(d.half.hist)/2/2-1
	}
	return int32(per*(len(d.taps)/2-(d.step-1))+hb)<<8 + int32(d.decim)<<7
}

func (d *iqDisc) tune(hz float32) { d.tuneAt(hz, hz) }

// tuneAt tunes the converter to hz with the channel filter centered on fc:
// the input is mixed down by fc before the filter and by hz-fc after it.
func (d *iqDisc) tuneAt(hz, fc float32) {
	d.turn(2 * math.Pi * (fc - d.fc) / basebandRate)
	d.hz, d.fc = hz, fc
	p := -2 * math.Pi * float64(fc) / basebandRate
	d.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	d.postW = float32(-2 * math.Pi * float64(hz-fc) * float64(d.decim) / basebandRate)
	s, c := sincosf(d.postW)
	d.postRot = complex(c, s)
}

func turnBy(w float32, age int) complex64 {
	s, c := sincosf(w * float32(age))
	return complex(c, s)
}

func (d *iqDisc) turn(w float32) {
	age0, per := 1, 1
	h := d.half
	nh := 0
	if h != nil {
		nh = len(h.hist) / 2
		age0, per = nh-nh/2+h.phase, 2
	}
	n := len(d.taps)
	ref := age0 + per*(d.phase+n-1-n/2)
	for r := range nh {
		j := (h.head - 1 - r + nh) % nh
		h.hist[j] = cmul(h.hist[j], turnBy(w, r+1-ref))
		h.hist[j+nh] = h.hist[j]
	}
	for r := range n {
		j := (d.head - 1 - r + n) % n
		d.hist[j] = cmul(d.hist[j], turnBy(w, age0+per*r-ref))
		d.hist[j+n] = d.hist[j]
	}
	if c := d.coh; c != nil {
		m := len(c.taps)
		for r := range m {
			j := (c.head - 1 - r + m) % m
			c.hist[j] = cmul(c.hist[j], turnBy(w, age0+per*r-ref))
			c.hist[j+m] = c.hist[j]
		}
	}
	d.ncoRot = cmul(d.ncoRot, turnBy(w, -ref))
	if m := len(d.afc); m > 0 {
		d.afcSum = 0
		for r := range m {
			j := (d.afcHead - 1 - r + m) % m
			d.afc[j] = cmul(d.afc[j], turnBy(w, r*d.decim))
			d.afcSum += d.afc[j]
		}
		d.afcPrev = d.afcSum
	}
}

func (d *iqDisc) push(v complex64) (float32, complex64, bool) {
	if d.zrun = zeroRunOf(v, d.zrun); d.zrun >= iqZeroRun {
		d.zeros = d.span
	} else if d.zeros > 0 {
		d.zeros--
	}
	if d.ncoN++; d.ncoN == 512 {
		d.ncoN = 0
		g := (3 - real(d.ncoRot)*real(d.ncoRot) - imag(d.ncoRot)*imag(d.ncoRot)) / 2
		d.ncoRot *= complex(g, 0)
		g = (3 - real(d.post)*real(d.post) - imag(d.post)*imag(d.post)) / 2
		d.post *= complex(g, 0)
	}
	w := cmul(v, d.ncoRot)
	d.ncoRot = cmul(d.ncoRot, d.ncoStep)
	if d.halfOn {
		var ok bool
		if w, ok = d.half.push(w); !ok {
			return 0, 0, false
		}
	}
	n := len(d.taps)
	d.hist[d.head], d.hist[d.head+n] = w, w
	if d.head++; d.head == n {
		d.head = 0
	}
	if d.coh != nil {
		d.coh.push(w)
	}
	if d.phase++; d.phase < d.step {
		return 0, 0, false
	}
	d.phase = 0
	if d.coh != nil {
		d.coh.out()
	}
	var re, im float32
	if n == iqTaps {
		h := (*[iqTaps]complex64)(d.hist[d.head : d.head+iqTaps])
		for k, t := range (*[iqTaps]float32)(d.taps) {
			re += t * real(h[k])
			im += t * imag(h[k])
		}
	} else {
		h := d.hist[d.head : d.head+n]
		h = h[:n]
		for k, t := range d.taps {
			re += t * real(h[k])
			im += t * imag(h[k])
		}
	}
	y := cmul(complex(re, im), d.post)
	if d.coh != nil {
		d.coh.y = cmul(d.coh.y, d.post)
	}
	d.post = cmul(d.post, d.postRot)
	p := cmul(y, complex(real(d.prev), -imag(d.prev)))
	d.prev = y
	d.acc += p
	d.pw += re*re + im*im
	if d.afc != nil {
		d.afcSum += y - d.afc[d.afcHead]
		d.afc[d.afcHead] = y
		if d.afcHead++; d.afcHead == len(d.afc) {
			d.afcHead, d.afcSum = 0, 0
			for _, a := range d.afc {
				d.afcSum += a
			}
		}
		d.accAvg += cmul(d.afcSum, complex(real(d.afcPrev), -imag(d.afcPrev)))
		d.afcPrev = d.afcSum
		d.pwAvg += real(d.afcSum)*real(d.afcSum) + imag(d.afcSum)*imag(d.afcSum)
	}
	if pw := re*re + im*im; d.zeros > 0 && (pw < d.floor || real(p)*real(p)+imag(p)*imag(p) < d.floor*pw) {
		return 0, y, true
	}
	return atan2f(imag(p), real(p)) * (float32(basebandRate/d.decim) / (2 * math.Pi)), y, true
}

const iqTunes = 16

type iqTune struct {
	n  int64
	hz float32
}

func (r *iqRx) ncoAt(n int64) float32 {
	hz := r.conv.hz
	for i := 0; i < min(r.nTunes, iqTunes); i++ {
		t := r.tunes[(r.nTunes-1-i)%iqTunes]
		if t.n <= n {
			return hz
		}
		hz = 0
		if j := r.nTunes - 2 - i; j >= 0 && i+1 < iqTunes {
			hz = r.tunes[j%iqTunes].hz
		}
	}
	return hz
}

type iqRx struct {
	gs        gainStep
	fe        *frontEnd
	wide      []complex64
	inChunk   int
	dc        dcBlock
	signed    bool
	conv      iqDisc
	d         *discRx
	audio     []int16
	afcN      int
	afcAvg    complex64
	afcPull   float32
	yHist     []complex64
	pow       *iqPower
	cHist     []complex64
	cLag      int64
	specPull  float32
	yN        int64
	refill    *iqRefill
	tunes     *[iqTunes]iqTune
	outN      int64
	nTunes    int
	maxHz     float32
	syncOnly  bool
	pullAgree bool
	spec      *spectrum
	specLock  bool
	wasLocked bool
	lockSeen  bool
	outBand   bool
	center    float32
	adjSide   float32
	fcMove    float32
	devTab    uint8
	raw       int
}

func newIQRx(cfg rxSetup, opt options) *iqRx {
	r := &iqRx{signed: cfg.Signed}
	r.dc.init(float64(cfg.BasebandHz))
	hz := opt.IQHz
	if hz == 0 {
		hz = discRate
	}
	lsb := cfg.Format.lsb() / iqZeroFloor
	r.conv.init(basebandRate/hz, iqFilter{taps: opt.IQTaps, beta: opt.IQKaiser, cutHz: opt.IQCutHz, half: opt.IQHalfband, afc: opt.IQAFCAvg, stop: opt.IQStopHz})
	r.conv.floor = lsb * lsb
	r.maxHz, r.syncOnly, r.pullAgree = opt.IQTuneMaxHz, opt.IQTuneSyncOnly, opt.IQPullAgree
	r.specPull = iqAFCPullHz
	if opt.IQSpecFine {
		r.specPull = iqAFCFineHz
	}
	if r.center = float32(cfg.OffsetHz); r.center != 0 {
		r.conv.tune(r.center)
	}
	if opt.IQSpecHunt {
		r.spec = newSpectrum(float64(cfg.BasebandHz))
	}
	n := opt.IQBuf
	if n == 0 {
		n = 1024
	}
	r.audio = make([]int16, 0, n)
	r.d = cfg.discOf()
	r.d.setup(discSetup{Hz: hz, OnBurst: cfg.OnBurst, opt: &opt, iq: true}, false)
	r.d.sink = cfg.sink
	r.devTab = devTabOf(&opt)
	r.gs.init()
	r.d.lagTicks += r.conv.lagTicks()
	if opt.SoftExt {
		x := softExtOf(&opt, 2*r.d.tm.step)
		if opt.CohExt {
			r.yHist = make([]complex64, opt.cohRing())
			if d := cohWideTaps - len(r.conv.taps); opt.CohWide && !r.conv.halfOn && r.conv.decim == 2 && d >= 0 && d%4 == 0 {
				r.conv.coh = newIQCohFIR(cohWideTaps, cohWideKaiser, cohWideCutHz)
				r.cHist, r.cLag = make([]complex64, opt.cohRing()), int64(d/4)
			}
			r.tunes = new([iqTunes]iqTune)
			x.setCoh(newCohStage(r, &opt))
			r.d.signMiss = opt.signMiss()
		}
		r.d.ext = x
		setExtOn()
		r.attachRefill(x)
	}
	if wantPower(&opt) {
		r.pow = newIQPower(hz, tickSeconds(int64(r.conv.lagTicks())))
		r.d.setPower(r.pow)
	}
	if r.spec != nil {
		r.spec.fast = !opt.SoftExt || !opt.CohExt
	}
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Format, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		if r.fe.rs != nil {
			r.conv.span += resampleTaps
		}
		r.inChunk = max(r.fe.size*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), r.fe.size)
		r.d.lagTicks += r.fe.lag
	}
	return r
}

func (r *iqRx) sample(v complex64) {
	switch {
	case r.adjSide != 0:
		if v, ok := r.gs.push(v); ok {
			r.take(v)
		}
		return
	case r.gs.n > 0:
		r.gs.drain(r.take)
	}
	r.take(v)
}

func (r *iqRx) take(v complex64) {
	if r.spec != nil {
		r.hunt(v)
	}
	if r.afcN++; r.afcN == iqAFCBlock {
		r.afcN = 0
		r.follow()
	}
	f, y, ok := r.conv.push(v)
	if r.refill != nil {
		r.keepRaw(v)
	}
	if !ok {
		return
	}
	if r.pow != nil {
		r.pow.push(y)
	}
	r.outN++
	if r.yHist != nil {
		r.yHist[r.yN&int64(len(r.yHist)-1)] = y
		if r.cHist != nil {
			r.cHist[(r.yN-r.cLag)&int64(len(r.cHist)-1)] = r.conv.coh.y
		}
		r.yN++
		r.noteOut()
	}
	n := len(r.audio)
	r.audio = r.audio[:n+1]
	r.audio[n] = int16(max32(min32(roundf(f*iqUnitsPerHz), math.MaxInt16), math.MinInt16))
	if n+1 == cap(r.audio) {
		r.writeAudio()
	}
}

func (r *iqRx) follow() {
	acc, pw := r.conv.acc, r.conv.pw
	incoherent := real(acc)*real(acc)+imag(acc)*imag(acc) < iqAFCCoherence*iqAFCCoherence*pw*pw
	if r.conv.afc != nil && !r.d.locked {
		acc = r.conv.accAvg
	}
	r.conv.acc, r.conv.pw, r.conv.accAvg, r.conv.pwAvg = 0, 0, 0, 0
	est := r.conv.hzOf(acc)
	if r.afcAvg == 0 {
		r.afcAvg = acc
	} else {
		r.afcAvg += complex(iqAFCAvgGain, 0) * (acc - r.afcAvg)
	}
	avg := r.conv.hzOf(r.afcAvg)
	r.writeAudio()
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	scale := f.gain * iqUnitsPerHz
	if r.d.locked {
		r.afcPull = 0
		hz := (f.dc + r.d.dcHz) / scale
		switch {
		case abs32(avg) >= iqAFCPullHz && !r.syncOnly && abs32(hz) >= iqAFCLockHz && hz*avg > 0:
			r.afcAvg = 0
			r.tune(avg, f.dc-avg*scale)
		case r.syncOnly:
			if int16(r.d.levelEnd-uint16(r.d.tuneNs)) > 0 {
				r.syncTune()
			}
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < max32(iqAFCPullHz, abs32(hz)/2):
			r.tune(hz, f.dc-hz*scale)
		}
		return
	}
	if incoherent || r.outBand {
		r.afcAvg, r.afcPull = 0, 0
		return
	}
	prev := r.afcPull
	r.afcPull = 0
	if abs32(est) >= iqAFCPullHz {
		r.afcPull = est
	}
	switch {
	case abs32(est) >= iqAFCPullHz && (!r.pullAgree || abs32(est) >= iqAFCJumpHz || abs32(prev) >= iqAFCPullHz && est*prev > 0):
		r.afcAvg = 0
		r.tune(est, 0)
	case abs32(avg) >= iqAFCFineHz:
		if r.pullAgree && abs32(avg) >= iqAFCPullHz {
			r.afcAvg = 0
		}
		r.tune(avg, 0)
	}
}

func (r *iqRx) syncTune() {
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	if scale <= 0 || r.d.looseLv {
		return
	}
	if hz := (f.dc + r.d.dcHz) / scale; abs32(hz) >= iqAFCFineHz {
		r.tune(hz, f.dc-hz*scale)
	}
}

func (r *iqRx) lockTune() {
	if !r.d.locked {
		r.lockSeen = false
		return
	}
	if !r.lockSeen && r.syncOnly {
		r.syncTune()
	}
	r.lockSeen = true
}

func (d *iqDisc) hzOf(acc complex64) float32 {
	return atan2f(imag(acc), real(acc)) * (float32(basebandRate/d.decim) / (2 * math.Pi))
}

func (r *iqRx) hunt(v complex64) {
	if r.d.locked {
		if !r.specLock {
			r.spec.reset()
		}
		r.specLock, r.wasLocked = true, true
		return
	}
	r.specLock = false
	if !r.spec.push(v) {
		return
	}
	hz := float32(r.spec.Hz)
	ok := r.spec.OK || r.spec.Seen && abs32(hz-r.center) > r.maxHz-iqQuickErrHz
	r.outBand = ok && abs32(hz-r.center) > iqTuneMaxHz
	if !ok {
		return
	}
	side := float32(0)
	switch {
	case !r.outBand:
	case hz > r.center:
		side = 1
	default:
		side = -1
	}
	if side != r.adjSide {
		r.adjSide = side
		r.conv.tuneAt(r.conv.hz, r.filterAt(r.conv.hz))
		r.d.fcOff = (r.conv.hz - r.conv.fc) * iqUnitsPerHz
	}
	if d := abs32(hz - r.center); d > iqTuneMaxHz || d <= r.maxHz && r.spec.Frames < specFrames/8 && (r.wasLocked || r.refill != nil || d < iqAFCJumpHz) || abs32(hz-r.conv.hz) < r.specPull {
		return
	}
	if r.writeAudio(); r.d.locked {
		return
	}
	r.afcAvg, r.afcPull, r.afcN = 0, 0, 0
	r.conv.acc, r.conv.pw, r.conv.accAvg, r.conv.pwAvg = 0, 0, 0, 0
	r.tuneWith(hz-r.conv.hz, 0, iqTuneMaxHz)
}

func (r *iqRx) lockLimit() float32 { return max32(r.maxHz, iqTuneMaxHz) }

func (r *iqRx) tuneLimit() float32 {
	if r.d.locked {
		return r.lockLimit()
	}
	if r.maxHz == 0 {
		return iqHuntMaxHz
	}
	return r.maxHz
}

func (r *iqRx) tune(hz, dc float32) { r.tuneWith(hz, dc, r.tuneLimit()) }

func (r *iqRx) tuneWith(hz, dc, lim float32) {
	if lim > 0 && abs32(r.conv.hz+hz-r.center) > lim {
		return
	}
	r.tuneConv(hz)
	r.tuneLean(hz, dc)
}

// filterAt returns the center of the channel filter for a converter tuned to
// hz: hz itself, or the channel center when hz lies past it toward an
// adjacent channel that the spectrum hunt found beyond the tuning range. The
// skirt of a strong adjacent channel rises toward it, so the filter keeps
// less of it at the channel center than on a carrier off center toward it.
func (r *iqRx) filterAt(hz float32) float32 {
	if r.adjSide*(hz-r.center) <= 0 {
		return hz
	}
	return r.center
}

func (r *iqRx) tuneConv(hz float32) {
	to, fc := r.conv.hz+hz, r.conv.fc
	r.conv.tuneAt(to, r.filterAt(to))
	r.fcMove = r.conv.fc - fc
	r.d.offsetHz -= hz * r.d.f.gain * iqUnitsPerHz
	r.noteTune(hz)
	if r.tunes != nil {
		r.tunes[r.nTunes%iqTunes] = iqTune{n: r.yN - 1, hz: r.conv.hz}
		r.nTunes++
	}
}

func (r *iqRx) tuneLean(hz, dc float32) {
	r.d.heldOver(hz, iqHeldHz)
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	r.afcPull -= hz
	p := -2 * math.Pi * float64(hz) / float64(basebandRate/r.conv.decim)
	r.afcAvg = cmul(r.afcAvg, complex(float32(math.Cos(p)), float32(math.Sin(p))))
	d := hz*scale + dc - f.dc
	if r.d.locked {
		f.mean += dc - f.dc
		r.d.refit = r.d.refit || abs32(r.fcMove) >= iqAFCPullHz
	} else {
		f.mean = dc
	}
	r.d.tunePrev, r.d.tuneNs = r.d.tuneHz, uint32(r.d.ns)
	r.d.fcPrev, r.d.fcOff = r.d.fcOff, (r.conv.hz-r.conv.fc)*iqUnitsPerHz
	f.dc, r.d.tuneHz = dc, dc
	f.refresh()
	if !r.d.locked {
		f.shiftHist(d)
		r.d.shift(d)
	}
}

func (r *iqRx) writeBaseband(x []complex64) {
	for _, v := range x {
		r.sample(v)
	}
	r.drain()
}

func (r *iqRx) drain() {
	if r.yHist != nil || r.conv.afc != nil {
		return
	}
	r.writeAudio()
}

func (r *iqRx) write(iq []byte) {
	r.bytes(iq)
	r.drain()
}

func (r *iqRx) bytes(iq []byte) {
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

func (r *iqRx) byteSample(i, q byte) complex64 {
	silent := i == 0 && q == 0
	if !r.signed {
		silent = (i == 127 || i == 128) && (q == 127 || q == 128)
	}
	if !silent {
		r.raw = 0
	} else {
		r.raw = min(r.raw+1, iqZeroQuiet)
	}
	var v complex64
	if r.signed {
		v = complex(float32(int8(i))/127.5, float32(int8(q))/127.5)
	} else {
		v = complex((float32(i)-127.5)/127.5, (float32(q)-127.5)/127.5)
	}
	if v = r.dc.remove(v); r.raw >= iqZeroQuiet {
		return 0
	}
	return v
}

func (r *iqRx) flush() {
	r.gs.drain(r.take)
	r.writeAudio()
	if r.endNarrow() {
		r.writeAudio()
	}
	r.d.flushIQ()
}

func zeroRunOf(v complex64, n int) int {
	if v != 0 {
		return 0
	}
	return min(n+1, iqZeroRun)
}
