package fsktx

import "math"

const (
	radioHz        = 96000
	radioLowpassHz = 8000.0
)

const (
	aoptWin48k = 1024
	aoptH      = optLPN / 2
)

const aoptAnchor = 2

const (
	aoptFarHz = 17000.0
	aoptFarDb = 20.0
)

// Weights of the audio optimizer on the receivers of EN 300 113 Annex C
const (
	aoptACP = 0.0
	aoptALT = 0.0
)

const (
	aoptLowHz  = 32000
	aoptACPLow = 0.0
)

const (
	aoptAnyRate = false
)

type aoptimizer struct {
	iters     int
	up        int
	settle    int
	delay     int
	win, ctx  int
	step      int
	anchor    int
	pinLo     int
	pinHi     int
	indep     bool
	p         []float64
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

func aoptSettle(hz int) int {
	al := 2 * math.Pi * radioLowpassHz / math.Sqrt2
	return int(math.Ceil((36/al+1/float64(hz))*float64(hz))) + 1
}

func newAudioOpt(iters, hz int) *aoptimizer {
	win := 64
	for float64(win)*math.Sqrt2 < aoptWin48k*float64(hz)/48000 || win < 4*(aoptSettle(hz)+aoptH) {
		win *= 2
	}
	return newAudioOptWin(iters, hz, win, aoptAnchor)
}

func newAudioOptWin(iters, hz, win, anchor int) *aoptimizer {
	up := 1
	for hz*up < radioHz {
		up *= 2
	}
	rhz := float64(hz * up)
	w := 2 * math.Pi * radioLowpassHz
	al := w / math.Sqrt2
	hold := 1 / float64(hz)
	n := win * up
	o := &aoptimizer{iters: iters, up: up, k2: 2 * math.Pi / radioHz, win: win, ctx: win / 4, step: win / 2, settle: aoptSettle(hz), anchor: anchor}
	free := win - o.ctx
	if up*hz != radioHz {
		o.k2 = 2 * math.Pi / rhz
	}
	ramp := func(t float64) float64 {
		if t <= 0 {
			return 0
		}
		return t - (1-math.Exp(-al*t)*math.Cos(al*t))/al
	}
	rise := func(t float64) float64 { return ramp(t) - ramp(t-hold) }
	o.wts = make([]float64, up*o.settle)
	for p := range up {
		end := float64(p+1) / rhz
		for j := range o.settle {
			t := end + float64(j)*hold
			o.wts[p*o.settle+j] = (rise(t) - rise(t-1/rhz)) * rhz
		}
	}
	sym := hz / gcd(hz, 4800)
	o.delay = (win - o.ctx - 1 + aoptH + sym - 1) / sym * sym
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
	acp := aoptACP
	if hz < aoptLowHz && aoptACPLow > 0 {
		acp = aoptACPLow
	}
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
			if acp > 0 {
				o.w2[k] += math.Pow(10, (acp-optAnnexC(a, 12500))/10)
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
	o.xa = make([]float64, win+2*aoptH+o.settle)
	o.e = make([]float64, win+o.settle)
	o.ge = make([]float64, win+o.settle)
	o.xv, o.g, o.d = make([]float64, free), make([]float64, free), make([]float64, free)
	o.xn, o.gn, o.p = make([]float64, free), make([]float64, free), make([]float64, free)
	for i := range optMem {
		o.s[i], o.y[i] = make([]float64, free), make([]float64, free)
	}
	return o
}

func (o *aoptimizer) process(buf []float64) {
	for i, v := range buf {
		o.f[o.nIn&(optRing-1)] = v
		o.x[o.nIn&(optRing-1)] = 0
		o.nIn++
		if o.nIn == o.nBlk*o.step+o.win-o.ctx {
			o.block(o.nBlk*o.step - o.ctx)
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

func (o *aoptimizer) eval(w0 int, x, g []float64) float64 {
	L, H := o.settle, aoptH
	win, ctx := o.win, o.ctx
	base := L + H
	for i := -base; i < win+H; i++ {
		var v float64
		switch {
		case i < ctx:
			if !o.indep {
				v = o.x[(w0+i)&(optRing-1)]
			}
		case i < win:
			v = x[i-ctx]
		}
		o.xa[i+base] = v
	}
	for i := -L; i < win; i++ {
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
	for k := -L + 1; k < win; k++ {
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
	for i := ctx; i < win; i++ {
		v := o.ge[i+L]
		for m := max(i-H, -L+1); m <= min(i+H, win-1); m++ {
			v -= o.ge[m+L] * o.lp[m+H-i]
		}
		g[i-ctx] = v
	}
	for i := o.pinLo; i < o.pinHi; i++ {
		g[i] = 0
	}
	return j * sc
}

func (o *aoptimizer) block(w0 int) {
	free := o.win - o.ctx
	if o.anchor > 0 && (o.nBlk+1)%o.anchor == 0 {
		o.indep = true
		clear(o.xv)
		o.solve(w0)
		o.indep = false
		copy(o.p, o.xv)
		for n := range free {
			o.xv[n] = o.x[(w0+o.ctx+n)&(optRing-1)]
		}
		o.pinLo, o.pinHi = o.step-o.settle-aoptH-o.ctx, o.step
		copy(o.xv[o.pinLo:o.pinHi], o.p[o.pinLo:o.pinHi])
		o.solve(w0)
		o.pinLo, o.pinHi = 0, 0
	} else {
		for n := range free {
			o.xv[n] = o.x[(w0+o.ctx+n)&(optRing-1)]
		}
		if o.anchor > 0 && o.nBlk%o.anchor == 0 {
			copy(o.xv[:o.step-o.ctx], o.p[o.step:])
		}
		o.solve(w0)
	}
	for n := range free {
		o.x[(w0+o.ctx+n)&(optRing-1)] = o.xv[n]
	}
}

func (o *aoptimizer) solve(w0 int) {
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
		var gamma float64
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
}
