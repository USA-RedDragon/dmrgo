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

// floorTheta is the phase turn of a symbol of value 1, 0.27 pi.
const floorTheta = 0.27 * math.Pi

// floorPhases is the number of carrier phases of the genie trellis: a
// symbol a turns the phase by 27a of them, exactly.
const floorPhases = 200

// floorSignal is one noisy BS outbound stream and what a genie knows of it:
// the transmitted symbols, the timing and the carrier.
type floorSignal struct {
	// y holds the noisy samples with the carrier removed.
	y []complex128
	// zp holds the prefix sums of conj(y)*s, s the noiseless signal with
	// the carrier removed: zp[i] sums samples 0 to i-1.
	zp []complex128
	// zv holds conj(y)*s per sample.
	zv []complex128
	// sym holds the transmitted symbols, slot i symbol k at 144i+k.
	sym []int8
	// scale is the log-likelihood of Re(conj(y)*s) per unit: 1/sigma^2 of
	// each noise component.
	scale float64
	// base is the sample of the impulse of symbol 0.
	base int
	// hcum holds the phase of a symbol of value 1 at sample offset m-80
	// from its impulse, in units of floorTheta.
	hcum []float64
}

// floorOffsetHz is the carrier offset of the sweep stream.
const floorOffsetHz = 1400

// newFloorSignal builds the signal of noisyBaseband(slots, floorOffsetHz,
// cn, seed), sample for sample, with the noiseless reference beside it.
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

// center returns the sample of the impulse of stream symbol n.
func (f *floorSignal) center(n int) int { return f.base + samplesPerSy*n }

// h returns the phase, in units of floorTheta, of a symbol of value 1 at
// sample offset off from its impulse.
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

// floorChange is a change of stream symbol n by d.
type floorChange struct {
	n, d int
}

// delta returns the log-likelihood of the signal with the symbol changes
// ch, in increasing symbol order, minus that of the transmitted signal,
// exactly: the changes turn the phase of every later sample. With tail
// false, the samples after the last change window do not count, as for a
// receiver that does not know the carrier phase after the change.
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

// genieTrellis is a BCJR detector over a span of stream symbols that a
// genie knows except for the free digits of some symbols: its state is the
// window of 2d symbols and the carrier phase, one of floorPhases, so the
// phase is exact and only the frequency pulse of the free symbols is cut
// to the window. The known symbols outside the window enter with their
// whole pulse.
type genieTrellis struct {
	// logZ is the log of the sum, or the max when max-log, of the path
	// metrics of the last run, the tail included.
	logZ  float64
	m     *cpmModel
	d, ns int
	alpha []float32
	beta  []float32
	nb    []float32
	cos   [floorPhases]float32
	sin   [floorPhases]float32
	corr  []complex64
	ws    []int
	ws2   []int
	yc    []complex128
	// f, n0, mask, maxlog and prior are the arguments of the current run.
	f      *floorSignal
	n0     int
	mask   func(int) uint8
	maxlog bool
	prior  []float32
}

// newGenieTrellis returns a genie trellis with a window of d symbols on
// each side for spans of up to maxSteps symbols.
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

// floorBeam is how far under the best state, in nats, a state still
// extends paths.
const floorBeam = 50

// logAddSteps is the number of table steps per nat of floorLogAdd.
const logAddSteps = 32

// logAddTable holds log(1+e^-x) at the upper end of each step of x, so
// that floorLogAdd never exceeds the exact sum: the floor's output density then
// stays a sub-probability.
var logAddTable = func() [24 * logAddSteps]float32 {
	var t [24 * logAddSteps]float32
	for i := range t {
		t[i] = float32(math.Log1p(math.Exp(-float64(i+1) / logAddSteps)))
	}
	return t
}()

// floorLogAdd returns log(e^a + e^b), rounded down by at most 1/64 nat.
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

// digitOf returns the trellis digit of symbol value v.
func digitOf(v int8) int { return (int(v) + 3) / 2 }

// phaseBefore returns the carrier phase state that the symbols before
// stream symbol n turn.
func (f *floorSignal) phaseBefore(n int) int {
	var p int
	for _, v := range f.sym[:n] {
		p += 27 * int(v)
	}
	return ((p % floorPhases) + floorPhases) % floorPhases
}

// windowOf returns the trellis state of the digits of stream symbols from
// to to-1.
func (f *floorSignal) windowOf(from, to int) int {
	var w int
	for n := from; n < to; n++ {
		w = w<<2 | digitOf(f.sym[n])
	}
	return w
}

// wrapPhase returns phase state q in 0 to floorPhases-1.
func wrapPhase(q int) int {
	switch {
	case q < 0:
		return q + floorPhases
	case q >= floorPhases:
		return q - floorPhases
	}
	return q
}

// add relaxes dst with v: the max when max-log, the log sum otherwise.
func (g *genieTrellis) add(dst *float32, v float32) {
	if g.maxlog {
		*dst = max(*dst, v)
		return
	}
	*dst = floorLogAdd(*dst, v)
}

// windows returns the windows of 2d+1 digits around symbol interval m
// that the allowed digits permit.
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

// correct sets g.yc to the samples of intervals n0 to n1-1 with the phase
// of the known symbols outside each window removed, those before it
// counting as complete in the state's phase.
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

// correlate sets g.corr of the windows ws of interval m.
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

// forward extends the paths of step k and returns the best metric it
// wrote, given top, the best metric of step k.
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

// backward computes the backward metrics of step k from those of step
// k+1 in g.beta, and the log-APPs of the newest digit of the step.
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

// tailInit sets the backward metrics after the last step: only the true
// window of the symbols after the span, and, with tail, the samples after
// the span with the phase that each state ends at. It then sets g.logZ.
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

// run detects stream symbols n0+d to n1+d-1 over symbol intervals n0 to
// n1-1. mask(n) gives the allowed digits of symbol n as a 4-bit set; the
// symbols n0-d to n0+d-1 and n1-d to n1+d-1 must have only their true
// digit allowed. prior, when not nil, holds log priors of the digits of
// the detected symbols, as app. With tail, the samples after the span
// count with the carrier phase that each path ends at. The log-APPs of
// the digits of symbol n0+d+k go to app[4k:4k+4], max-log when maxlog,
// exact otherwise.
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

// burstChanges appends the symbol changes of burst c against the burst of
// slot i in f to ch.
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
