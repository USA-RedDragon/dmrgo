package main

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// cpmModel holds the DMR 4FSK waveform over one symbol interval for every
// window of 2d+1 symbols around the symbol, for coherent detection. Symbols
// before the window add their whole phase step, h*pi*a, to a phase that the
// detector carries with each survivor.
type cpmModel struct {
	d, sps  int
	states  int
	windows int
	// tpl holds the conjugate of the template of window w at offset j in
	// tpl[w*sps+j]. The newest symbol is the least significant base-4 digit.
	tpl  []complex64
	step [4]complex64
	q    []float64
	h    float64
}

// newCPMModel returns the model with a window of d symbols on each side, at
// sps samples per symbol and modulation index h (0.27 nominal).
func newCPMModel(d, sps int, h float64) *cpmModel {
	m := &cpmModel{d: d, sps: sps, states: 1 << (4 * d), windows: 1 << (4*d + 2)}
	taps := rrcTapsAt(float64(sps))
	m.q = make([]float64, len(taps)+1)
	for i, v := range taps {
		m.q[i+1] = m.q[i] + v
	}
	m.tpl = make([]complex64, m.windows*sps)
	m.setH(h)
	return m
}

// setH recomputes the templates for modulation index h.
func (m *cpmModel) setH(h float64) {
	sps, d := m.sps, m.d
	n := len(m.q) - 1
	half := n / 2
	pulse := func(j int) float64 {
		i := j + half
		switch {
		case i < 0:
			return 0
		case i >= n:
			return h * math.Pi
		}
		return h * math.Pi * m.q[i+1]
	}
	for w := range m.windows {
		for j := range sps {
			off := j - sps/2
			var ph float64
			for i := -d; i <= d; i++ {
				digit := (w >> (2 * (d - i))) & 3
				ph += float64(2*digit-3) * pulse(off-i*sps)
			}
			m.tpl[w*sps+j] = complex64(complex(math.Cos(ph), -math.Sin(ph)))
		}
	}
	for v := range 4 {
		ph := h * math.Pi * float64(2*v-3)
		m.step[v] = complex64(complex(math.Cos(ph), -math.Sin(ph)))
	}
	m.h = h
}

// rrcTapsAt returns rrcTaps for sps samples per symbol.
func rrcTapsAt(sps float64) []float64 {
	return rrcTapsSpan(sps, rrcSpan)
}

// rrcTapsSpan returns rrcTaps for sps samples per symbol, span symbols on
// each side.
func rrcTapsSpan(sps float64, span int) []float64 {
	n := int(2*float64(span)*sps) + 1
	h := make([]float64, n)
	var c float64
	for i := range h {
		t := float64(i-n/2) / sps
		switch {
		case t == 0:
			h[i] = 1 - rrcAlpha + 4*rrcAlpha/math.Pi
		case math.Abs(math.Abs(4*rrcAlpha*t)-1) < 1e-9:
			h[i] = rrcAlpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*rrcAlpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*rrcAlpha)))
		default:
			h[i] = (math.Sin(math.Pi*t*(1-rrcAlpha)) + 4*rrcAlpha*t*math.Cos(math.Pi*t*(1+rrcAlpha))) /
				(math.Pi * t * (1 - (4*rrcAlpha*t)*(4*rrcAlpha*t)))
		}
		c += h[i]
	}
	for i := range h {
		h[i] /= c
	}
	return h
}

// trellisSource feeds a trellis run and takes its output, interval by
// interval.
type trellisSource interface {
	// runSamples writes the samples of intervals k0 to k1-1 into x, sps per
	// interval.
	runSamples(k0, k1 int, x []complex64)
	// runPrior writes the log priors of the digits of the newest symbol of the
	// window of interval k into lp.
	runPrior(k int, lp []float32)
	// runKeep takes the log-APPs of the newest symbol of the window of
	// interval k, four digits.
	runKeep(k int, app []float32)
}

// trellis is a max-log BCJR detector over the windows of a cpmModel, with
// the phase of the symbols before the window carried per survivor (per
// survivor processing). It runs in blocks of win symbol intervals: the
// forward pass of a block goes look intervals past it, and the backward
// pass starts there, so its buffers hold win+look intervals, and the
// forward pass of the next block repeats the look intervals from the state
// at the end of the block. With win at least the run, it is the whole-run
// BCJR.
type trellis struct {
	m         *cpmModel
	maxSteps  int
	win, look int
	x         []complex64
	lp        []float32
	alpha     []float32
	beta      []float32
	next      []float32
	rot       []complex64
	nrot      []complex64
	bm        []float32
	corr      []complex64
	// saveA and saveR hold the forward state at the end of a block, and
	// last the forward metrics after the last interval of a run.
	saveA []float32
	saveR []complex64
	last  []float32
	// app holds the log-APPs of one interval for runKeep.
	app [4]float32
	// slp holds the log priors of the digits of the first window state,
	// oldest first. Zero means no prior.
	slp []float32
}

