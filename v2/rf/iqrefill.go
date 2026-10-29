//go:build !tinygo

package rf

import "math"

const (
	iqRefillScale = 8192
	iqRefillHz    = 1000
)

type iqRefill struct {
	raw   [][2]int16
	head  int
	n     int64
	from  int64
	tuned int64
	hz0   float32
	idle  bool
	conv  iqDisc
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
	x.refill = r.refillKept
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
