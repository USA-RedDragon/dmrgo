package rf

import (
	"encoding/binary"
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

const (
	cicOrder          = 3
	resamplePhases    = 256
	resampleTaps      = 72
	resampleBeta      = 8.0
	wideHz            = 17000.0
	dcSeconds         = 1
	dcStartSeconds    = 0.01
	dcRampSeconds     = 0.001
	dcFastSeconds     = 0.05
	dcMove            = 0.1
	dcEdgeSeconds     = 0.002
	dcCarrierShare    = 0.5
	dcFlat            = 1.2
	dcTurn            = 0.5
	dcFitSeconds      = 0.02
	dcFitGrow         = 2
	dcFitBlockSeconds = 0.004
	dcFitMove         = 0.01
)

type dcBlock struct {
	dc, fast    complex64
	pow         float32
	env, env2   float32
	proj, edge  float32
	n, m        float32
	slow, quick float32
	start, cap  float32
	ramp, prior float32
	fit         dcFit
}

// dcFit fits circles to the I/Q samples of the first dcFitSeconds after
// the start or a reset of the DC blocker, in blocks of dcFitBlockSeconds.
// An FM signal has a constant envelope, so its samples lie on a circle
// around the DC offset; the center of the least squares circle (I. Kasa,
// IEEE Trans. Instrum. Meas. 25(1), 1976) is the offset after a few turns of
// the phase, where a running mean still holds a share of the signal. The
// mean of the centers replaces the DC estimate when it moves it by more than
// a tenth of the radius, and the running mean goes on from it. Each
// block weights its samples by (u*(1-u))^2 over its length, so a signal
// 12.5 kHz away moves the center by under 1e-4 of that signal.
type dcFit struct {
	w, x, y, xx, xy, yy, z, xz, yz, zz float32
	cx, cy, blocks, r2                 float32
	u, du, k, end                      float32
}

func (d *dcBlock) init(rate float64) {
	d.slow = float32(1 / (dcSeconds * rate))
	d.quick = float32(1 / (dcFastSeconds * rate))
	d.start = float32(dcStartSeconds * rate)
	d.cap = float32(dcSeconds * rate)
	d.edge = float32(1 / (dcEdgeSeconds * rate))
	d.ramp = float32(dcRampSeconds * rate)
	d.prior = d.start
	d.fit = dcFit{du: float32(1 / math.Round(dcFitBlockSeconds*rate)), end: float32(dcFitSeconds * rate)}
}

func (f *dcFit) reset() {
	*f = dcFit{du: f.du, end: f.end}
}

func (f *dcFit) clear() {
	f.w, f.x, f.y, f.xx, f.xy, f.yy, f.z, f.xz, f.yz, f.zz, f.u = 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
}

// push adds v to the block and returns the mean of the fitted centers of the
// blocks so far, and the number of those blocks, at the end of a block whose
// fit holds. A fit whose squared radius is over dcFitGrow times the mean
// starts the mean again: the blocks before it held the ramp of the call.
func (f *dcFit) push(v complex64) (complex64, float32, bool) {
	f.k++
	f.u += f.du
	q := f.u * (1 - f.u)
	w := q * q
	x, y := real(v), imag(v)
	z := x*x + y*y
	wx, wy, wz := w*x, w*y, w*z
	f.w, f.x, f.y, f.z = f.w+w, f.x+wx, f.y+wy, f.z+wz
	f.xx, f.xy, f.yy = f.xx+wx*x, f.xy+wx*y, f.yy+wy*y
	f.xz, f.yz, f.zz = f.xz+wx*z, f.yz+wy*z, f.zz+wz*z
	if f.u < 1-f.du/2 {
		return 0, 0, false
	}
	c, r2, ok := f.center()
	f.clear()
	if !ok {
		return 0, 0, false
	}
	if r2 > dcFitGrow*f.r2 {
		f.blocks = 0
	}
	f.blocks++
	f.cx += (real(c) - f.cx) / f.blocks
	f.cy += (imag(c) - f.cy) / f.blocks
	f.r2 += (r2 - f.r2) / f.blocks
	return complex(f.cx, f.cy), f.blocks, true
}

// center returns the center of the fitted circle when the samples go round
// it (the smaller axis of their spread is at least half the larger) and stay
// near it (the spread of the squared radius is at most half of it).
func (f *dcFit) center() (complex64, float32, bool) {
	n := f.w
	if n <= 0 {
		return 0, 0, false
	}
	mx, my, mz := f.x/n, f.y/n, f.z/n
	cxx, cxy, cyy := f.xx/n-mx*mx, f.xy/n-mx*my, f.yy/n-my*my
	cxz, cyz, czz := f.xz/n-mx*mz, f.yz/n-my*mz, f.zz/n-mz*mz
	det := cxx*cyy - cxy*cxy
	if det <= 0 {
		return 0, 0, false
	}
	ax, ay := (cyy*cxz-cxy*cyz)/det, (cxx*cyz-cxy*cxz)/det
	cx, cy := ax/2, ay/2
	r2 := mz - ax*mx - ay*my + cx*cx + cy*cy
	res := czz - ax*cxz - ay*cyz
	if r2 <= 0 || 16*det < r2*r2 || 4*res > r2*r2 {
		return 0, 0, false
	}
	return complex(cx, cy), r2, true
}

// gain returns the gain of update n of a running mean that starts from a
// prior of d.prior samples (d.start at zero, or the samples of the circle
// fits) and weights the first d.ramp samples by a smoothstep from zero to one. A mean with full weight from the first sample
// takes in a signal f Hz away as a step at 0 Hz of about
// 1/(2*pi*f*(n+d.prior)) of that signal, which next to an adjacent channel
// 60 dB stronger is larger than the wanted signal; with the smoothstep the
// step from a signal 9 to 16 kHz away stays under 5e-6 of it after 4 ms.
func (d *dcBlock) gain(n float32) float32 {
	if n >= d.ramp {
		return 1 / (n - d.ramp/2 + d.prior)
	}
	x := n / d.ramp
	return x * x * (3 - 2*x) / (d.ramp*x*x*x*(1-x/2) + d.prior)
}

func (d *dcBlock) remove(v complex64) complex64 {
	p := real(v)*real(v) + imag(v)*imag(v)
	d.env += (p - d.env) * d.edge
	d.env2 += (p*p - d.env2) * d.edge
	d.proj += (real(v)*real(d.dc) + imag(v)*imag(d.dc) - d.proj) * d.edge
	if c := real(d.dc)*real(d.dc) + imag(d.dc)*imag(d.dc); c > dcCarrierShare*d.env && d.env2 < dcFlat*d.env*d.env && d.proj < dcTurn*c {
		d.dc, d.fast, d.n, d.m, d.proj, d.prior = 0, 0, 0, 0, 0, d.start
		d.fit.reset()
	}
	if d.fit.k < d.fit.end {
		if c, b, ok := d.fit.push(v); ok && dcFitMove*d.fit.r2 < real(c-d.dc)*real(c-d.dc)+imag(c-d.dc)*imag(c-d.dc) {
			d.dc, d.fast, d.n, d.m, d.prior = c, c, 0, 0, d.start+b/d.fit.du
			d.proj = real(c)*real(c) + imag(c)*imag(c)
		}
	}
	d.n = min32(d.n+1, d.cap)
	d.m = min32(d.m+1, d.cap)
	g := max32(d.slow, d.gain(d.n))
	q := max32(d.quick, d.gain(d.m))
	d.dc += (v - d.dc) * complex(g, 0)
	d.fast += (v - d.fast) * complex(q, 0)
	e := v - d.fast
	d.pow += (real(e)*real(e) + imag(e)*imag(e) - d.pow) * q
	if x := d.fast - d.dc; real(x)*real(x)+imag(x)*imag(x) > dcMove*d.pow {
		d.dc, d.n = d.fast, min32(d.m, 1/d.quick+d.ramp/2-d.prior)
	}
	return v - d.dc
}

type frontEnd struct {
	format IQFormat
	size   int
	dec    int
	lag    int32
	gain   float32
	integ  [cicOrder][2]int64
	comb   [cicOrder][2]int64
	phase  int
	rs     *resample.Complex
	dc     dcBlock
	raw    int
	quiet  int
}

func rawRunOf(s []byte, f IQFormat, n, most int) int {
	silent := true
	if f == IQFormatCU8 {
		silent = (s[0] == 127 || s[0] == 128) && (s[1] == 127 || s[1] == 128)
	} else {
		for _, b := range s {
			silent = silent && b == 0
		}
	}
	if !silent {
		return 0
	}
	return min(n+1, most)
}

func newFrontEnd(rate float64, format IQFormat, out float64) *frontEnd {
	dec := max(int(rate/(4*out)), 1)
	mid := rate / float64(dec)
	f := &frontEnd{format: format, size: format.bytes(), dec: dec}
	f.quiet = max(int(rate*iqZeroSeconds), iqZeroRun) + dec*cicOrder
	f.lag = int32(math.Round(float64(dec-1) / 2 / rate * basebandRate * 256))
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
		f.raw = rawRunOf(in[i:i+f.size], f.format, f.raw, f.quiet)
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
		if v = f.dc.remove(v); f.raw >= f.quiet {
			v = 0
		}
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
