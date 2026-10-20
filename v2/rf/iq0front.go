package rf

import "math"

const (
	iq0MaxDecim  = 5
	iq0MaxPoly   = 3
	iq0Coherence = 0.2
)

type iq0Front struct {
	front int
	radHz float32
	box   complex64
	pow   float32
	wts   [iq0MaxDecim]complex64
	poly  int
	psum  [iq0MaxPoly]complex64
	pacc  [iq0MaxPoly]complex64
	back  [iq0MaxPoly]complex64
	ptaps [iq0MaxPoly][iq0MaxDecim]float32
}

func iq0Lowpass(h []float32, fc float32) {
	n := len(h)
	var sum float32
	for i := range h {
		k := float32(i) - float32(n-1)/2
		s := 2 * fc
		if k != 0 {
			sn, _ := sincosf(2 * math.Pi * fc * k)
			s = sn / (math.Pi * k)
		}
		_, c := sincosf(2 * math.Pi * float32(i) / float32(n-1))
		h[i] = s * (0.54 - 0.46*c)
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}
}

func (d *iqDisc) initLean(decim int, opt *options) {
	*d = iqDisc{decim: decim, ncoStep: 1}
	d.front = opt.IQFront
	d.radHz = float32(basebandRate/decim) / (2 * math.Pi)
	for k := range d.wts {
		d.wts[k] = 1
	}
	for j := range d.back {
		d.back[j] = 1
	}
	cut := float32(opt.IQCutHz)
	if cut == 0 {
		cut = discCutHz
	}
	if d.front == 3 {
		d.poly = max(min(opt.IQPoly, iq0MaxPoly), 2)
		var h [iq0MaxPoly * iq0MaxDecim]float32
		iq0Lowpass(h[:d.poly*decim], cut/basebandRate)
		for j := range d.poly {
			for p := range decim {
				d.ptaps[j][p] = h[j*decim+decim-1-p]
			}
		}
	}
}

func (d *iqDisc) tuneLean(hz float32) {
	d.hz = hz
	w := -2 * math.Pi * hz / basebandRate
	s, c := sincosf(w * float32(d.decim))
	d.ncoStep = complex(c, s)
	if d.front >= 2 {
		for k := range d.decim {
			s, c := sincosf(w * float32(k))
			d.wts[k] = complex(c, s)
		}
		for j := range d.poly {
			s, c := sincosf(-w * float32(j*d.decim))
			d.back[j] = complex(c, s)
		}
	}
}

func (d *iqDisc) pushLean(v complex64) (float32, complex64, bool) {
	p := d.phase
	switch d.front {
	case 2:
		d.box += v * d.wts[p]
	case 3:
		u := v * d.wts[p]
		for j := range d.poly {
			t := d.ptaps[j][p]
			d.psum[j] += complex(t*real(u), t*imag(u))
		}
	default:
		d.box += v
	}
	if d.phase++; d.phase < d.decim {
		return 0, 0, false
	}
	d.phase = 0
	y := d.endBlock()
	return d.block(y), y, true
}

func (d *iqDisc) endBlock() complex64 {
	if d.front != 3 {
		y := d.box
		d.box = 0
		return y
	}
	y := d.endPoly(&d.psum)
	d.psum = [iq0MaxPoly]complex64{}
	return y
}

func (d *iqDisc) endPoly(s *[iq0MaxPoly]complex64) complex64 {
	y := d.pacc[0] + s[0]
	n := d.poly
	for j := 1; j < n; j++ {
		d.pacc[j-1] = d.pacc[j] + d.back[j]*s[j]
	}
	d.pacc[n-1] = 0
	return y
}

//go:noinline
func (d *iqDisc) block(y complex64) float32 {
	p := y * complex(real(d.prev), -imag(d.prev)) * d.ncoStep
	d.prev = y
	d.acc += p
	d.pow += real(y)*real(y) + imag(y)*imag(y)
	return atanLean(imag(p), real(p)) * d.radHz
}

func (r *iqRx) writeLean(x []complex64) []Burst {
	r.out = r.out[:0]
	d := &r.conv
	dec := d.decim
	for len(x) > 0 {
		if d.phase != 0 || len(x) < dec {
			if f, _, ok := d.pushLean(x[0]); ok {
				r.leanOut(f)
			}
			x = x[1:]
			continue
		}
		n := min(len(x)/dec, (iqAFCBlock-r.afcN+dec-1)/dec, cap(r.audio)-len(r.audio))
		r.leanRun(x[:n*dec])
		x = x[n*dec:]
		r.afcN += n * dec
		r.leanFlush()
	}
	r.drain()
	return r.out
}

