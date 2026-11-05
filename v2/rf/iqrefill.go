package rf

import "math"

const (
	iqRefillHz = 1000
)

type iqRefill struct {
	soft     *softExt
	raw      []complex64
	head     int
	n        int64
	from     int64
	tuned    int64
	hz0      float32
	idle     bool
	out      int64
	wide     []complex64
	mixed    []complex128
	settle   bool
	settleHz float32
	conv     iqDisc
	narrow   []float32
	wideTaps []float32
	dd       int64
	drop     int
	narrowOn bool
}

func (r *iqRx) attachRefill(x *softExt) {
	if r.conv.taps == nil || r.conv.halfOn || len(x.acq) == 0 {
		return
	}
	h := 1
	if r.d.f.half {
		h = 2
	}
	warm := len(r.conv.taps) + (2*audio12Taps+halfTaps)*h*r.conv.decim
	lag := min(cap(r.audio)*r.conv.decim, iqAFCBlock) + slotSamplesN + 512
	f := &iqRefill{raw: make([]complex64, 2*len(x.acq)*h*r.conv.decim+warm+lag), tuned: -1}
	f.conv.hist = make([]complex64, 2*len(r.conv.taps))
	if r.tunes == nil {
		f.setNarrow(r)
	}
	r.refill = f
	x.refill, x.rt.want = r.refillKept, r.retuneWanted
	f.soft = x
	if r.cHist != nil {
		f.wide = make([]complex64, int64(r.conv.decim)*int64(len(r.cHist))+2*int64(len(r.conv.coh.taps)))
		f.mixed = make([]complex128, len(f.wide)+2*max(len(r.conv.coh.taps), len(r.conv.taps)))
		x.refillCoh = r.refillCoh
	}
}

// setNarrow makes the narrow channel filter that the receiver decodes through
// once it locks. The wide filter takes a carrier anywhere in the tuning range
// while the receiver hunts; on the tuned carrier, the narrow filter keeps out
// the edge of a DMR channel 12.5 kHz away, whose power 4.5 to 6.25 kHz from
// the carrier is not limited by the adjacent channel power of EN 300 113. It
// is longer by 2*dd outputs, so its output for a sample index comes dd outputs
// later.
func (f *iqRefill) setNarrow(r *iqRx) {
	n := narrowTaps(r.conv.taps)
	d := (len(n) - len(r.conv.taps)) / 2
	if r.conv.step != r.conv.decim || d%r.conv.step != 0 {
		return
	}
	f.narrow, f.wideTaps, f.dd = n, r.conv.taps, int64(d/r.conv.step)
	r.conv.hist = make([]complex64, 2*len(n))
	f.conv.hist = make([]complex64, 2*len(n))
}

func (r *iqRx) settleCoh() {
	f := r.refill
	if f == nil || !f.settle {
		return
	}
	f.settle = false
	d := f.settleHz
	r.tuneLean(d, r.d.f.dc-d*r.d.f.gain*iqUnitsPerHz)
}

func (r *iqRx) unsettledExt() float32 {
	if f := r.refill; f != nil && f.settle {
		return f.settleHz
	}
	return 0
}

func (r *iqRx) noteOutExt() {
	if f := r.refill; f != nil {
		f.out = f.n
	}
}

func (r *iqRx) refillCoh(hz float64) {
	f := r.refill
	scale := r.d.f.gain * iqUnitsPerHz
	if f == nil || f.wide == nil || r.yN == 0 || scale <= 0 {
		return
	}
	d := float32(hz) - r.conv.hz
	if abs32(d) < cohFreqSpan {
		d = 0
	}
	main, coh := r.conv.taps, r.conv.coh.taps
	span := int64(max(len(main), len(coh)))
	lo := max(0, f.n-int64(len(f.wide)))
	per := int64(r.conv.decim)
	k0 := max(r.yN-int64(len(r.yHist)), r.yN-int64(len(r.cHist))+r.cLag, r.yN-1-(f.out-lo-span)/per)
	if k0 >= r.yN || f.out-per*(r.yN-k0)+per-span < lo {
		return
	}
	if d != 0 {
		r.tuneConv(d)
		f.settle, f.settleHz = true, d
	}
	for r.nTunes > 0 && r.tunes[(r.nTunes-1)%iqTunes].n >= k0 {
		r.nTunes--
	}
	r.tunes[r.nTunes%iqTunes] = iqTune{n: k0, hz: r.conv.hz}
	r.nTunes++
	w := 2 * math.Pi * float64(r.conv.fc) / basebandRate
	rot := complex128(r.conv.ncoRot)
	i0 := f.out - per*(r.yN-k0) + per - span
	for i := i0; i < f.out; i++ {
		s, c := math.Sincos(w * float64(f.n-i))
		f.mixed[i-i0] = complex128(f.wide[i%int64(len(f.wide))]) * rot * complex(c, s)
	}
	for k := k0; k < r.yN; k++ {
		e := f.out - 1 - per*(r.yN-1-k)
		p := cmul(r.conv.post, turnBy(r.conv.postW, int(k-r.yN)))
		r.yHist[k&int64(len(r.yHist)-1)] = cmul(f.fir(main, e-i0), p)
		r.cHist[(k-r.cLag)&int64(len(r.cHist)-1)] = cmul(f.fir(coh, e-i0), p)
	}
}

