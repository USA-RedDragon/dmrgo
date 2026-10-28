package rf

import (
	"encoding/binary"
	"math"
)

const (
	cicOrder       = 3
	resamplePhases = 256
	resampleTaps   = 72
	wideHz         = 17000.0
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

func (d *dcBlock) init(rate float64) {
	d.slow = float32(1 / (dcSeconds * rate))
	d.quick = float32(1 / (dcFastSeconds * rate))
	d.start = float32(dcStartSeconds * rate)
	d.cap = float32(dcSeconds * rate)
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

type frontEnd struct {
	format IQFormat
	size   int
	dec    int
	gain   float32
	integ  [cicOrder][2]int64
	comb   [cicOrder][2]int64
	phase  int
	coef   []float32
	hist   []complex64
	head   int
	t      float64
	step   float64
	n      int64
	dc     dcBlock
}

func newFrontEnd(rate float64, format IQFormat, out float64) *frontEnd {
	dec := max(int(rate/(4*out)), 1)
	mid := rate / float64(dec)
	f := &frontEnd{format: format, size: format.bytes(), dec: dec, step: mid / out}
	f.gain = float32(1 / (frontScale(format) * math.Pow(float64(dec), cicOrder)))
	f.dc.init(mid)
	if f.step == 1 {
		return f
	}
	f.hist = make([]complex64, 2*resampleTaps)
	f.coef = make([]float32, resamplePhases*resampleTaps)
	fc := out / 2 / mid
	const beta = 8.0
	for p := range resamplePhases {
		for j := range resampleTaps {
			u := float64(resampleTaps/2-1-j) + float64(p)/resamplePhases
			v := 2 * fc
			if u != 0 {
				v = math.Sin(2*math.Pi*fc*u) / (math.Pi * u)
			}
			r := u / (resampleTaps / 2)
			v *= bessel0(beta*math.Sqrt(max(0, 1-r*r))) / bessel0(beta)
			f.coef[p*resampleTaps+j] = float32(v)
		}
	}
	return f
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

func frontScale(format IQFormat) float64 {
	switch format {
	case IQFormatCU8, IQFormatCS8:
		return 255
	case IQFormatCF32:
		return cf32Scale
	case IQFormatDefault, IQFormatCS16:
	}
	return 32768
}

const cf32Scale = 1 << 23

func (f *frontEnd) sample(in []byte) (re, im int64) {
	switch f.format {
	case IQFormatCU8:
		return 2*int64(in[0]) - 255, 2*int64(in[1]) - 255
	case IQFormatCS8:
		return 2 * signed8(in[0]), 2 * signed8(in[1])
	case IQFormatCF32:
		return cf32Fixed(in[0:4]), cf32Fixed(in[4:8])
	case IQFormatDefault, IQFormatCS16:
	}
	return int64(int16(binary.LittleEndian.Uint16(in))), int64(int16(binary.LittleEndian.Uint16(in[2:])))
}

func cf32Fixed(b []byte) int64 {
	v := math.Float32frombits(binary.LittleEndian.Uint32(b))
	switch {
	case v >= 2:
		return 2 * cf32Scale
	case v <= -2:
		return -2 * cf32Scale
	case v == v:
		return int64(v * cf32Scale)
	}
	return 0
}

func (f *frontEnd) process(in []byte, out []complex64) int {
	w := 0
	for i := 0; i+f.size <= len(in); i += f.size {
		re, im := f.sample(in[i:])
		for s := range cicOrder {
			f.integ[s][0] += re
			f.integ[s][1] += im
			re, im = f.integ[s][0], f.integ[s][1]
		}
		f.phase++
		if f.phase < f.dec {
			continue
		}
		f.phase = 0
		for s := range cicOrder {
			re, im, f.comb[s][0], f.comb[s][1] = re-f.comb[s][0], im-f.comb[s][1], re, im
		}
		v := complex(float32(re)*f.gain, float32(im)*f.gain)
		v = f.dc.remove(v)
		if f.coef == nil {
			out[w] = v
			w++
			f.n++
			continue
		}
		f.hist[f.head] = v
		f.hist[f.head+resampleTaps] = v
		f.head = (f.head + 1) % resampleTaps
		if f.n >= int64(math.Floor(f.t))+resampleTaps/2 {
			frac := f.t - math.Floor(f.t)
			h := f.coef[int(frac*resamplePhases)*resampleTaps:][:resampleTaps]
			x := f.hist[f.head : f.head+resampleTaps]
			var acc complex64
			for k, c := range h {
				acc += x[k] * complex(c, 0)
			}
			out[w] = acc
			w++
			f.t += f.step
		}
		f.n++
	}
	return w
}

func signed8(b byte) int64 {
	v := int64(b)
	if v > 127 {
		v -= 256
	}
	return v
}
