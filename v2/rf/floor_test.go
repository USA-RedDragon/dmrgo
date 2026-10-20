package rf

import (
	"math"
	"math/bits"
	"math/cmplx"
	"math/rand/v2"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const floorTheta = 0.27 * math.Pi

const floorPhases = 200

type floorSignal struct {
	y     []complex128
	zp    []complex128
	zv    []complex128
	sym   []int8
	scale float64
	base  int
	hcum  []float64
}

const floorOffsetHz = 1400

func newFloorSignal(slots []rftx.BSSlot, cn float64, seed uint64) *floorSignal {
	fm, env := rftx.ModulateBS(slots)
	pad := int(benchPad * basebandRate)
	n := len(fm) + 2*pad
	f := &floorSignal{y: make([]complex128, n), zp: make([]complex128, n+1), zv: make([]complex128, n), base: pad + 60}
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	f.scale = 1 / (sigma * sigma)
	var ph, cph float64
	for i := range n {
		var re, im float64
		var s complex128
		k := i - pad
		if k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + floorOffsetHz) / basebandRate
			cph += 2 * math.Pi * floorOffsetHz / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
			s = complex(re, im)
		}
		x := complex(float64(float32(re+rng.NormFloat64()*sigma)), float64(float32(im+rng.NormFloat64()*sigma)))
		rot := complex(math.Cos(cph), -math.Sin(cph))
		f.y[i] = x * rot
		f.zv[i] = cmplx.Conj(x) * s
		f.zp[i+1] = f.zp[i] + f.zv[i]
	}
	f.sym = make([]int8, 0, len(slots)*slotSymbols)
	for _, s := range slots {
		for _, v := range rftx.SymbolsFromBits(unpacked.Unpack(s.CACH[:])) {
			f.sym = append(f.sym, int8(v))
		}
		for _, v := range rftx.SymbolsFromBurst(s.Burst) {
			f.sym = append(f.sym, int8(v))
		}
	}
	taps := rftx.RRCTaps()
	f.hcum = make([]float64, len(taps))
	var c float64
	for i, v := range taps {
		c += v
		f.hcum[i] = c
	}
	return f
}

func (f *floorSignal) center(n int) int { return f.base + samplesPerSy*n }

func (f *floorSignal) h(off int) float64 {
	half := len(f.hcum) / 2
	switch i := off + half; {
	case i < 0:
		return 0
	case i >= len(f.hcum):
		return 1
	default:
		return f.hcum[i]
	}
}

type floorChange struct {
	n, d int
}

func (f *floorSignal) delta(ch []floorChange, tail bool) float64 {
	if len(ch) == 0 {
		return 0
	}
	half := len(f.hcum) / 2
	var sum float64
	var phase float64
	i := 0
	for i < len(ch) {
		a := f.center(ch[i].n) - half
		j := i
		b := f.center(ch[j].n) + half
		for j+1 < len(ch) && f.center(ch[j+1].n)-half <= b+1 {
			j++
			b = f.center(ch[j].n) + half
		}
		a, b = max(a, 0), min(b, len(f.zv)-1)
		for s := a; s <= b; s++ {
			var p float64
			for _, c := range ch[i : j+1] {
				p += float64(c.d) * f.h(s-f.center(c.n))
			}
			p = phase + floorTheta*p
			sum += real(f.zv[s] * (cmplx.Exp(complex(0, p)) - 1))
		}
		for _, c := range ch[i : j+1] {
			phase += floorTheta * float64(c.d)
		}
		end := len(f.zv)
		if j+1 < len(ch) {
			end = max(f.center(ch[j+1].n)-half, b+1)
		} else if !tail {
			end = b + 1
		}
		if end > b+1 {
			sum += real((f.zp[end] - f.zp[b+1]) * (cmplx.Exp(complex(0, phase)) - 1))
		}
		i = j + 1
	}
	return sum * f.scale
}

type genieTrellis struct {
	logZ   float64
	m      *cpmModel
	d, ns  int
	alpha  []float32
	beta   []float32
	nb     []float32
	cos    [floorPhases]float32
	sin    [floorPhases]float32
	corr   []complex64
	ws     []int
	ws2    []int
	yc     []complex128
	f      *floorSignal
	n0     int
	mask   func(int) uint8
	maxlog bool
	prior  []float32
}