func (f *iqRefill) fir(taps []float32, e int64) complex64 {
	var y complex128
	for j, t := range taps {
		y += complex(float64(t), 0) * f.mixed[e-int64(len(taps)-1-j)]
	}
	return complex64(y)
}

func (r *iqRx) keepRawExt(v complex64) {
	f := r.refill
	switch {
	case r.d.locked && !r.d.hunt:
		f.idle = true
	case f.idle:
		f.idle, f.from = false, f.n
		fallthrough
	default:
		f.raw[f.head] = v
	}
	if f.wide != nil {
		f.wide[f.n%int64(len(f.wide))] = v
	}
	if f.head++; f.head == len(f.raw) {
		f.head = 0
	}
	f.n++
}

func (r *iqRx) noteTuneExt(hz float32) {
	if f := r.refill; f != nil && !r.d.locked {
		if f.tuned < 0 {
			f.hz0 = r.conv.hz - hz
		}
		f.tuned = f.n
	}
}

func (r *iqRx) lockOffset() float32 {
	d := &r.d.f
	if d.gain <= 0 {
		return 0
	}
	return (d.dc + r.d.dcHz) / (d.gain * iqUnitsPerHz)
}

func (r *iqRx) retuneWanted(*leanCore) bool {
	if r.refill == nil || r.tunes != nil || r.d.locked {
		return false
	}
	return r.refill.narrow != nil || r.lockTuned()
}

func (r *iqRx) lockTuned() bool {
	hz := r.lockOffset()
	lim := r.tuneLimit()
	return abs32(hz) >= iqRefillHz && (lim <= 0 || abs32(r.conv.hz+hz-r.center) <= lim)
}

// lockRetune tunes onto the carrier the receiver locks on, puts the narrow
// filter in place of the wide one, and replays the kept acquisition and the
// rest of the audio through the converter at the new tune and filter, so the
// lock burst is decoded the same way as the bursts after it. It returns the
// part of rest that is ready: the narrow filter needs dd more samples for each
// output, so the last dd outputs of rest come from the live converter.
func (r *iqRx) lockRetune(rest []int16) []int16 {
	f := r.refill
	tuned := r.lockTuned()
	r.afcAvg, r.afcPull, r.afcN = 0, 0, 0
	r.conv.acc, r.conv.pw, r.conv.accAvg, r.conv.pwAvg = 0, 0, 0, 0
	if tuned {
		r.tuneWith(r.lockOffset(), 0, r.tuneLimit())
	}
	taps, dd, swap := r.conv.taps, int64(0), false
	if f.narrowOn {
		dd = f.dd
	} else if f.narrow != nil && r.canRegen(rest, f.narrow, f.dd) {
		taps, dd, swap = f.narrow, f.dd, true
	}
	if dd != 0 {
		r.refillWith(&r.d.leanCore, r.d.n, f.wideTaps, 0, true)
		f.soft.wideFit(&r.d.leanCore)
	}
	r.refillWith(&r.d.leanCore, r.d.n, taps, dd, swap || dd != 0)
	if tuned || swap {
		if n, ok := r.regenAudio(rest, taps, dd); ok {
			rest = rest[:n]
		}
	}
	if swap {
		r.toNarrow()
	}
	f.soft.relock(&r.d.leanCore)
	return rest
}

