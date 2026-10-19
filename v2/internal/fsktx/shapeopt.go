package fsktx

import "math"

// Sizes of the out-of-band phase optimizer of IQConfig.Shape, in shaper
// samples at iqShaperHz: each block optimizes a window of optWin samples,
// optCtx of them already final, and makes optStep of them final.
const (
	optWin  = 1024
	optCtx  = 256
	optStep = 512
	optRing = 4096
	optLPN  = 161
	optMem  = 8
	optFree = optWin - optCtx
	// optDelay is the delay of the optimizer: the lookahead of a block,
	// a whole block, and the half-length of the high pass.
	optDelay = (optWin - optCtx - 1 + optLPN/2 + 9) / 10 * 10
)

// Tuning of the optimizer.
const (
	optInHz    = 3000.0
	optFromHz  = 5000.0
	optPenalty = 0.0
	optLPHz    = 3750.0
	optLPBeta  = 8.0
	// optNoChain makes each block start from no correction and see none in
	// its context, so that the output depends on a finite span of symbols.
	optNoChain = false
)

// optimizer adds a phase correction to the shaped deviation that lowers
// the power outside the channel, weighted by Emission Mask D, with the
// frequency below optInHz unchanged: a high pass removes everything below
// it from the correction, so the receive filter and the deviation tests
// see the same signal. It delays the deviation by optDelay shaper samples,
// and not the keying: the caller keys by the lag, as for the pulse.
type optimizer struct {
	iters      int
	f, psi     []float64
	lp         []float64
	w2, inband []float64
	taper      []float64
	cosT, sinT []float64
	rev        []int
	phi0       []float64
	re, im     []float64
	pr, pi     []float64
	x, g, d    []float64
	xn, gn     []float64
	s, y       [optMem][]float64
	rho        [optMem]float64
	alpha      [optMem]float64
	nIn, nBlk  int
	jscale     float64
	prevPsiH   float64
}