func (r *iqRx) leanOut(f float32) {
	n := len(r.audio)
	r.audio = r.audio[:n+1]
	r.audio[n] = iq0Sample(f * iqUnitsPerHz)
	r.afcN += r.conv.decim
	r.leanFlush()
}

func (r *iqRx) leanFlush() {
	if len(r.audio) == cap(r.audio) {
		r.d.write(r.audio)
		r.audio = r.audio[:0]
	}
	if r.afcN >= iqAFCBlock {
		r.afcN = 0
		r.follow()
	}
}

func iq0Sample(u float32) int16 {
	return int16(int32(u+(1<<15+0.5)) - 1<<15)
}

//go:noinline
func (r *iqRx) leanRun(x []complex64) {
	d := &r.conv
	dec := d.decim
	n := len(r.audio)
	r.audio = r.audio[:n+len(x)/dec]
	out := r.audio[n:]
	prev, acc, pow, step := d.prev, d.acc, d.pow, d.ncoStep
	k := d.radHz * iqUnitsPerHz
	w := d.wts[:dec]
	t0, t1, t2 := d.ptaps[0][:dec], d.ptaps[1][:dec], d.ptaps[2][:dec]
	a0, a1 := d.pacc[0], d.pacc[1]
	b1, b2 := d.back[1], d.back[2]
	for i := range out {
		b := x[i*dec : i*dec+dec]
		var y complex64
		switch {
		case d.front == 1:
			for _, v := range b {
				y += v
			}
		case d.front == 2:
			for j, v := range b {
				y += v * w[j]
			}
		case d.poly == 2:
			var s0, s1 complex64
			for j, v := range b {
				u := v * w[j]
				s0 += complex(t0[j]*real(u), t0[j]*imag(u))
				s1 += complex(t1[j]*real(u), t1[j]*imag(u))
			}
			y = a0 + s0
			a0 = b1 * s1
		default:
			var s0, s1, s2 complex64
			for j, v := range b {
				u := v * w[j]
				s0 += complex(t0[j]*real(u), t0[j]*imag(u))
				s1 += complex(t1[j]*real(u), t1[j]*imag(u))
				s2 += complex(t2[j]*real(u), t2[j]*imag(u))
			}
			y = a0 + s0
			a0, a1 = a1+b1*s1, b2*s2
		}
		p := y * complex(real(prev), -imag(prev)) * step
		prev = y
		acc += p
		pow += real(y)*real(y) + imag(y)*imag(y)
		out[i] = iq0Sample(atanLean(imag(p), real(p)) * k)
	}
	d.prev, d.acc, d.pow = prev, acc, pow
	d.pacc[0], d.pacc[1] = a0, a1
}

func (d *iqDisc) quiet(acc complex64) bool {
	p := d.pow
	d.pow = 0
	return real(acc)*real(acc)+imag(acc)*imag(acc) < iq0Coherence*iq0Coherence*p*p
}

func atanLean(y, x float32) float32 {
	ax, ay := x, y
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	if ax == 0 && ay == 0 {
		return 0
	}
	swap := ay > ax
	n, m := ay, ax
	if swap {
		n, m = ax, ay
	}
	a := n / m
	r := a * (math.Pi/4 + 0.273 - 0.273*a)
	if swap {
		r = math.Pi/2 - r
	}
	if x < 0 {
		r = math.Pi - r
	}
	if y < 0 {
		r = -r
	}
	return r
}

func newIQ0(cfg Config, opt options) *iqRx {
	r := &iqRx{signed: cfg.Signed}
	r.dc.init(float64(cfg.BasebandHz))
	hz := opt.IQHz
	if hz == 0 {
		hz = discRate / 2
	}
	r.conv.initLean(basebandRate/hz, &opt)
	n := opt.IQBuf
	if n == 0 {
		n = 1024
	}
	r.audio = make([]int16, 0, n)
	on := cfg.OnBurst
	if on == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(cfg.BasebandHz*3/100*2)+16)
		on = func(b *Burst) {
			if n := len(r.out); n < cap(r.out) {
				r.out = r.out[:n+1]
				r.out[n] = *b
			}
		}
	}
	if err := r.d.Init(DiscConfig{Hz: hz, OnBurst: on, opt: &opt}); err != nil {
		panic(err)
	}
	return r
}