func (r *iqRx) canRegen(rest []int16, taps []float32, dd int64) bool {
	f := r.refill
	per := int64(r.conv.step)
	k0 := min(r.outN-int64(len(rest)), r.outN-dd)
	warm := (int64(len(taps)) + per - 1) / per
	s := (k0 - warm + dd) * per
	return s >= 0 && s >= max(f.n-int64(len(f.raw)), f.from)
}

// regenAudio runs the kept raw samples through a fresh converter with taps,
// whose output for index j comes dd outputs late, and writes its outputs over
// rest, which holds the outputs from r.outN-len(rest) on. It runs up to the
// newest kept sample, so the converter ends where the live one stands, and
// returns how many outputs of rest it wrote.
func (r *iqRx) regenAudio(rest []int16, taps []float32, dd int64) (int, bool) {
	f := r.refill
	per := int64(r.conv.step)
	k0 := r.outN - int64(len(rest))
	warm := (int64(len(taps)) + per - 1) / per
	s := (min(k0, r.outN-dd) - warm + dd) * per
	if s < max(f.n-int64(len(f.raw)), f.from) || s < 0 {
		return 0, false
	}
	cv := &f.conv
	hist := cv.hist
	clear(hist)
	*cv = iqDisc{decim: r.conv.decim, step: r.conv.step, taps: taps, hist: hist, ncoRot: 1, ncoStep: r.conv.ncoStep, hz: r.conv.hz, fc: r.conv.fc, post: 1, postW: r.conv.postW, postRot: r.conv.postRot, floor: r.conv.floor, span: r.conv.span}
	p := f.head - int(f.n-s)
	if p < 0 {
		p += len(f.raw)
	}
	j := s/per - dd
	for i := s; i < f.n; i++ {
		w := f.raw[p]
		if p++; p == len(f.raw) {
			p = 0
		}
		v, _, ok := cv.push(w)
		if !ok {
			continue
		}
		if j >= k0 && j < k0+int64(len(rest)) {
			rest[j-k0] = int16(max(min(roundf(v*iqUnitsPerHz), math.MaxInt16), math.MinInt16))
		}
		j++
	}
	f.drop = int(max(0, k0-j))
	return int(min(int64(len(rest)), max(0, j-k0))), true
}

func (r *iqRx) toNarrow() {
	f := r.refill
	c := &r.conv
	c.taps, c.span = f.narrow, c.span+len(f.narrow)-len(f.wideTaps)
	c.hist, f.conv.hist = f.conv.hist, c.hist
	c.head, c.phase, c.prev, c.ncoRot, c.post = f.conv.head, f.conv.phase, f.conv.prev, f.conv.ncoRot, f.conv.post
	r.outN -= f.dd
	if r.pow != nil {
		r.pow.lag += float64(f.dd) / r.pow.rate
	}
	f.narrowOn = true
}

// toWide puts the wide filter back when the receiver loses the lock. The
// wide filter has the outputs of the next dd indexes already in its reach, so
// toWide writes them from the narrow history before the live converter goes on.
func (r *iqRx) toWide() {
	f := r.refill
	c := &r.conv
	n := len(c.taps)
	h := c.hist[c.head : c.head+n]
	w := f.wideTaps
	m := len(w)
	scale := float32(basebandRate/c.decim) / (2 * math.Pi)
	for t := range f.dd {
		e := n - 1 - c.phase - int(f.dd-1-t)*c.step
		var re, im float32
		for k, v := range w {
			x := h[e-(m-1)+k]
			re += v * real(x)
			im += v * imag(x)
		}
		y := complex(re, im)
		p := cmul(y, complex(real(c.prev), -imag(c.prev)))
		c.prev = y
		if len(r.audio) == cap(r.audio) {
			r.writeAudio()
		}
		k := len(r.audio)
		r.audio = r.audio[:k+1]
		r.audio[k] = int16(max(min(roundf(atan2f(imag(p), real(p))*scale*iqUnitsPerHz), math.MaxInt16), math.MinInt16))
		r.outN++
	}
	copy(c.hist, h[n-m:])
	copy(c.hist[m:], c.hist[:m])
	c.taps, c.head, c.span = w, 0, c.span-n+m
	if r.pow != nil {
		r.pow.lag -= float64(f.dd) / r.pow.rate
	}
	f.narrowOn = false
}

func (r *iqRx) endNarrowExt() bool {
	if f := r.refill; f != nil && f.narrowOn {
		r.toWide()
		return true
	}
	return false
}