func newOptimizer(iters int) *optimizer {
	o := &optimizer{iters: iters}
	o.f = make([]float64, optRing)
	o.psi = make([]float64, optRing)
	o.lp = make([]float64, optLPN)
	var sum float64
	h := optLPN / 2
	fc := optLPHz / iqShaperHz
	for i := range o.lp {
		t := float64(i - h)
		v := 2 * fc
		if t != 0 {
			v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
		}
		x := t / float64(h+1)
		v *= besselI0(optLPBeta*math.Sqrt(max(0, 1-x*x))) / besselI0(optLPBeta)
		o.lp[i] = v
		sum += v
	}
	for i := range o.lp {
		o.lp[i] /= sum
	}
	o.w2 = make([]float64, optWin)
	o.inband = make([]float64, optWin)
	o.taper = make([]float64, optWin)
	for k := range optWin {
		fk := float64(k) * iqShaperHz / optWin
		if k >= optWin/2 {
			fk -= iqShaperHz
		}
		a := math.Abs(fk)
		switch {
		case a < optInHz:
			o.inband[k] = 1
		case a > optFromHz:
			o.w2[k] = math.Pow(10, optMaskAttn(a)/10)
		}
		o.taper[k] = 0.5 - 0.5*math.Cos(2*math.Pi*(float64(k)+0.5)/optWin)
	}
	o.cosT = make([]float64, optWin/2)
	o.sinT = make([]float64, optWin/2)
	for k := range o.cosT {
		o.sinT[k], o.cosT[k] = math.Sincos(-2 * math.Pi * float64(k) / optWin)
	}
	o.rev = make([]int, optWin)
	bits := 0
	for 1<<bits < optWin {
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
	o.phi0 = make([]float64, optWin)
	o.re, o.im = make([]float64, optWin), make([]float64, optWin)
	o.pr, o.pi = make([]float64, optWin), make([]float64, optWin)
	o.x, o.g, o.d = make([]float64, optFree), make([]float64, optFree), make([]float64, optFree)
	o.xn, o.gn = make([]float64, optFree), make([]float64, optFree)
	for i := range optMem {
		o.s[i], o.y[i] = make([]float64, optFree), make([]float64, optFree)
	}
	return o
}

// optMaskAttn is the attenuation of Emission Mask D at fd Hz from the
// carrier.
func optMaskAttn(fd float64) float64 {
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

// process takes the shaped deviation in buf and replaces it with the
// delayed corrected deviation.
func (o *optimizer) process(buf []float64) {
	for i, v := range buf {
		o.f[o.nIn&(optRing-1)] = v
		o.psi[o.nIn&(optRing-1)] = 0
		o.nIn++
		if o.nIn == o.nBlk*optStep+optWin-optCtx {
			o.block(o.nBlk*optStep - optCtx)
			o.nBlk++
		}
		m := o.nIn - 1 - optDelay
		if m < 0 {
			buf[i] = 0
			continue
		}
		ph := o.psiH(m)
		buf[i] = o.f[m&(optRing-1)] + (ph-o.prevPsiH)*iqShaperHz/(2*math.Pi)
		o.prevPsiH = ph
	}
}

// psiH is the high-passed correction at sample m.
func (o *optimizer) psiH(m int) float64 {
	h := optLPN / 2
	var acc float64
	for k, c := range o.lp {
		acc += c * o.psi[(m+h-k)&(optRing-1)]
	}
	return o.psi[m&(optRing-1)] - acc
}

func (o *optimizer) fft(re, im []float64, inv bool) {
	for i, r := range o.rev {
		if i < r {
			re[i], re[r] = re[r], re[i]
			im[i], im[r] = im[r], im[i]
		}
	}
	for size := 2; size <= optWin; size <<= 1 {
		half, st := size/2, optWin/size
		for start := 0; start < optWin; start += size {
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
	if inv {
		for i := range re {
			re[i] /= optWin
			im[i] /= optWin
		}
	}
}

// eval returns the cost of the free correction x and its gradient in g.
func (o *optimizer) eval(w0 int, x, g []float64) float64 {
	re, im, pr, pi := o.re, o.im, o.pr, o.pi
	for n := range optWin {
		p := o.psi[(w0+n)&(optRing-1)]
		if optNoChain {
			p = 0
		}
		if n >= optCtx {
			p = x[n-optCtx]
		}
		s, c := math.Sincos(o.phi0[n] + p)
		re[n], im[n] = o.taper[n]*c, o.taper[n]*s
		pr[n], pi[n] = o.taper[n]*p, 0
	}
	o.fft(re, im, false)
	if optPenalty > 0 {
		o.fft(pr, pi, false)
	}
	var j, jp float64
	for k := range optWin {
		w := o.w2[k]
		j += w * (re[k]*re[k] + im[k]*im[k])
		re[k], im[k] = w*re[k], w*im[k]
		l := o.inband[k]
		jp += l * (pr[k]*pr[k] + pi[k]*pi[k])
		pr[k], pi[k] = l*pr[k], l*pi[k]
	}
	o.fft(re, im, true)
	if optPenalty > 0 {
		o.fft(pr, pi, true)
	}
	sc := 1 / (o.jscale * optWin)
	for n := optCtx; n < optWin; n++ {
		p := x[n-optCtx]
		s, c := math.Sincos(o.phi0[n] + p)
		sr, si := o.taper[n]*c, o.taper[n]*s
		gn := -2 * (re[n]*si - im[n]*sr)
		g[n-optCtx] = gn*optWin*sc + optPenalty*2*o.taper[n]*pr[n]
	}
	return j*sc + optPenalty*jp/optWin
}

// block optimizes the window from shaper sample w0.
func (o *optimizer) block(w0 int) {
	var ph float64
	for n := range optWin {
		ph += o.f[(w0+n)&(optRing-1)] * 2 * math.Pi / iqShaperHz
		o.phi0[n] = ph
	}
	for n := range optFree {
		o.x[n] = o.psi[(w0+optCtx+n)&(optRing-1)]
		if optNoChain {
			o.x[n] = 0
		}
	}
	o.jscale = 1
	j := o.eval(w0, o.x, o.g)
	if !(j > 1e-14) {
		return
	}
	o.jscale = j
	j = o.eval(w0, o.x, o.g)
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
			gamma = 1e-3 / max(mx, 1e-300)
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
				o.xn[i] = o.x[i] - step*o.d[i]
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
		for i := range o.x {
			o.s[k][i] = o.xn[i] - o.x[i]
			o.y[k][i] = o.gn[i] - o.g[i]
		}
		sy := dot(o.s[k], o.y[k])
		copy(o.x, o.xn)
		copy(o.g, o.gn)
		j = jn
		if sy > 1e-300 {
			o.rho[k] = 1 / sy
			m++
		}
	}
	for n := range optFree {
		o.psi[(w0+optCtx+n)&(optRing-1)] = o.x[n]
	}
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func axpy(a float64, x, y []float64) {
	for i := range y {
		y[i] += a * x[i]
	}
}
