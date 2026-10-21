package main

import "math"

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
	dc, fast    complex64
	pow         float32
	n, m        float32
	slow, quick float32
	start, cap  float32
}

func newDCBlock(rate float64) *dcBlock {
	return &dcBlock{
		slow: float32(1 / (dcSeconds * rate)), quick: float32(1 / (dcFastSeconds * rate)),
		start: float32(dcStartSeconds * rate), cap: float32(dcSeconds * rate),
	}
}

func (d *dcBlock) remove(v complex64) complex64 {
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
	dec         int
	cic         []float32
	ring        []complex64
	head, ph    int
	dc          *dcBlock
	coef        []float32
	hist        []complex64
	rhead       int
	t, step     float64
	n           int64
	passThrough bool
}

func newResampler(rate, out float64, dc bool) *resampler {
	r := &resampler{}
	r.dec = max(int(rate/(4*out)), 1)
	mid := rate / float64(r.dec)
	if r.dec > 1 {
		box := make([]float64, r.dec)
		for i := range box {
			box[i] = 1
		}
		cic := []float64{1}
		for range cicOrder {
			cic = convolve(cic, box)
		}
		g := math.Pow(float64(r.dec), cicOrder)
		r.cic = make([]float32, len(cic))
		for i := range cic {
			r.cic[i] = float32(cic[i] / g)
		}
		r.ring = make([]complex64, 2*len(r.cic))
	}
	if dc {
		r.dc = newDCBlock(mid)
	}
	r.step = mid / out
	if r.step == 1 {
		r.passThrough = true
		return r
	}
	r.hist = make([]complex64, 2*resampleTaps)
	r.coef = make([]float32, resamplePhases*resampleTaps)
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
			v *= bessel0(beta*math.Sqrt(max(0, 1-x*x))) / bessel0(beta)
			r.coef[p*resampleTaps+j] = float32(v)
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

func bessel0(x float64) float64 {
	s, t := 1.0, 1.0
	for k := 1; k < 50; k++ {
		t *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		s += t
		if t < 1e-12*s {
			break
		}
	}
	return s
}

func (r *resampler) push(v complex64, out []complex64) []complex64 {
	if r.cic != nil {
		l := len(r.cic)
		r.ring[r.head], r.ring[r.head+l] = v, v
		r.head = (r.head + 1) % l
		if r.ph++; r.ph < r.dec {
			return out
		}
		r.ph = 0
		x := r.ring[r.head : r.head+l]
		var re, im float32
		for k, c := range r.cic {
			re += real(x[k]) * c
			im += imag(x[k]) * c
		}
		v = complex(re, im)
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
		var re, im float32
		for k, c := range h {
			re += real(x[k]) * c
			im += imag(x[k]) * c
		}
		out = append(out, complex(re, im))
		r.t += r.step
	}
	r.n++
	return out
}