func (r *iqRx) writeAudioExt() {
	x := r.audio
	f := r.refill
	for len(x) > 0 {
		if f != nil && f.drop > 0 {
			k := min(f.drop, len(x))
			x, f.drop = x[k:], f.drop-k
			continue
		}
		n := r.d.writeN(x)
		x = x[n:]
		if !r.d.lockDue {
			break
		}
		x = r.lockRetune(x)
		r.d.lockDue = false
	}
	r.settleCoh()
	r.audio = r.audio[:0]
	if f != nil && f.narrowOn && !r.d.locked {
		r.toWide()
	}
}

func (r *iqRx) refillKept(c *leanCore, m int64) {
	var dd int64
	if r.refill.narrowOn {
		dd = r.refill.dd
	}
	r.refillWith(c, m, r.conv.taps, dd, false)
}

// refillWith runs the kept raw samples through a fresh converter with taps,
// whose output for index j comes dd outputs late, and through a copy of the
// lean front, and writes the result over the kept acquisition up to lean
// sample m. It runs when the converter tuned since the samples were kept, or
// when force is set.
func (r *iqRx) refillWith(c *leanCore, m int64, taps []float32, dd int64, force bool) {
	f := r.refill
	if f == nil || f.tuned < 0 && !force {
		return
	}
	if abs32(r.conv.hz-f.hz0) < iqRefillHz && !force {
		f.tuned = -1
		return
	}
	h := int64(1)
	if r.d.f.half {
		h = 2
	}
	per := h * int64(r.conv.decim)
	k := min(int64(c.acqKept()), m-c.acqFrom)
	end := m * per
	if end > f.n || k <= 0 {
		return
	}
	warm := int64(len(taps)) + int64(2*audio12Taps+halfTaps)*per
	s := max((m-k)*per-warm, f.n-int64(len(f.raw)), f.from)
	s = (s + per - 1) / per * per
	if f.tuned <= s && !force {
		return
	}
	cv := &f.conv
	hist := cv.hist
	clear(hist)
	*cv = iqDisc{decim: r.conv.decim, step: r.conv.step, taps: taps, hist: hist, ncoRot: 1, ncoStep: r.conv.ncoStep, hz: r.conv.hz, fc: r.conv.fc, post: 1, postW: r.conv.postW, postRot: r.conv.postRot, floor: r.conv.floor, span: r.conv.span}
	fr := r.d.f
	fr.rrcHist, fr.hbHist, fr.rrcHead, fr.hbHead, fr.odd = [2 * audio12Taps]int8{}, [halfTaps]int8{}, 0, 0, false
	j := s / per
	first := max(m-k, j+warm/per+1)
	if s == 0 {
		first = m - k
	}
	p := f.head - int(f.n-s)
	if p < 0 {
		p += len(f.raw)
	}
	stop := min(end+dd*int64(r.conv.step), f.n)
	for i := s; i < stop; i++ {
		w := f.raw[p]
		if p++; p == len(f.raw) {
			p = 0
		}
		v, _, ok := cv.push(w)
		if !ok {
			continue
		}
		if dd > 0 {
			dd--
			continue
		}
		u := fr.quantize(int16(max(min(roundf(v*iqUnitsPerHz), math.MaxInt16), math.MinInt16)))
		var acc int32
		switch {
		case fr.slow:
			acc = fr.mf96(u)
		case fr.half:
			w, ok := fr.halfBand(u)
			if !ok {
				continue
			}
			acc = fr.mf12(w)
		default:
			acc = fr.mf12(u)
		}
		if j >= first {
			c.setKept(m-1-j, int16(keptQ(acc)))
		}
		j++
	}
	f.tuned = -1
}

func (r *leanCore) setKept(back int64, v int16) {
	buf, an := r.acqBuf()
	if back < 0 || back >= int64(an) {
		return
	}
	h := r.acqHead - 1 - int32(back)
	if h < 0 {
		h += an
	}
	if h == r.acqHead-1 && h&1 == 0 {
		r.acqLow = v
		return
	}
	i := h >> 1
	if i < 0 || i >= int32(len(buf)) {
		return
	}
	w := uint16(buf[i])
	if h&1 != 0 {
		w = w&0xff | uint16(uint8(v))<<8
	} else {
		w = w&0xff00 | uint16(uint8(v))
	}
	buf[i] = int16(w)
}
