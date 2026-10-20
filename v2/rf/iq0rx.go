package rf

import "math"

const iq0Chunk = 32

const iq0LowHz = 9600

type iq0Rx struct {
	d      DiscReceiver
	audio  []int16
	out    []Burst
	prev   complex64
	acc    complex64
	pow    float32
	afcAvg complex64
	step   complex64
	hz     float32
	k, sub float32
	afcN   int
	decim  int
	unit   int
	box    complex64
	phase  int
	poly   int
	pl     *iq0Poly
	dc     dcBlock
	carry  byteCarry
	signed bool
}

type iq0Poly struct {
	psum  [iq0MaxPoly]complex64
	pacc  [iq0MaxPoly - 1]complex64
	ptaps [iq0MaxPoly][iq0MaxDecim]float32
}

func newIQ0Receiver(cfg Config, opt options) *Receiver {
	if opt.IQFront == 4 {
		return &Receiver{iq0: newIQ0Rx(cfg, opt)}
	}
	return &Receiver{iqd: newIQ0(cfg, opt)}
}

func newIQ0Rx(cfg Config, opt options) *iq0Rx {
	hz := opt.IQHz
	if hz == 0 {
		hz = discRate / 2
	}
	r := &iq0Rx{signed: cfg.Signed, decim: cfg.BasebandHz / hz, unit: basebandRate / cfg.BasebandHz, step: 1}
	r.dc.init(float64(cfg.BasebandHz))
	r.k = float32(hz) / (2 * math.Pi) * iqUnitsPerHz
	if opt.IQPoly > 1 {
		r.poly = min(opt.IQPoly, iq0MaxPoly)
		r.pl = &iq0Poly{}
		cut := float32(opt.IQCutHz)
		if cut == 0 {
			cut = discCutHz
		}
		dec := r.decim
		var h [iq0MaxPoly * iq0MaxDecim]float32
		iq0Lowpass(h[:r.poly*dec], cut/basebandRate)
		for j := range r.poly {
			for p := range dec {
				r.pl.ptaps[j][p] = h[j*dec+dec-1-p]
			}
		}
	}
	n := opt.IQBuf
	if n == 0 {
		n = 32
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

func (r *iq0Rx) write(iq []byte) []Burst {
	r.out = r.out[:0]
	pair, iq := r.carry.split(iq)
	var buf [iq0Chunk]complex64
	for _, s := range [2][]byte{pair, iq} {
		for len(s) >= 2 {
			n := min(len(s)/2, iq0Chunk)
			for i := range n {
				a, b := s[2*i], s[2*i+1]
				var v complex64
				if r.signed {
					v = complex(float32(int32(a)<<24>>24)/127.5, float32(int32(b)<<24>>24)/127.5)
				} else {
					v = complex((float32(a)-127.5)/127.5, (float32(b)-127.5)/127.5)
				}
				buf[i] = r.dc.remove(v)
			}
			r.run(buf[:n])
			s = s[2*n:]
		}
	}
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	return r.out
}

func (r *iq0Rx) writeBaseband(x []complex64) []Burst {
	r.out = r.out[:0]
	r.run(x)
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	return r.out
}

func (r *iq0Rx) run(x []complex64) {
	dec := r.decim
	for len(x) > 0 {
		if r.phase > 0 || len(x) < dec {
			r.push(x[0])
			x = x[1:]
			continue
		}
		du := dec * r.unit
		n := min(len(x)/dec, (iqAFCBlock-r.afcN+du-1)/du, cap(r.audio)-len(r.audio))
		if r.poly > 1 {
			r.polyRun(x[:n*dec])
		} else {
			r.boxRun(x[:n*dec])
		}
		x = x[n*dec:]
		r.afcN += n * du
		r.next()
	}
}

func (r *iq0Rx) push(v complex64) {
	p := r.phase
	if r.poly <= 1 {
		r.box += v
	} else {
		for j := range r.poly {
			t := r.pl.ptaps[j][p]
			r.pl.psum[j] += complex(t*real(v), t*imag(v))
		}
	}
	if r.phase++; r.phase < r.decim {
		return
	}
	r.phase = 0
	y := r.box
	r.box = 0
	if r.poly > 1 {
		y = r.pl.pacc[0] + r.pl.psum[0]
		r.pl.pacc[0] = r.pl.psum[1]
		if r.poly == 3 {
			r.pl.pacc[0] += r.pl.pacc[1]
			r.pl.pacc[1] = r.pl.psum[2]
		}
		r.pl.psum = [iq0MaxPoly]complex64{}
	}
	r.one(y)
}

//go:noinline
func (r *iq0Rx) polyRun(x []complex64) {
	dec := r.decim
	n := len(r.audio)
	m := len(x) / dec
	r.audio = r.audio[:n+m]
	out := r.audio[n : n+m]
	pr, pi := real(r.prev), imag(r.prev)
	ar, ai, pow := real(r.acc), imag(r.acc), r.pow
	k, sub, half := r.k, r.sub, r.k*math.Pi
	t0, t1, t2 := r.pl.ptaps[0][:dec], r.pl.ptaps[1][:dec], r.pl.ptaps[2][:dec]
	a0, a1 := r.pl.pacc[0], r.pl.pacc[1]
	three := r.poly == 3
	for i := range out {
		b := x[:dec:dec]
		x = x[dec:]
		var s0r, s0i, s1r, s1i, s2r, s2i float32
		for j, v := range b {
			s0r += t0[j] * real(v)
			s0i += t0[j] * imag(v)
			s1r += t1[j] * real(v)
			s1i += t1[j] * imag(v)
			if three {
				s2r += t2[j] * real(v)
				s2i += t2[j] * imag(v)
			}
		}
		yr, yi := real(a0)+s0r, imag(a0)+s0i
		a0 = complex(s1r, s1i)
		if three {
			a0 += a1
			a1 = complex(s2r, s2i)
		}
		c := yi*pr - yr*pi
		d := yr*pr + yi*pi
		pr, pi = yr, yi
		ar += d
		ai += c
		pow += yr*yr + yi*yi
		out[i] = iq0Freq(c, d, k, sub, half)
	}
	r.prev, r.acc, r.pow = complex(pr, pi), complex(ar, ai), pow
	r.pl.pacc[0], r.pl.pacc[1] = a0, a1
}

func (r *iq0Rx) one(y complex64) {
	pr, pi := real(r.prev), imag(r.prev)
	yr, yi := real(y), imag(y)
	c := yi*pr - yr*pi
	d := yr*pr + yi*pi
	r.prev = y
	r.acc += complex(d, c)
	r.pow += yr*yr + yi*yi
	n := len(r.audio)
	r.audio = r.audio[:n+1]
	r.audio[n] = iq0Freq(c, d, r.k, r.sub, r.k*math.Pi)
	r.afcN += r.decim * r.unit
	r.next()
}

func (r *iq0Rx) next() {
	if len(r.audio) == cap(r.audio) {
		r.d.write(r.audio)
		r.audio = r.audio[:0]
	}
	if r.afcN >= iqAFCBlock {
		r.afcN = 0
		r.follow()
	}
}

//go:noinline
func (r *iq0Rx) boxRun(x []complex64) {
	dec := r.decim
	n := len(r.audio)
	m := len(x) / dec
	r.audio = r.audio[:n+m]
	out := r.audio[n : n+m]
	pr, pi := real(r.prev), imag(r.prev)
	ar, ai, pow := real(r.acc), imag(r.acc), r.pow
	k, sub, half := r.k, r.sub, r.k*math.Pi
	switch dec {
	case 1:
		for i, v := range x[:len(out)] {
			yr, yi := real(v), imag(v)
			c := yi*pr - yr*pi
			d := yr*pr + yi*pi
			pr, pi = yr, yi
			ar += d
			ai += c
			pow += yr*yr + yi*yi
			out[i] = iq0Freq(c, d, k, sub, half)
		}
	case 5:
		for i := range out {
			b := x[:5:5]
			x = x[5:]
			yr := real(b[0]) + real(b[1]) + real(b[2]) + real(b[3]) + real(b[4])
			yi := imag(b[0]) + imag(b[1]) + imag(b[2]) + imag(b[3]) + imag(b[4])
			c := yi*pr - yr*pi
			d := yr*pr + yi*pi
			pr, pi = yr, yi
			ar += d
			ai += c
			pow += yr*yr + yi*yi
			out[i] = iq0Freq(c, d, k, sub, half)
		}
	default:
		for i := range out {
			var yr, yi float32
			for _, v := range x[i*dec : i*dec+dec] {
				yr += real(v)
				yi += imag(v)
			}
			c := yi*pr - yr*pi
			d := yr*pr + yi*pi
			pr, pi = yr, yi
			ar += d
			ai += c
			pow += yr*yr + yi*yi
			out[i] = iq0Freq(c, d, k, sub, half)
		}
	}
	r.prev, r.acc, r.pow = complex(pr, pi), complex(ar, ai), pow
}

func (r *iq0Rx) follow() {
	acc := r.acc * r.step
	p := r.pow
	r.acc, r.pow = 0, 0
	rate := float32(basebandRate / (r.decim * r.unit))
	est := atan2f(imag(acc), real(acc)) * (rate / (2 * math.Pi))
	quiet := real(acc)*real(acc)+imag(acc)*imag(acc) < iq0Coherence*iq0Coherence*p*p
	switch {
	case quiet:
	case r.afcAvg == 0:
		r.afcAvg = acc
	default:
		r.afcAvg += complex(iqAFCAvgGain, 0) * (acc - r.afcAvg)
	}
	avg := atan2f(imag(r.afcAvg), real(r.afcAvg)) * (rate / (2 * math.Pi))
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	scale := f.gain * iqUnitsPerHz
	if r.d.locked {
		hz := (f.dc + r.d.dcHz) / scale
		switch {
		case abs32(avg) >= iqAFCPullHz && abs32(hz) >= iqAFCLockHz && hz*avg > 0:
			r.afcAvg = 0
			r.tune(avg, f.dc-avg*scale)
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < iqAFCPullHz:
			r.tune(hz, f.dc-hz*scale)
		}
		return
	}
	switch {
	case quiet:
	case abs32(est) >= iqAFCPullHz:
		r.afcAvg = 0
		r.tune(est, 0)
	case abs32(avg) >= iqAFCFineHz:
		r.tune(avg, 0)
	}
}

func (r *iq0Rx) tune(hz, dc float32) {
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	r.hz += hz
	rate := float32(basebandRate / (r.decim * r.unit))
	w := -2 * math.Pi * r.hz / rate
	s, c := sincosf(w)
	r.step = complex(c, s)
	r.sub = r.hz * iqUnitsPerHz
	s, c = sincosf(-2 * math.Pi * hz / rate)
	r.afcAvg *= complex(c, s)
	d := hz*scale + dc - f.dc
	if r.d.locked {
		f.mean += dc - f.dc
	} else {
		f.mean = dc
	}
	f.dc, r.d.tuneHz = dc, dc
	f.refresh()
	if !r.d.locked {
		f.shiftHist(d)
		r.d.shift(d)
	}
}

func (r *iq0Rx) flush() []Burst {
	r.out = r.out[:0]
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	r.d.flush()
	return r.out
}

func iq0Freq(c, d, k, sub, half float32) int16 {
	ax, ay := abs32(d), abs32(c)
	n, m := ay, ax
	if ay > ax {
		n, m = ax, ay
	}
	a := n / (m + 1e-30)
	v := a * (math.Pi/4 + 0.273 - 0.273*a)
	if ay > ax {
		v = math.Pi/2 - v
	}
	if d < 0 {
		v = math.Pi - v
	}
	if c < 0 {
		v = -v
	}
	return int16(max(min(roundBits(wrapHalf(v*k-sub, half)), math.MaxInt16), math.MinInt16))
}

func wrapHalf(u, half float32) float32 {
	if u < -half {
		return u + 2*half
	}
	if u > half {
		return u - 2*half
	}
	return u
}
