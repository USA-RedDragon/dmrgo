//go:build !tinygo

package rf

import "math"

const (
	iqRefillScale = 8192
	iqRefillHz    = 1000
)

type iqRefill struct {
	soft     *softExt
	raw      [][2]int16
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
	f := &iqRefill{raw: make([][2]int16, 2*len(x.acq)*h*r.conv.decim+warm+lag), tuned: -1}
	f.conv.hist = make([]complex64, 2*len(r.conv.taps))
	r.refill = f
	x.refill, x.rt.want = r.refillKept, r.retuneWanted
	f.soft = x
	if r.cHist != nil {
		f.wide = make([]complex64, int64(r.conv.decim)*int64(len(r.cHist))+2*int64(len(r.conv.coh.taps)))
		f.mixed = make([]complex128, len(f.wide)+2*max(len(r.conv.coh.taps), len(r.conv.taps)))
		x.refillCoh, x.settleCoh = r.refillCoh, r.settleCoh
	}
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

func (r *iqRx) unsettled() float32 {
	if f := r.refill; f != nil && f.settle {
		return f.settleHz
	}
	return 0
}

func (r *iqRx) noteOut() {
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
	w := 2 * math.Pi * float64(r.conv.hz) / basebandRate
	rot := complex128(r.conv.ncoRot)
	i0 := f.out - per*(r.yN-k0) + per - span
	for i := i0; i < f.out; i++ {
		s, c := math.Sincos(w * float64(f.n-i))
		f.mixed[i-i0] = complex128(f.wide[i%int64(len(f.wide))]) * rot * complex(c, s)
	}
	fir := func(taps []float32, e int64) complex64 {
		var y complex128
		for j, t := range taps {
			y += complex(float64(t), 0) * f.mixed[e-int64(len(taps)-1-j)-i0]
		}
		return complex64(y)
	}
	for k := k0; k < r.yN; k++ {
		e := f.out - 1 - per*(r.yN-1-k)
		r.yHist[k&int64(len(r.yHist)-1)] = fir(main, e)
		r.cHist[(k-r.cLag)&int64(len(r.cHist)-1)] = fir(coh, e)
	}
}

func (r *iqRx) keepRaw(v complex64) {
	f := r.refill
	switch {
	case r.d.locked && !r.d.hunt:
		f.idle = true
	case f.idle:
		f.idle, f.from = false, f.n
		fallthrough
	default:
		f.raw[f.head] = [2]int16{refillQ(real(v)), refillQ(imag(v))}
	}
	if f.wide != nil {
		f.wide[f.n%int64(len(f.wide))] = v
	}
	if f.head++; f.head == len(f.raw) {
		f.head = 0
	}
	f.n++
}

func refillQ(x float32) int16 {
	return int16(max(min(roundf(x*iqRefillScale), math.MaxInt16), math.MinInt16))
}

func (r *iqRx) noteTune(hz float32) {
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
	hz := r.lockOffset()
	lim := r.tuneLimit()
	return abs32(hz) >= iqRefillHz && (lim <= 0 || abs32(r.conv.hz+hz-r.center) <= lim)
}

func (r *iqRx) lockRetune(rest []int16) {
	hz := r.lockOffset()
	r.afcAvg, r.afcPull, r.afcN = 0, 0, 0
	r.conv.acc, r.conv.pw, r.conv.accAvg, r.conv.pwAvg = 0, 0, 0, 0
	r.tuneWith(hz, 0, r.tuneLimit())
	r.refillKept(&r.d.leanCore, r.d.n)
	r.regenAudio(rest)
	r.refill.soft.relock(&r.d.leanCore)
}

func (r *iqRx) regenAudio(rest []int16) bool {
	f := r.refill
	per := int64(r.conv.step)
	k0 := r.outN - int64(len(rest))
	warm := (int64(len(r.conv.taps)) + per - 1) / per
	s := (k0 - warm) * per
	if s < max(f.n-int64(len(f.raw)), f.from) || k0 < 0 {
		return false
	}
	cv := &f.conv
	hist := cv.hist
	clear(hist)
	*cv = iqDisc{decim: r.conv.decim, step: r.conv.step, taps: r.conv.taps, hist: hist, ncoRot: 1, ncoStep: r.conv.ncoStep, hz: r.conv.hz}
	p := f.head - int(f.n-s)
	if p < 0 {
		p += len(f.raw)
	}
	j := s / per
	for i := s; i < f.n && j < k0+int64(len(rest)); i++ {
		w := f.raw[p]
		if p++; p == len(f.raw) {
			p = 0
		}
		v, _, ok := cv.push(complex(float32(w[0])*(1.0/iqRefillScale), float32(w[1])*(1.0/iqRefillScale)))
		if !ok {
			continue
		}
		if j >= k0 {
			rest[j-k0] = int16(max(min(roundf(v*iqUnitsPerHz), math.MaxInt16), math.MinInt16))
		}
		j++
	}
	return true
}

func (r *iqRx) writeAudio() {
	x := r.audio
	for len(x) > 0 {
		n := r.d.writeN(x)
		x = x[n:]
		if !r.d.lockDue {
			break
		}
		r.lockRetune(x)
		r.d.lockDue = false
	}
	r.audio = r.audio[:0]
}

func (r *iqRx) refillKept(c *leanCore, m int64) {
	f := r.refill
	if f == nil || f.tuned < 0 {
		return
	}
	if abs32(r.conv.hz-f.hz0) < iqRefillHz {
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
	warm := int64(len(r.conv.taps)) + int64(2*audio12Taps+halfTaps)*per
	s := max((m-k)*per-warm, f.n-int64(len(f.raw)), f.from)
	s = (s + per - 1) / per * per
	if f.tuned <= s {
		return
	}
	cv := &f.conv
	hist := cv.hist
	clear(hist)
	*cv = iqDisc{decim: r.conv.decim, step: r.conv.step, taps: r.conv.taps, hist: hist, ncoRot: 1, ncoStep: r.conv.ncoStep, hz: r.conv.hz}
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
	for i := s; i < end; i++ {
		w := f.raw[p]
		if p++; p == len(f.raw) {
			p = 0
		}
		v, _, ok := cv.push(complex(float32(w[0])*(1.0/iqRefillScale), float32(w[1])*(1.0/iqRefillScale)))
		if !ok {
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
			c.setKept(m-1-j, int16(int8((acc+1<<13)>>14)))
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
