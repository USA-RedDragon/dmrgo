package rf

import (
	"encoding/binary"
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

const (
	cicOrder       = 3
	resamplePhases = 256
	resampleTaps   = 72
	resampleBeta   = 8.0
	wideHz         = 17000.0
	dcSeconds      = 1
	dcStartSeconds = 0.01
	dcFastSeconds  = 0.05
	dcMove         = 0.1
	dcEdgeSeconds  = 0.002
	dcCarrierShare = 0.5
	dcFlat         = 1.2
	dcTurn         = 0.5
)

type dcBlock struct {
	dc, fast    complex64
	pow         float32
	env, env2   float32
	proj, edge  float32
	n, m        float32
	slow, quick float32
	start, cap  float32
}

func (d *dcBlock) init(rate float64) {
	d.slow = float32(1 / (dcSeconds * rate))
	d.quick = float32(1 / (dcFastSeconds * rate))
	d.start = float32(dcStartSeconds * rate)
	d.cap = float32(dcSeconds * rate)
	d.edge = float32(1 / (dcEdgeSeconds * rate))
}

func (d *dcBlock) remove(v complex64) complex64 {
	p := real(v)*real(v) + imag(v)*imag(v)
	d.env += (p - d.env) * d.edge
	d.env2 += (p*p - d.env2) * d.edge
	d.proj += (real(v)*real(d.dc) + imag(v)*imag(d.dc) - d.proj) * d.edge
	if c := real(d.dc)*real(d.dc) + imag(d.dc)*imag(d.dc); c > dcCarrierShare*d.env && d.env2 < dcFlat*d.env*d.env && d.proj < dcTurn*c {
		d.dc, d.fast, d.n, d.m, d.proj = 0, 0, 0, 0, 0
	}
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
	rs     *resample.Complex
	dc     dcBlock
}

func newFrontEnd(rate float64, format IQFormat, out float64) *frontEnd {
	dec := max(int(rate/(4*out)), 1)
	mid := rate / float64(dec)
	f := &frontEnd{format: format, size: format.bytes(), dec: dec}
	f.gain = float32(1 / (frontScale(format) * math.Pow(float64(dec), cicOrder)))
	f.dc.init(mid)
	if step := mid / out; step != 1 {
		h := make([]float32, resamplePhases*resampleTaps)
		resample.Design(h, resampleTaps, resamplePhases, out/2/mid, resampleBeta)
		f.rs = new(resample.Complex)
		f.rs.Init(h, resampleTaps, resamplePhases, false, step, make([]complex64, 2*resampleTaps))
	}
	return f
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
		if f.rs == nil {
			out[w] = v
			w++
			continue
		}
		w += f.rs.Push(v, out[w:w+1])
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