// newTrellis returns a trellis over model m for runs of up to maxSteps
// symbol intervals in blocks of win, with look intervals of lookahead.
func newTrellis(m *cpmModel, maxSteps, win, look int) *trellis {
	win = min(win, maxSteps)
	look = min(look, maxSteps-win)
	n := win + look
	return &trellis{
		m: m, maxSteps: maxSteps, win: win, look: look, x: make([]complex64, n*m.sps), lp: make([]float32, 4*n),
		alpha: make([]float32, (n+1)*m.states), beta: make([]float32, m.states), next: make([]float32, m.states),
		rot: make([]complex64, m.states), nrot: make([]complex64, m.states),
		bm: make([]float32, n*m.windows), corr: make([]complex64, m.windows),
		saveA: make([]float32, m.states), saveR: make([]complex64, m.states), last: make([]float32, m.states),
		slp: make([]float32, 8*m.d),
	}
}

const negInf = float32(-1e30)

// run detects the symbols of steps intervals of src, whose interval k holds
// the samples of symbol k+d centered on it, from the carried phase rot0
// (e^-j*phase) before symbol 0, with the priors of src and slp. scale is
// 2A/sigma^2 and mu the gain of the per-survivor phase tracking. The
// log-APPs of symbols 2d to steps+2d-1 go to src.runKeep.
func (t *trellis) run(src trellisSource, steps int, rot0 complex64, scale, mu float32) {
	m := t.m
	ns := m.states
	alpha := t.alpha[:ns]
	for s := range ns {
		var a float32
		for i := range 2 * m.d {
			a += t.slp[4*i+(s>>(2*(2*m.d-1-i)))&3]
		}
		alpha[s] = a
		t.rot[s] = rot0
	}
	for b0 := 0; b0 < steps; b0 += t.win {
		b1 := min(b0+t.win, steps)
		e := min(b1+t.look, steps)
		src.runSamples(b0, e, t.x)
		for k := b0; k < e; k++ {
			if k == b1 {
				copy(t.saveA, t.alpha[(k-b0)*ns:(k-b0+1)*ns])
				copy(t.saveR, t.rot)
			}
			lp := t.lp[4*(k-b0) : 4*(k-b0)+4]
			src.runPrior(k, lp)
			t.forward(k-b0, lp, scale, mu)
		}
		if e == steps {
			copy(t.last, t.alpha[(e-b0)*ns:(e-b0+1)*ns])
		}
		t.backward(src, b0, b1, e)
		if b1 < e {
			copy(t.alpha[:ns], t.saveA)
			copy(t.rot, t.saveR)
		}
	}
}

// forward runs the forward pass over row j of the block buffers with the
// log priors lp.
func (t *trellis) forward(j int, lp []float32, scale, mu float32) {
	m := t.m
	sps, ns := m.sps, m.states
	shift := 4 * m.d
	seg := t.x[j*sps : (j+1)*sps]
	for w := range m.windows / 2 {
		tp := m.tpl[w*sps : (w+1)*sps]
		var ac, bd, ad, bc float32
		for i, v := range seg {
			ac += real(v) * real(tp[i])
			bd += imag(v) * imag(tp[i])
			ad += real(v) * imag(tp[i])
			bc += imag(v) * real(tp[i])
		}
		t.corr[w] = complex(ac-bd, ad+bc)
		t.corr[m.windows-1-w] = complex(ac+bd, bc-ad)
	}
	alpha := t.alpha[j*ns : (j+1)*ns]
	next := t.alpha[(j+1)*ns : (j+2)*ns]
	bm := t.bm[j*m.windows : (j+1)*m.windows]
	for s2 := range ns {
		bonus := lp[s2&3]
		best, bd := negInf, 0
		for d0 := range 4 {
			w := d0<<shift | s2
			ro, c := t.rot[w>>2], t.corr[w]
			v := scale*(real(ro)*real(c)-imag(ro)*imag(c)) + bonus
			bm[w] = v
			if a := alpha[w>>2] + v; a > best {
				best, bd = a, d0
			}
		}
		next[s2] = best
		w := bd<<shift | s2
		r := t.rot[w>>2] * m.step[bd]
		if mu != 0 && best > negInf {
			c := t.rot[w>>2] * t.corr[w]
			if a := sqrtf(real(c)*real(c) + imag(c)*imag(c)); a > 0 {
				e := mu * imag(c) / a
				r *= complex(1, -e)
			}
		}
		t.nrot[s2] = r
	}
	t.rot, t.nrot = t.nrot, t.rot
}

