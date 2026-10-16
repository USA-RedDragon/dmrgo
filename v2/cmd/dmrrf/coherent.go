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
	taps := rrcTapsAt(sps)
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
func rrcTapsAt(sps int) []float64 {
	n := 2*rrcSpan*sps + 1
	h := make([]float64, n)
	var c float64
	for i := range h {
		t := float64(i-n/2) / float64(sps)
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

// trellis is a max-log BCJR detector over the windows of a cpmModel, with
// the phase of the symbols before the window carried per survivor (per
// survivor processing). Its buffers hold up to maxSteps symbol intervals.
type trellis struct {
	m        *cpmModel
	maxSteps int
	alpha    []float32
	beta     []float32
	next     []float32
	rot      []complex64
	nrot     []complex64
	bm       []float32
	corr     []complex64
	// lp holds, per step, the log prior of each digit of the newest symbol
	// of the window, and slp the log priors of the digits of the first
	// window state, oldest first. Zero means no prior.
	lp  []float32
	slp []float32
}

// newTrellis returns a trellis over model m for runs of up to maxSteps
// symbol intervals.
func newTrellis(m *cpmModel, maxSteps int) *trellis {
	return &trellis{
		m: m, maxSteps: maxSteps,
		alpha: make([]float32, (maxSteps+1)*m.states), beta: make([]float32, m.states), next: make([]float32, m.states),
		rot: make([]complex64, m.states), nrot: make([]complex64, m.states),
		bm: make([]float32, maxSteps*m.windows), corr: make([]complex64, m.windows),
		lp: make([]float32, 4*maxSteps), slp: make([]float32, 8*m.d),
	}
}

const negInf = float32(-1e30)

// run detects the symbols of steps intervals of x, whose interval k is
// x[k*sps:(k+1)*sps], the samples of symbol k+d centered on it, from the
// carried phase rot0 (e^-j*phase) before symbol 0, with the priors of lp
// and slp. scale is 2A/sigma^2 and mu the gain of the per-survivor phase
// tracking. The symbol log-APPs of symbols 2d to steps+2d-1 go to app, four
// per symbol.
func (t *trellis) run(x []complex64, steps int, rot0 complex64, scale, mu float32, app []float32) {
	m := t.m
	sps, ns := m.sps, m.states
	mask := ns - 1
	shift := 4 * m.d
	alpha := t.alpha[:ns]
	for s := range ns {
		var a float32
		for i := range 2 * m.d {
			a += t.slp[4*i+(s>>(2*(2*m.d-1-i)))&3]
		}
		alpha[s] = a
		t.rot[s] = rot0
	}
	for k := range steps {
		seg := x[k*sps : (k+1)*sps]
		for w := range m.windows {
			tp := m.tpl[w*sps : (w+1)*sps]
			var c complex64
			for j, v := range seg {
				c += v * tp[j]
			}
			t.corr[w] = c
		}
		alpha := t.alpha[k*ns : (k+1)*ns]
		next := t.alpha[(k+1)*ns : (k+2)*ns]
		bm := t.bm[k*m.windows : (k+1)*m.windows]
		lp := t.lp[4*k : 4*k+4]
		for s2 := range ns {
			bonus := lp[s2&3]
			best, bd := negInf, 0
			for d0 := range 4 {
				w := d0<<shift | s2
				v := scale*real(t.rot[w>>2]*t.corr[w]) + bonus
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
				if a := float32(math.Hypot(float64(real(c)), float64(imag(c)))); a > 0 {
					e := mu * imag(c) / a
					r *= complex(1, -e)
				}
			}
			t.nrot[s2] = r
		}
		t.rot, t.nrot = t.nrot, t.rot
	}
	beta := t.beta
	for s := range ns {
		beta[s] = 0
	}
	for k := steps - 1; k >= 0; k-- {
		alpha := t.alpha[k*ns : (k+1)*ns]
		bm := t.bm[k*m.windows : (k+1)*m.windows]
		a := app[4*k : 4*k+4]
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
	}
	t.beta = beta
}

// best returns the highest path metric at the end of the last run of
// that many steps.
func (t *trellis) best(steps int) float32 {
	ns := t.m.states
	b := negInf
	for _, v := range t.alpha[steps*ns : (steps+1)*ns] {
		b = max(b, v)
	}
	return b
}

// phasePulse holds the phase response of one symbol of value 1: q[i] is the
// share of the step h*pi reached i-half samples from the symbol center.
type phasePulse struct {
	q    []float64
	half int
	span int
}

// newPhasePulse returns the phase pulse at sps samples per symbol.
func newPhasePulse(sps int) phasePulse {
	taps := rrcTapsAt(sps)
	p := phasePulse{q: make([]float64, len(taps)+1), half: len(taps) / 2}
	for i, v := range taps {
		p.q[i+1] = p.q[i] + v
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
	taps := rrcTapsAt(sps)
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
// -3..3) at the start of aligned samples x, with the amplitude and the noise
// variance per sample.
func (m *cpmModel) anchor(x []complex64, syms []int) (rot complex64, amp, noise float64) {
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
	n := float64(steps * sps)
	amp = cmplx.Abs(c) / n
	rot = complex64(cmplx.Conj(c) / complex(cmplx.Abs(c), 0))
	var e float64
	ph = 0
	for k := range steps {
		w := 0
		for i := range 2*d + 1 {
			w = w<<2 | (syms[k+i]+3)/2
		}
		for j := range sps {
			ref := complex128(m.tpl[w*sps+j])
			want := complex(amp, 0) * cmplx.Conj(ref) * cmplx.Exp(complex(0, ph)) / complex128(rot)
			diff := complex128(x[(k+d)*sps+j]) - want
			e += real(diff)*real(diff) + imag(diff)*imag(diff)
		}
		ph += m.h * math.Pi * float64(syms[k])
	}
	return rot, amp, e / n
}