func newGenieTrellis(d, maxSteps int) *genieTrellis {
	m := newCPMModel(d, samplesPerSy)
	g := &genieTrellis{m: m, d: d, ns: m.states}
	n := m.states * floorPhases
	g.alpha = make([]float32, (maxSteps+1)*n)
	g.beta = make([]float32, n)
	g.nb = make([]float32, n)
	g.corr = make([]complex64, m.windows)
	for p := range floorPhases {
		g.cos[p] = float32(math.Cos(2 * math.Pi * float64(p) / floorPhases))
		g.sin[p] = float32(math.Sin(2 * math.Pi * float64(p) / floorPhases))
	}
	return g
}

const floorBeam = 50

const logAddSteps = 32

var logAddTable = func() [24 * logAddSteps]float32 {
	var t [24 * logAddSteps]float32
	for i := range t {
		t[i] = float32(math.Log1p(math.Exp(-float64(i+1) / logAddSteps)))
	}
	return t
}()

func floorLogAdd(a, b float32) float32 {
	if a < b {
		a, b = b, a
	}
	x := (a - b) * logAddSteps
	if !(x < float32(len(logAddTable))) {
		return a
	}
	return a + logAddTable[int(x)]
}

func digitOf(v int8) int { return (int(v) + 3) / 2 }

func (f *floorSignal) phaseBefore(n int) int {
	var p int
	for _, v := range f.sym[:n] {
		p += 27 * int(v)
	}
	return ((p % floorPhases) + floorPhases) % floorPhases
}

func (f *floorSignal) windowOf(from, to int) int {
	var w int
	for n := from; n < to; n++ {
		w = w<<2 | digitOf(f.sym[n])
	}
	return w
}

func wrapPhase(q int) int {
	switch {
	case q < 0:
		return q + floorPhases
	case q >= floorPhases:
		return q - floorPhases
	}
	return q
}

func (g *genieTrellis) add(dst *float32, v float32) {
	if g.maxlog {
		*dst = max(*dst, v)
		return
	}
	*dst = floorLogAdd(*dst, v)
}

func (g *genieTrellis) windows(m int) []int {
	g.ws = append(g.ws[:0], 0)
	for n := m - g.d; n <= m+g.d; n++ {
		mk := g.mask(n)
		g.ws2 = g.ws2[:0]
		for _, w := range g.ws {
			for v := range 4 {
				if mk>>v&1 != 0 {
					g.ws2 = append(g.ws2, w<<2|v)
				}
			}
		}
		g.ws, g.ws2 = g.ws2, g.ws
	}
	return g.ws
}

func (g *genieTrellis) correct(n1 int) {
	f, d, sps := g.f, g.d, samplesPerSy
	span := rrcSpan + d + 1
	g.yc = g.yc[:0]
	for m := g.n0; m < n1; m++ {
		c0 := f.center(m) - sps/2
		for j := range sps {
			s := c0 + j
			var ph float64
			for n := max(m-span, 0); n <= min(m+span, len(f.sym)-1); n++ {
				if n >= m-d && n <= m+d || bits.OnesCount8(g.mask(n)) != 1 {
					continue
				}
				hv := f.h(s - f.center(n))
				if n < m-d {
					hv--
				}
				ph += float64(f.sym[n]) * hv
			}
			g.yc = append(g.yc, f.y[s]*cmplx.Exp(complex(0, -floorTheta*ph)))
		}
	}
}

func (g *genieTrellis) correlate(m int, ws []int) {
	sps := samplesPerSy
	yc := g.yc[(m-g.n0)*sps : (m-g.n0+1)*sps]
	for _, w := range ws {
		tp := g.m.tpl[w*sps : (w+1)*sps]
		var c complex128
		for j := range sps {
			c += yc[j] * complex128(tp[j])
		}
		g.corr[w] = complex64(c * complex(g.f.scale, 0))
	}
}

func (g *genieTrellis) forward(k int, top float32) float32 {
	S := g.ns * floorPhases
	ws := g.windows(g.n0 + k)
	g.correlate(g.n0+k, ws)
	cur := g.alpha[k*S : (k+1)*S]
	next := g.alpha[(k+1)*S : (k+2)*S]
	for i := range next {
		next[i] = negInf
	}
	thr := top - floorBeam
	ntop := negInf
	for _, wf := range ws {
		re, im := real(g.corr[wf]), imag(g.corr[wf])
		turn := 27 * (2*(wf>>(4*g.d)) - 3)
		src := cur[(wf>>2)*floorPhases : (wf>>2+1)*floorPhases]
		s2 := wf & (g.ns - 1)
		dst := next[s2*floorPhases : (s2+1)*floorPhases]
		var bonus float32
		if g.prior != nil {
			bonus = g.prior[4*k+wf&3]
		}
		for p, a := range src {
			if a < thr {
				continue
			}
			v := a + re*g.cos[p] + im*g.sin[p] + bonus
			g.add(&dst[wrapPhase(p+turn)], v)
			ntop = max(ntop, v)
		}
	}
	return ntop
}