// backward runs the backward pass of the block from interval b0, whose
// forward pass reaches e, from equal metrics after e, and keeps the
// log-APPs of the intervals before b1.
func (t *trellis) backward(src trellisSource, b0, b1, e int) {
	m := t.m
	ns := m.states
	mask := ns - 1
	beta := t.beta
	for s := range ns {
		beta[s] = 0
	}
	a := t.app[:]
	for k := e - 1; k >= b0; k-- {
		j := k - b0
		alpha := t.alpha[j*ns : (j+1)*ns]
		bm := t.bm[j*m.windows : (j+1)*m.windows]
		for v := range a {
			a[v] = negInf
		}
		for s := range ns {
			t.next[s] = negInf
		}
		for w := range m.windows {
			v := bm[w]
			if v == negInf {
				continue
			}
			s, s2 := w>>2, w&mask
			b := v + beta[s2]
			if b > t.next[s] {
				t.next[s] = b
			}
			if p := alpha[s] + b; p > a[w&3] {
				a[w&3] = p
			}
		}
		beta, t.next = t.next, beta
		if k < b1 {
			src.runKeep(k, a)
		}
	}
	t.beta = beta
}

// best returns the highest path metric at the end of the last run.
func (t *trellis) best() float32 {
	b := negInf
	for _, v := range t.last {
		b = max(b, v)
	}
	return b
}

// phasePulse holds the phase response of one symbol of value 1: q[i] is the
// share of the step h*pi reached i-half samples from the symbol center.
type phasePulse struct {
	q    []float64
	q32  []float32
	half int
	span int
}

// newPhasePulse returns the phase pulse at sps samples per symbol.
func newPhasePulse(sps int) phasePulse {
	taps := rrcTapsAt(float64(sps))
	p := phasePulse{q: make([]float64, len(taps)+1), q32: make([]float32, len(taps)+1), half: len(taps) / 2}
	for i, v := range taps {
		p.q[i+1] = p.q[i] + v
		p.q32[i+1] = float32(p.q[i+1])
	}
	p.span = p.half/sps + 1
	return p
}

// refPhaseInto writes the phase at each sample of the symbols syms, as
// refPhase, into out using the pulse p.
func (p *phasePulse) refPhaseInto(syms []int, sps int, h float64, out []float64) {
	var done float64
	n := len(p.q) - 1
	for k := range syms {
		if i := k - p.span - 1; i >= 0 {
			done += h * math.Pi * float64(syms[i])
		}
		for j := range sps {
			t := k*sps + j - sps/2
			ph := done
			for i := max(k-p.span, 0); i <= min(k+p.span, len(syms)-1); i++ {
				idx := t - i*sps + p.half
				switch {
				case idx >= n:
					ph += h * math.Pi * float64(syms[i])
				case idx >= 0:
					ph += h * math.Pi * float64(syms[i]) * p.q[idx+1]
				}
			}
			out[k*sps+j] = ph
		}
	}
}

// refPhase returns the phase at each sample of the symbols syms when
// symbols before and after them are 0: n*sps samples, sample j of symbol k
// at offset j-sps/2 from its center, phase 0 before the first symbol.
func refPhase(syms []int, sps int, h float64) []float64 {
	taps := rrcTapsAt(float64(sps))
	half := len(taps) / 2
	q := make([]float64, len(taps)+1)
	for i, v := range taps {
		q[i+1] = q[i] + v
	}
	span := half/sps + 1
	out := make([]float64, len(syms)*sps)
	var done float64
	for k := range syms {
		if i := k - span - 1; i >= 0 {
			done += h * math.Pi * float64(syms[i])
		}
		for j := range sps {
			t := k*sps + j - sps/2
			ph := done
			for i := max(k-span, 0); i <= min(k+span, len(syms)-1); i++ {
				idx := t - i*sps + half
				switch {
				case idx >= len(taps):
					ph += h * math.Pi * float64(syms[i])
				case idx >= 0:
					ph += h * math.Pi * float64(syms[i]) * q[idx+1]
				}
			}
			out[k*sps+j] = ph
		}
	}
	return out
}

// dibitOfDigit returns the dibit of symbol digit v (symbol 2v-3).
func dibitOfDigit(v int) (bit.Bit, bit.Bit) {
	switch v {
	case 3:
		return 0, 1
	case 2:
		return 0, 0
	case 1:
		return 1, 0
	}
	return 1, 1
}

// anchor estimates the carried phase before known symbols syms (values
// -3..3) at the start of aligned samples x.
func (m *cpmModel) anchor(x []complex64, syms []int) complex64 {
	sps, d := m.sps, m.d
	steps := len(syms) - 2*d
	var c complex128
	ph := 0.0
	for k := range steps {
		w := 0
		for i := range 2*d + 1 {
			w = w<<2 | (syms[k+i]+3)/2
		}
		var s complex128
		for j := range sps {
			s += complex128(x[(k+d)*sps+j] * m.tpl[w*sps+j])
		}
		c += s * cmplx.Exp(complex(0, -ph))
		ph += m.h * math.Pi * float64(syms[k])
	}
	return complex64(cmplx.Conj(c) / complex(cmplx.Abs(c), 0))
}
