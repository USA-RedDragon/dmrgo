//go:build !tinygo

package fsktx

import (
	"math"
	"math/bits"
	"math/cmplx"
	"math/rand/v2"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

const (
	dacRate = 96000
	rxRate  = 48000
)

func refDeviation(syms []int8) []float64 {
	const rate = dacRate
	k, err := newPoly(pulse{span: 24}, rate, false)
	if err != nil {
		panic(err)
	}
	buf := make([]float64, rate/SymbolRate+1)
	out := make([]float64, 0, len(syms)*rate/SymbolRate+1)
	for _, s := range syms {
		n := k.symbol(buf, s)
		out = append(out, buf[:n]...)
	}
	return out
}

func fm(freq []float64) []complex128 {
	const rate = dacRate
	x := make([]complex128, len(freq))
	var ph float64
	for i, f := range freq {
		ph += 2 * math.Pi * f / rate
		if ph > math.Pi {
			ph -= 2 * math.Pi
		} else if ph < -math.Pi {
			ph += 2 * math.Pi
		}
		x[i] = cmplx.Rect(1, ph)
	}
	return x
}

const dacLowpassHz = 8000

type radio struct {
	ideal           bool
	acHz            float64
	preHz, preTopHz float64
	gain            float64
}

type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (q *biquad) step(x float64) float64 {
	y := q.b0*x + q.b1*q.x1 + q.b2*q.x2 - q.a1*q.y1 - q.a2*q.y2
	q.x2, q.x1, q.y2, q.y1 = q.x1, x, q.y1, y
	return y
}

func butter2(fc, rate float64) biquad {
	k := math.Tan(math.Pi * fc / rate)
	n := 1 / (1 + math.Sqrt2*k + k*k)
	return biquad{b0: k * k * n, b1: 2 * k * k * n, b2: k * k * n, a1: 2 * (k*k - 1) * n, a2: (1 - math.Sqrt2*k + k*k) * n}
}

func shelf(zHz, pHz, rate float64) biquad {
	z := math.Tan(math.Pi * zHz / rate)
	p := math.Tan(math.Pi * pHz / rate)
	if zHz == 0 {
		n := 1 / (1 + p)
		return biquad{b0: n, b1: -n, a1: (p - 1) * n}
	}
	b0, b1 := 1+1/z, 1-1/z
	a0, a1 := 1+1/p, 1-1/p
	return biquad{b0: b0 / a0, b1: b1 / a0, a1: a1 / a0}
}

func dacFM(a []int16, hz int, r radio) []complex128 {
	hzPerUnit := 3 * DeviationHz / tableUnits
	if r.gain != 0 {
		hzPerUnit *= r.gain
	}
	if r.ideal || dacRate%hz != 0 {
		x := make([]complex128, len(a))
		for i, v := range a {
			x[i] = complex(float64(v)*hzPerUnit, 0)
		}
		y := resampleBand(x, hz, dacRate, 2900, min(float64(hz)-2900, 38000))
		f := make([]float64, len(y))
		for i, v := range y {
			f[i] = real(v)
		}
		return fm(f)
	}
	up := dacRate / hz
	lp := butter2(dacLowpassHz, dacRate)
	var ac, pre biquad
	if r.acHz > 0 {
		ac = shelf(0, r.acHz, dacRate)
	}
	if r.preHz > 0 {
		pre = shelf(r.preHz, r.preTopHz, dacRate)
	}
	f := make([]float64, 0, len(a)*up)
	for _, v := range a {
		x := float64(v) * hzPerUnit
		for range up {
			y := x
			if r.preHz > 0 {
				y = pre.step(y)
			}
			if r.acHz > 0 {
				y = ac.step(y)
			}
			f = append(f, lp.step(y))
		}
	}
	return fm(f)
}

func fromIQ(b []int16) []complex128 {
	x := make([]complex128, len(b)/2)
	for i := range x {
		x[i] = complex(float64(b[2*i]), float64(b[2*i+1]))
	}
	return x
}

func resample(x []complex128, rateIn, rateOut int) []complex128 {
	return resampleBand(x, rateIn, rateOut, 10000, 38000)
}

func resampleBand(x []complex128, rateIn, rateOut int, pass, stop float64) []complex128 {
	g := gcd(rateIn, rateOut)
	l, m := rateOut/g, rateIn/g
	rate := float64(rateIn)
	trans := (stop - pass) / rate
	n := int(math.Ceil((100-8)/(2.285*2*math.Pi*trans))) | 1
	half := n / 2
	fc := (pass + stop) / 2 / rate
	beta := 0.1102 * (100 - 8.7)
	taps := make([][]float64, l)
	for p := range l {
		taps[p] = make([]float64, 2*half+1)
		var sum float64
		for j := -half; j <= half; j++ {
			t := float64(j) - float64(p)/float64(l)
			v := 2 * fc
			if t != 0 {
				v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
			}
			v *= taper(winKaiser, beta, t/float64(half+1))
			taps[p][j+half] = v
			sum += v
		}
		for j := range taps[p] {
			taps[p][j] /= sum
		}
	}
	out := make([]complex128, 0, int(int64(len(x))*int64(l)/int64(m))+1)
	for nOut := 0; ; nOut++ {
		pos := int64(nOut) * int64(m)
		base, p := int(pos/int64(l)), int(pos%int64(l))
		if base >= len(x) {
			break
		}
		var re, im float64
		t := taps[p]
		for j := range t {
			k := base + j - half
			if k < 0 || k >= len(x) {
				continue
			}
			re += real(x[k]) * t[j]
			im += imag(x[k]) * t[j]
		}
		out = append(out, complex(re, im))
	}
	return out
}

func fft(x []complex128) {
	n := len(x)
	b := bits.TrailingZeros(uint(n))
	for i := range n {
		if r := int(bits.Reverse(uint(i)) >> (bits.UintSize - b)); i < r {
			x[i], x[r] = x[r], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Rect(1, -2*math.Pi/float64(size))
		for start := 0; start < n; start += size {
			t := complex(1, 0)
			for k := range size / 2 {
				u := x[start+k]
				v := x[start+k+size/2] * t
				x[start+k], x[start+k+size/2] = u+v, u-v
				t *= w
			}
		}
	}
}

type spectrum struct {
	avg, peak []float64
	binHz     float64
	rate      float64
}

func newSpectrum(x []complex128, rate float64) spectrum {
	n := 1
	for float64(n) < rate/50 {
		n <<= 1
	}
	w := make([]float64, n)
	var wsum float64
	for i := range w {
		t := 2 * math.Pi * float64(i) / float64(n)
		w[i] = 0.35875 - 0.48829*math.Cos(t) + 0.14128*math.Cos(2*t) - 0.01168*math.Cos(3*t)
		wsum += w[i] * w[i]
	}
	s := spectrum{avg: make([]float64, n), peak: make([]float64, n), binHz: rate / float64(n), rate: rate}
	buf := make([]complex128, n)
	frames := 0
	var power float64
	for _, v := range x {
		power += real(v)*real(v) + imag(v)*imag(v)
	}
	power /= float64(len(x))
	for start := 0; start+n <= len(x); start += n / 2 {
		for i := range buf {
			buf[i] = x[start+i] * complex(w[i], 0)
		}
		fft(buf)
		for i, v := range buf {
			p := (real(v)*real(v) + imag(v)*imag(v)) / (float64(n) * wsum) / power
			s.avg[i] += p
			s.peak[i] = max(s.peak[i], p)
		}
		frames++
	}
	for i := range s.avg {
		s.avg[i] /= float64(frames)
	}
	return s
}

func (s spectrum) freq(i int) float64 {
	if i >= len(s.avg)/2 {
		i -= len(s.avg)
	}
	return float64(i) * s.binHz
}

func (s spectrum) band(lo, hi float64) float64 {
	var p float64
	for i, v := range s.avg {
		if f := s.freq(i); f >= lo && f <= hi {
			p += v
		}
	}
	return p
}

// ETSI EN 300 113 clause 7.4 and table C.1
func (s spectrum) acp(offset float64) float64 {
	if offset+4250 > s.rate/2 {
		return math.NaN()
	}
	up := s.band(offset-4250, offset+4250)
	down := s.band(-offset-4250, -offset+4250)
	return 10 * math.Log10(max(up, down))
}

func maskAttn(fd float64) float64 {
	fd = math.Abs(fd) / 1000
	switch {
	case fd <= 5.625:
		return 0
	case fd <= 12.5:
		return 7.27 * (fd - 2.88)
	default:
		return 60
	}
}

func (s spectrum) maskMargin() float64 {
	per := int(math.Round(100 / s.binHz))
	n := len(s.peak)
	margin := math.Inf(1)
	for i := range n {
		f := s.freq(i)
		if math.Abs(f) < 5625 || math.Abs(f) > 62500 || math.Abs(f) > s.rate/2-100 {
			continue
		}
		var p float64
		for j := range max(per, 1) {
			p += s.peak[(i+j-per/2+n)%n]
		}
		a := -10 * math.Log10(p)
		margin = min(margin, a-maskAttn(f))
	}
	return margin
}

func demod(x []complex128, rate float64) []float64 {
	f := make([]float64, len(x))
	for i := 1; i < len(x); i++ {
		f[i] = cmplx.Phase(x[i]*cmplx.Conj(x[i-1])) * rate / (2 * math.Pi)
	}
	return f
}

type fskResult struct {
	Delay    float64
	FSKError float64
	DevError float64
	OffsetHz float64
}

const rxSpan = 16

func fskAt(f []float64, delay float64, lo, hi int) []float64 {
	sps := rxRate / SymbolRate
	d0 := math.Floor(delay)
	frac := delay - d0
	half := rxSpan * sps
	g := make([]float64, 2*half+1)
	var sum float64
	for j := -half; j <= half; j++ {
		t := (float64(j) - frac) / float64(sps)
		g[j+half] = rrc(t)
		sum += g[j+half]
	}
	out := make([]float64, 0, hi-lo)
	for k := lo; k < hi; k++ {
		c := k*sps + int(d0)
		var z float64
		for j := -half; j <= half; j++ {
			if i := c + j; i >= 0 && i < len(f) {
				z += f[i] * g[j+half]
			}
		}
		out = append(out, z/sum)
	}
	return out
}

func fit(z []float64, syms []int8) (fe, gain, off float64) {
	var sx, sy, sxx, sxy float64
	n := float64(len(z))
	for i, v := range z {
		x := float64(syms[i]) * DeviationHz
		sx += x
		sy += v
		sxx += x * x
		sxy += x * v
	}
	gain = (n*sxy - sx*sy) / (n*sxx - sx*sx)
	off = (sy - gain*sx) / n
	var e float64
	for i, v := range z {
		d := v - gain*float64(syms[i])*DeviationHz - off
		e += d * d
	}
	return 100 * math.Sqrt(e/n) / (3 * DeviationHz * gain), gain, off
}

func fsk(x []complex128, syms []int8) fskResult {
	f := demod(x, rxRate)
	sps := rxRate / SymbolRate
	best, bestC := 0, math.Inf(-1)
	for d := 0; d < 80*sps; d++ {
		var c float64
		for k := 40; k < min(len(syms), 3000); k++ {
			if i := k*sps + d; i < len(f) {
				c += float64(syms[k]) * f[i]
			}
		}
		if c > bestC {
			best, bestC = d, c
		}
	}
	lo, hi := 40, min(len(syms)-40, 2000)
	cost := func(delay float64) float64 {
		fe, _, _ := fit(fskAt(f, delay, lo, hi), syms[lo:hi])
		return fe
	}
	a, b := float64(best)-1.5, float64(best)+1.5
	const gr = 0.6180339887498949
	c, d := b-gr*(b-a), a+gr*(b-a)
	fc, fd := cost(c), cost(d)
	for range 40 {
		if fc < fd {
			b, d, fd = d, c, fc
			c = b - gr*(b-a)
			fc = cost(c)
		} else {
			a, c, fc = c, d, fd
			d = a + gr*(b-a)
			fd = cost(d)
		}
	}
	delay := (a + b) / 2
	hi = len(syms) - 40
	fe, gain, off := fit(fskAt(f, delay, lo, hi), syms[lo:hi])
	return fskResult{Delay: delay, FSKError: fe, DevError: 100 * (gain - 1), OffsetHz: off}
}

const rxPad = 0.3

func noisy(x []complex128, shift, n int, cn float64, seed uint64) []complex64 {
	pad := int(rxPad * rxRate)
	var p float64
	mid := x[len(x)/4 : 3*len(x)/4]
	for _, v := range mid {
		p += real(v)*real(v) + imag(v)*imag(v)
	}
	scale := 1 / math.Sqrt(p/float64(len(mid)))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(rxRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	out := make([]complex64, n)
	for i := range out {
		var v complex128
		if k := i - pad + shift; k >= 0 && k < len(x) {
			v = x[k] * complex(scale, 0)
		}
		out[i] = complex64(v + complex(rng.NormFloat64()*sigma, rng.NormFloat64()*sigma))
	}
	return out
}

const discHz = 9600

func discAudio(x []complex64) []int16 {
	const n = 31
	h := make([]float64, n)
	fc := 6250.0 / rxRate
	for i := range h {
		k := float64(i - n/2)
		v := 2 * fc
		if k != 0 {
			v = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		h[i] = v * (0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(n-1)))
	}
	step := rxRate / discHz
	aa := make([]float64, 63)
	afc := 0.42 * discHz / rxRate
	var sum float64
	for i := range aa {
		k := float64(i - len(aa)/2)
		v := 2 * afc
		if k != 0 {
			v = math.Sin(2*math.Pi*afc*k) / (math.Pi * k)
		}
		aa[i] = v * (0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(len(aa)-1)))
		sum += aa[i]
	}
	for i := range aa {
		aa[i] /= sum
	}
	disc := make([]float64, 0, len(x))
	var prev complex128
	for i := 0; i+len(h) <= len(x); i++ {
		var y complex128
		for k, t := range h {
			y += complex128(x[i+k]) * complex(t, 0)
		}
		d := y * complex(real(prev), -imag(prev))
		prev = y
		disc = append(disc, math.Atan2(imag(d), real(d))*rxRate/(2*math.Pi))
	}
	out := make([]int16, 0, len(disc)/step)
	for i := 0; i+len(aa) <= len(disc); i += step {
		var hz float64
		for k, t := range aa {
			hz += disc[i+k] * t
		}
		out = append(out, int16(max(min(math.Round(hz*0.6+300), math.MaxInt16), math.MinInt16)))
	}
	return out
}

func receiveDisc(x []complex64) []rf.Burst {
	var out []rf.Burst
	r, err := rf.NewDisc(&rf.DiscConfig{Hz: discHz, Tier: rf.Tier0, OnBurst: func(b *rf.Burst) { out = append(out, *b) }})
	if err != nil {
		panic(err)
	}
	a := discAudio(x)
	for k := 0; k < len(a); k += 1024 {
		check(r.Write(a[k:min(k+1024, len(a))]))
	}
	r.Flush()
	return out
}

// ETSI TS 102 361-1 clause 10.2.2.3
func etsiDeviation(x []complex128) float64 {
	f := demod(x, rxRate)
	lp := butter2(4000, rxRate)
	var hi, lo float64
	for i, v := range f {
		y := lp.step(v)
		if i > len(f)/4 && i < 3*len(f)/4 {
			hi, lo = max(hi, y), min(lo, y)
		}
	}
	return 100 * (max(hi, -lo)/2749 - 1)
}

func probit(cns []float64, ok, n []int) float64 {
	var sw, sx, sy, sxx, sxy float64
	pts := 0
	for i := range cns {
		r := float64(ok[i]) / float64(max(n[i], 1))
		if r < 0.5 || r > 0.985 {
			continue
		}
		z := math.Sqrt2 * math.Erfinv(2*r-1)
		phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
		w := float64(n[i]) * phi * phi / (r * (1 - r))
		sw += w
		sx += w * cns[i]
		sy += w * z
		sxx += w * cns[i] * cns[i]
		sxy += w * cns[i] * z
		pts++
	}
	if pts < 2 {
		rec := make([]float64, len(cns))
		for i := range cns {
			rec[i] = float64(ok[i]) / float64(max(n[i], 1))
		}
		v, _ := crossing(cns, rec)
		return v
	}
	b := (sw*sxy - sx*sy) / (sw*sxx - sx*sx)
	a := (sy - b*sx) / sw
	return (math.Sqrt2*math.Erfinv(0.8) - a) / b
}

func receive(x []complex64, t rf.Tier) []rf.Burst {
	var out []rf.Burst
	r, err := rf.New(&rf.Config{Rate: rxRate, Tier: t, OnBurst: func(b *rf.Burst) { out = append(out, *b) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(x); k += 4096 {
		check(r.WriteBaseband(x[k:min(k+4096, len(x))]))
	}
	r.Flush()
	return out
}

func crossing(cns, rec []float64) (float64, bool) {
	for i := range rec {
		if rec[i] >= 0.9 {
			if i == 0 {
				return cns[0], false
			}
			a, b := rec[i-1], rec[i]
			return cns[i-1] + (0.9-a)/(b-a)*(cns[i]-cns[i-1]), true
		}
	}
	return math.NaN(), false
}
