package fsktx

import "math"

// Model of the FM radio that an Audio drives, for the out-of-band
// optimizer of AudioConfig.Opt: the zero-order hold of the DAC, an analog
// two-pole Butterworth low pass at radioLowpassHz, and FM at radioHz.
const (
	radioHz        = 96000
	radioLowpassHz = 8000.0
)

// Sizes of the audio optimizer, in samples at the audio rate.
const (
	aoptWin  = 1024
	aoptCtx  = 256
	aoptStep = 512
	aoptFree = aoptWin - aoptCtx
	aoptH    = optLPN / 2
)

// Extra weight of the audio optimizer, in dB, on the power from aoptFarHz
// from the carrier, which takes in the alternate channel.
const (
	aoptFarHz = 17000.0
	aoptFarDb = 20.0
)

// Weights of the audio optimizer on the receivers of EN 300 113 Annex C
// at the widest tolerances, as optACP and optALT: above 0, the power that
// the adjacent (aoptACP) or alternate (aoptALT) channel limit counts
// weighs as much as that of Mask D at that many dB.
const (
	aoptACP = 0.0
	aoptALT = 0.0
)

const (
	// aoptAnyRate makes the audio optimizer take every rate: its radio
	// model runs at the rate times the least power of two that reaches
	// radioHz.
	aoptAnyRate = false
)

// aoptimizer is the optimizer of IQConfig.Shape for audio: it adds a
// correction to the deviation that lowers the power of the FM signal of
// the radio outside the channel, weighted by Emission Mask D. The
// correction is high-passed so that the deviation below optInHz does not
// change, and the radio model maps it to the RF phase, so the optimizer
// sees the spectrum that the radio sends.
type aoptimizer struct {
	iters     int
	up        int
	settle    int
	delay     int
	hz, rhz   float64
	k2        float64
	wts       []float64
	f, x      []float64
	lp        []float64
	w2, taper []float64
	cosT      []float64
	sinT      []float64
	rev       []int
	re, im    []float64
	xa, e     []float64
	gout, ge  []float64
	xv, g, d  []float64
	xn, gn    []float64
	s, y      [optMem][]float64
	rho       [optMem]float64
	alpha     [optMem]float64
	nIn, nBlk int
	jscale    float64
}

// aoptRate reports whether the audio optimizer takes the rate hz: radioHz
// is hz times a power of two.
func aoptRate(hz int) bool {
	if aoptAnyRate && hz > 0 {
		return true
	}
	if hz <= 0 || radioHz%hz != 0 {
		return false
	}
	up := radioHz / hz
	return up&(up-1) == 0
}

