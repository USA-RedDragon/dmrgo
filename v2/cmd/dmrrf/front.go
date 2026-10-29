package main

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

const (
	cicOrder       = 3
	resamplePhases = 256
	resampleTaps   = 72
	dcSeconds      = 1
	dcStartSeconds = 0.01
	dcFastSeconds  = 0.05
	dcMove         = 0.1
)

type dcBlock struct {
	dc, fast    complex128
	pow         float64
	n, m        float64
	slow, quick float64
	start, cap  float64
}

func newDCBlock(rate float64) *dcBlock {
	return &dcBlock{slow: 1 / (dcSeconds * rate), quick: 1 / (dcFastSeconds * rate), start: dcStartSeconds * rate, cap: dcSeconds * rate}
}

func (d *dcBlock) remove(v complex128) complex128 {
	d.n = min(d.n+1, d.cap)
	d.m = min(d.m+1, d.cap)
	g := max(d.slow, 1/(d.n+d.start))
	q := max(d.quick, 1/(d.m+d.start))
	d.dc += (v - d.dc) * complex(g, 0)
	d.fast += (v - d.fast) * complex(q, 0)
	e := v - d.fast
	d.pow += (real(e)*real(e) + imag(e)*imag(e) - d.pow) * q
	if x := d.fast - d.dc; real(x)*real(x)+imag(x)*imag(x) > dcMove*d.pow {
		d.dc, d.n = d.fast, min(d.m, 1/d.quick-d.start)
	}
	return v - d.dc
}

type resampler struct {
	rot, osc    complex128
	mixed       int
	dec         int
	cic         []float64
	ring        []complex128
	head, ph    int
	dc          *dcBlock
	coef        []float64
	hist        []complex128
	rhead       int
	t, step     float64
	n           int64
	passThrough bool
}

func newResampler(rate, out, shiftHz float64, dc bool) *resampler {
	r := &resampler{osc: 1}
	if shiftHz != 0 {
		r.rot = cmplx.Exp(complex(0, 2*math.Pi*shiftHz/rate))
	}
	r.dec = max(int(rate/(4*out)), 1)
	mid := rate / float64(r.dec)
	if r.dec > 1 {
		box := make([]float64, r.dec)
		for i := range box {
			box[i] = 1
		}
		r.cic = []float64{1}
		for range cicOrder {
			r.cic = convolve(r.cic, box)
		}
		g := math.Pow(float64(r.dec), cicOrder)
		for i := range r.cic {
			r.cic[i] /= g
		}
		r.ring = make([]complex128, 2*len(r.cic))
	}
	if dc {
		r.dc = newDCBlock(mid)
	}
	r.step = mid / out
	if r.step == 1 {
		r.passThrough = true
		return r
	}
	r.hist = make([]complex128, 2*resampleTaps)
	r.coef = make([]float64, resamplePhases*resampleTaps)
	fc := min(out, mid) / 2 / mid
	const beta = 8.0
	for p := range resamplePhases {
		for j := range resampleTaps {
			u := float64(resampleTaps/2-1-j) + float64(p)/resamplePhases
			v := 2 * fc
			if u != 0 {
				v = math.Sin(2*math.Pi*fc*u) / (math.Pi * u)
			}
			x := u / (resampleTaps / 2)
			v *= resample.Bessel0(beta*math.Sqrt(max(0, 1-x*x))) / resample.Bessel0(beta)
			r.coef[p*resampleTaps+j] = v
		}
	}
	return r
}

func convolve(a, b []float64) []float64 {
	out := make([]float64, len(a)+len(b)-1)
	for i, x := range a {
		for j, y := range b {
			out[i+j] += x * y
		}
	}
	return out
}

func (r *resampler) push(v complex128, out []complex128) []complex128 {
	if r.rot != 0 {
		v *= r.osc
		r.osc *= r.rot
		if r.mixed++; r.mixed == 1024 {
			r.osc /= complex(cmplx.Abs(r.osc), 0)
			r.mixed = 0
		}
	}
	if r.cic != nil {
		l := len(r.cic)
		r.ring[r.head], r.ring[r.head+l] = v, v
		r.head = (r.head + 1) % l
		if r.ph++; r.ph < r.dec {
			return out
		}
		r.ph = 0
		x := r.ring[r.head : r.head+l]
		var acc complex128
		for k, c := range r.cic {
			acc += x[k] * complex(c, 0)
		}
		v = acc
	}
	if r.dc != nil {
		v = r.dc.remove(v)
	}
	if r.passThrough {
		return append(out, v)
	}
	r.hist[r.rhead], r.hist[r.rhead+resampleTaps] = v, v
	r.rhead = (r.rhead + 1) % resampleTaps
	for r.n >= int64(math.Floor(r.t))+resampleTaps/2 {
		frac := r.t - math.Floor(r.t)
		h := r.coef[int(frac*resamplePhases)*resampleTaps:][:resampleTaps]
		x := r.hist[r.rhead : r.rhead+resampleTaps]
		var acc complex128
		for k, c := range h {
			acc += x[k] * complex(c, 0)
		}
		out = append(out, acc)
		r.t += r.step
	}
	r.n++
	return out
}