func (g *genieTrellis) backward(k int, app []float32) {
	S := g.ns * floorPhases
	ws := g.windows(g.n0 + k)
	g.correlate(g.n0+k, ws)
	cur := g.alpha[k*S : (k+1)*S]
	nb := g.nb
	for i := range nb {
		nb[i] = negInf
	}
	a := app[4*k : 4*k+4]
	for v := range a {
		a[v] = negInf
	}
	atop := negInf
	for _, v := range cur {
		atop = max(atop, v)
	}
	thr := atop - floorBeam
	for _, wf := range ws {
		re, im := real(g.corr[wf]), imag(g.corr[wf])
		turn := 27 * (2*(wf>>(4*g.d)) - 3)
		src := cur[(wf>>2)*floorPhases : (wf>>2+1)*floorPhases]
		s2 := wf & (g.ns - 1)
		bs := g.beta[s2*floorPhases : (s2+1)*floorPhases]
		dst := nb[(wf>>2)*floorPhases : (wf>>2+1)*floorPhases]
		v := wf & 3
		var bonus float32
		if g.prior != nil {
			bonus = g.prior[4*k+v]
		}
		for p, al := range src {
			if al < thr {
				continue
			}
			b := bs[wrapPhase(p+turn)]
			if b <= negInf/2 {
				continue
			}
			x := re*g.cos[p] + im*g.sin[p] + b + bonus
			g.add(&dst[p], x)
			g.add(&a[v], al+x)
		}
	}
	g.beta, g.nb = nb, g.beta
}

func (g *genieTrellis) tailInit(n1 int, tail bool) {
	f, d := g.f, g.d
	S := g.ns * floorPhases
	wEnd := f.windowOf(n1-d, n1+d)
	pEnd := f.phaseBefore(n1 - d)
	for i := range g.beta {
		g.beta[i] = negInf
	}
	var zt complex128
	if tail {
		zt = (f.zp[len(f.zv)] - f.zp[f.center(n1)-samplesPerSy/2]) * complex(f.scale, 0)
	}
	for p := range floorPhases {
		dp := float64(p-pEnd) * 2 * math.Pi / floorPhases
		g.beta[wEnd*floorPhases+p] = float32(real(zt * (cmplx.Exp(complex(0, dp)) - 1)))
	}
	steps := n1 - g.n0
	z := math.Inf(-1)
	for i, a := range g.alpha[steps*S : (steps+1)*S] {
		if a <= negInf/2 || g.beta[i] <= negInf/2 {
			continue
		}
		v := float64(a) + float64(g.beta[i])
		switch {
		case g.maxlog:
			z = math.Max(z, v)
		case v > z:
			z = v + math.Log1p(math.Exp(z-v))
		default:
			z += math.Log1p(math.Exp(v - z))
		}
	}
	g.logZ = z
}

func (g *genieTrellis) run(f *floorSignal, n0, n1 int, mask func(int) uint8, maxlog, tail bool, prior, app []float32) {
	g.f, g.n0, g.mask, g.maxlog, g.prior = f, n0, mask, maxlog, prior
	S := g.ns * floorPhases
	alpha := g.alpha[:S]
	for i := range alpha {
		alpha[i] = negInf
	}
	alpha[f.windowOf(n0-g.d, n0+g.d)*floorPhases+f.phaseBefore(n0-g.d)] = 0
	g.correct(n1)
	top := float32(0)
	for k := range n1 - n0 {
		top = g.forward(k, top)
	}
	g.tailInit(n1, tail)
	for k := n1 - n0 - 1; k >= 0; k-- {
		g.backward(k, app)
	}
}

func (f *floorSignal) burstChanges(ch []floorChange, i int, c *layer2.RawBurst) []floorChange {
	syms := rftx.SymbolsFromBurst(*c)
	for k, v := range syms {
		n := i*slotSymbols + cachSymbols + k
		if dv := v - int(f.sym[n]); dv != 0 {
			ch = append(ch, floorChange{n: n, d: dv})
		}
	}
	return ch
}