// newAudioOpt returns an aoptimizer for audio at hz, which aoptRate
// takes, with iters iterations per block.
func newAudioOpt(iters, hz int) *aoptimizer {
	up := 1
	for hz*up < radioHz {
		up *= 2
	}
	rhz := float64(hz * up)
	n := aoptWin * up
	o := &aoptimizer{iters: iters, up: up, hz: float64(hz), rhz: rhz, k2: 2 * math.Pi / radioHz}
	if up*hz != radioHz {
		o.k2 = 2 * math.Pi / rhz
	}
	w := 2 * math.Pi * radioLowpassHz
	al := w / math.Sqrt2
	hold := 1 / float64(hz)
	ramp := func(t float64) float64 {
		if t <= 0 {
			return 0
		}
		return t - (1-math.Exp(-al*t)*math.Cos(al*t))/al
	}
	rise := func(t float64) float64 { return ramp(t) - ramp(t-hold) }
	o.settle = int(math.Ceil((36/al+hold)*float64(hz))) + 1
	o.wts = make([]float64, up*o.settle)
	for p := range up {
		end := float64(p+1) / rhz
		for j := range o.settle {
			t := end + float64(j)*hold
			o.wts[p*o.settle+j] = (rise(t) - rise(t-1/rhz)) * rhz
		}
	}
	sym := hz / gcd(hz, 4800)
	o.delay = (aoptWin - aoptCtx - 1 + aoptH + sym - 1) / sym * sym
	o.f = make([]float64, optRing)
	o.x = make([]float64, optRing)
	o.lp = make([]float64, optLPN)
	var sum float64
	fc := optLPHz / float64(hz)
	for i := range o.lp {
		t := float64(i - aoptH)
		v := 2 * fc
		if t != 0 {
			v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
		}
		u := t / float64(aoptH+1)
		v *= besselI0(optLPBeta*math.Sqrt(max(0, 1-u*u))) / besselI0(optLPBeta)
		o.lp[i] = v
		sum += v
	}
	for i := range o.lp {
		o.lp[i] /= sum
	}
	o.w2 = make([]float64, n)
	o.taper = make([]float64, n)
	for k := range n {
		fk := float64(k) * rhz / float64(n)
		if k >= n/2 {
			fk -= rhz
		}
		if a := math.Abs(fk); a > optFromHz {
			db := optMaskAttn(a)
			if a > aoptFarHz {
				db += aoptFarDb
			}
			o.w2[k] = math.Pow(10, db/10)
			if aoptACP > 0 {
				o.w2[k] += math.Pow(10, (aoptACP-optAnnexC(a, 12500))/10)
			}
			if aoptALT > 0 {
				o.w2[k] += math.Pow(10, (aoptALT-optAnnexC(a, 25000))/10)
			}
		}
		o.taper[k] = 0.5 - 0.5*math.Cos(2*math.Pi*(float64(k)+0.5)/float64(n))
	}
	o.cosT = make([]float64, n/2)
	o.sinT = make([]float64, n/2)
	for k := range o.cosT {
		o.sinT[k], o.cosT[k] = math.Sincos(-2 * math.Pi * float64(k) / float64(n))
	}
	o.rev = make([]int, n)
	bits := 0
	for 1<<bits < n {
		bits++
	}
	for i := range o.rev {
		r := 0
		for b := range bits {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		o.rev[i] = r
	}
	o.re, o.im = make([]float64, n), make([]float64, n)
	o.gout = make([]float64, n)
	o.xa = make([]float64, aoptWin+2*aoptH+o.settle)
	o.e = make([]float64, aoptWin+o.settle)
	o.ge = make([]float64, aoptWin+o.settle)
	o.xv, o.g, o.d = make([]float64, aoptFree), make([]float64, aoptFree), make([]float64, aoptFree)
	o.xn, o.gn = make([]float64, aoptFree), make([]float64, aoptFree)
	for i := range optMem {
		o.s[i], o.y[i] = make([]float64, aoptFree), make([]float64, aoptFree)
	}
	return o
}

// process takes the deviation in buf and replaces it with the delayed
// corrected deviation.
func (o *aoptimizer) process(buf []float64) {
	for i, v := range buf {
		o.f[o.nIn&(optRing-1)] = v
		o.x[o.nIn&(optRing-1)] = 0
		o.nIn++
		if o.nIn == o.nBlk*aoptStep+aoptWin-aoptCtx {
			o.block(o.nBlk*aoptStep - aoptCtx)
			o.nBlk++
		}
		m := o.nIn - 1 - o.delay
		if m < 0 {
			buf[i] = 0
			continue
		}
		var acc float64
		for k, c := range o.lp {
			acc += c * o.x[(m+aoptH-k)&(optRing-1)]
		}
		buf[i] = o.f[m&(optRing-1)] + o.x[m&(optRing-1)] - acc
	}
}

func (o *aoptimizer) fft(re, im []float64, inv bool) {
	n := len(re)
	for i, r := range o.rev {
		if i < r {
			re[i], re[r] = re[r], re[i]
			im[i], im[r] = im[r], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half, st := size/2, n/size
		for start := 0; start < n; start += size {
			for k := range half {
				c, s := o.cosT[k*st], o.sinT[k*st]
				if inv {
					s = -s
				}
				a, b := start+k, start+k+half
				tr := re[b]*c - im[b]*s
				ti := re[b]*s + im[b]*c
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
			}
		}
	}
}

// eval returns the cost of the free correction x of the window from w0
// and its gradient in g.
func (o *aoptimizer) eval(w0 int, x, g []float64) float64 {
	L, H := o.settle, aoptH
	base := L + H
	for i := -base; i < aoptWin+H; i++ {
		var v float64
		switch {
		case i < aoptCtx:
			v = o.x[(w0+i)&(optRing-1)]
		case i < aoptWin:
			v = x[i-aoptCtx]
		}
		o.xa[i+base] = v
	}
	// e[i+L] is the deviation with the high-passed correction at i, from -L.
	for i := -L; i < aoptWin; i++ {
		var acc float64
		for k, c := range o.lp {
			acc += c * o.xa[i+H-k+base]
		}
		o.e[i+L] = o.f[(w0+i)&(optRing-1)] + o.xa[i+base] - acc
	}
	n := len(o.re)
	re, im := o.re, o.im
	var ph float64
	k2 := o.k2
	for t := range n {
		k, p := t/o.up, t%o.up
		w := o.wts[p*L : p*L+L]
		var d float64
		for j, c := range w {
			d += c * o.e[k-j+L]
		}
		ph += k2 * d
		s, c := math.Sincos(ph)
		re[t], im[t] = o.taper[t]*c, o.taper[t]*s
		o.gout[t] = ph
	}
	o.fft(re, im, false)
	var j float64
	for k := range n {
		w := o.w2[k]
		j += w * (re[k]*re[k] + im[k]*im[k])
		re[k], im[k] = w*re[k], w*im[k]
	}
	o.fft(re, im, true)
	sc := 1 / (o.jscale * float64(n))
	var acc float64
	for t := n - 1; t >= 0; t-- {
		s, c := math.Sincos(o.gout[t])
		zr, zi := o.taper[t]*c, o.taper[t]*s
		acc += -2 * (zi*re[t] - zr*im[t]) * sc
		o.gout[t] = acc * k2
	}
	for i := range o.ge {
		o.ge[i] = 0
	}
	for k := -L + 1; k < aoptWin; k++ {
		var v float64
		for p := range o.up {
			w := o.wts[p*L : p*L+L]
			for jj, c := range w {
				t := o.up*(k+jj) + p
				if t >= n {
					break
				}
				if t >= 0 {
					v += o.gout[t] * c
				}
			}
		}
		o.ge[k+L] = v
	}
	for i := aoptCtx; i < aoptWin; i++ {
		v := o.ge[i+L]
		for m := max(i-H, -L+1); m <= min(i+H, aoptWin-1); m++ {
			v -= o.ge[m+L] * o.lp[m+H-i]
		}
		g[i-aoptCtx] = v
	}
	return j * sc
}

// block optimizes the window from sample w0.
func (o *aoptimizer) block(w0 int) {
	for n := range aoptFree {
		o.xv[n] = o.x[(w0+aoptCtx+n)&(optRing-1)]
	}
	o.jscale = 1
	j := o.eval(w0, o.xv, o.g)
	if !(j > 1e-14) {
		return
	}
	o.jscale = j
	j = o.eval(w0, o.xv, o.g)
	m := 0
	for it := 0; it < o.iters; it++ {
		copy(o.d, o.g)
		top := min(m, optMem)
		for i := 1; i <= top; i++ {
			k := (m - i) % optMem
			o.alpha[k] = o.rho[k] * dot(o.s[k], o.d)
			axpy(-o.alpha[k], o.y[k], o.d)
		}
		gamma := 0.0
		if m > 0 {
			k := (m - 1) % optMem
			gamma = dot(o.s[k], o.y[k]) / dot(o.y[k], o.y[k])
		} else {
			var mx float64
			for _, v := range o.g {
				mx = max(mx, math.Abs(v))
			}
			gamma = 1 / max(mx, 1e-300)
		}
		for i := range o.d {
			o.d[i] *= gamma
		}
		for i := top; i >= 1; i-- {
			k := (m - i) % optMem
			b := o.rho[k] * dot(o.y[k], o.d)
			axpy(o.alpha[k]-b, o.s[k], o.d)
		}
		gd := dot(o.g, o.d)
		if !(gd > 0) {
			break
		}
		step := 1.0
		var jn float64
		ok := false
		for range 30 {
			for i := range o.xn {
				o.xn[i] = o.xv[i] - step*o.d[i]
			}
			jn = o.eval(w0, o.xn, o.gn)
			if jn <= j-1e-4*step*gd {
				ok = true
				break
			}
			step /= 2
		}
		if !ok {
			break
		}
		k := m % optMem
		for i := range o.xv {
			o.s[k][i] = o.xn[i] - o.xv[i]
			o.y[k][i] = o.gn[i] - o.g[i]
		}
		sy := dot(o.s[k], o.y[k])
		copy(o.xv, o.xn)
		copy(o.g, o.gn)
		j = jn
		if sy > 1e-300 {
			o.rho[k] = 1 / sy
			m++
		}
	}
	for n := range aoptFree {
		o.x[(w0+aoptCtx+n)&(optRing-1)] = o.xv[n]
	}
}
