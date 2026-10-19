package rf

import "math"

// Layout of the exact-phase trellis.
const (
	// phaseStates is the number of carrier phases of the exact-phase
	// trellis by default: a symbol a turns the phase by 27*a of them, which
	// is 0.27*pi*a.
	phaseStates = 200
	// driftPenalty is the cost, in nats, of a step of one phase state.
	driftPenalty = 3
	// exactBeam is how far under the best state a state can be and still
	// extend paths: in nats, or in branch metrics of the step when those are
	// larger.
	exactBeam = 40
	// anchorKappa is the concentration, in nats, of the prior on the
	// carrier phase that an anchor gives the first state.
	anchorKappa = 20
)

// xtrellis is a max-log BCJR detector over the windows of a cpmModel and
// the carrier phase: its state is the window of 2d symbols and one of np
// phases, so paths merge only with the same phase. A symbol a turns the
// phase by step*a phases, the nearest to 0.27*pi*a. Its buffers hold up
// to maxSteps symbol intervals.
type xtrellis struct {
	m        *cpmModel
	maxSteps int
	np, step int
	alpha    []float32
	beta     []float32
	next     []float32
	corr     []complex64
	top      []float32
	beam     []float32
	// row is the row of the cosine and sine of each phase in phaseCos and
	// phaseSin: 0 for phaseStates phases and 1 for 52.
	row int
	// off holds the phase states that each leaving symbol digit turns.
	off [4]int
	// lp and slp hold the digit priors as in trellis.
	lp  []float32
	slp []float32
	// logmap sums the paths exactly instead of taking the best.
	logmap bool
}

// jacobian holds log(1+exp(-i/8)) for i from 0 to 96.
var jacobian = [97]float32{0.69314718, 0.63259904, 0.57593942, 0.52312326, 0.47407698, 0.42870068, 0.38687101, 0.34844458, 0.31326169, 0.28115014, 0.25192908, 0.22541265, 0.20141328, 0.17974464, 0.16022415, 0.14267506, 0.12692801, 0.11282228, 0.10020656, 0.088939465, 0.078889734, 0.0699362, 0.061967589, 0.054882179, 0.048587352, 0.042999079, 0.038041372, 0.0336457, 0.029750418, 0.026300195, 0.023245464, 0.020541901, 0.018149928, 0.016034256, 0.014163457, 0.01250957, 0.011047745, 0.009755911, 0.0086144838, 0.0076060944, 0.0067153485, 0.0059286084, 0.0052337982, 0.004620229, 0.0040784433, 0.0036000751, 0.0031777265, 0.0028048569, 0.0024756851, 0.002185102, 0.0019285932, 0.0017021703, 0.0015023102, 0.001325901, 0.0011701947, 0.0010327642, 0.00091146645, 0.00080440939, 0.00070992233, 0.00062653039, 0.00055293148, 0.00048797616, 0.0004306498, 0.00038005673, 0.00033540637, 0.00029600092, 0.00026122444, 0.00023053329, 0.00020344767, 0.00017954409, 0.00015844877, 0.00013983185, 0.00012340219, 0.00010890284, 9.6107034e-05, 8.4814638e-05, 7.4849029e-05, 6.6054326e-05, 5.8292965e-05, 5.1443537e-05, 4.5398899e-05, 4.0064495e-05, 3.5356876e-05, 3.1202398e-05, 2.753607e-05, 2.4300536e-05, 2.1445178e-05, 1.8925327e-05, 1.6701561e-05, 1.4739091e-05, 1.3007213e-05, 1.1478834e-05, 1.0130042e-05, 8.9397363e-06, 7.8892937e-06, 6.9622805e-06, 6.1441935e-06}

// logAdd returns log(exp(a)+exp(b)), with the correction term interpolated
// in jacobian, within 5e-4 nats.
func logAdd(a, b float32) float32 {
	if a < b {
		a, b = b, a
	}
	x := 8 * (a - b)
	if x >= 96 {
		return a
	}
	i := int(x)
	f := x - float32(i)
	return a + jacobian[i] + f*(jacobian[i+1]-jacobian[i])
}

// newXTrellis returns an exact-phase trellis over model m with np carrier
// phases, phaseStates or 52, for runs of up to maxSteps symbol intervals.
func newXTrellis(m *cpmModel, np, maxSteps int) *xtrellis {
	n := m.states * np
	t := &xtrellis{
		m: m, maxSteps: maxSteps, np: np, step: int(math.Round(0.135 * float64(np))),
		alpha: make([]float32, (maxSteps+1)*n), beta: make([]float32, n), next: make([]float32, n),
		corr: make([]complex64, maxSteps*m.windows), top: make([]float32, maxSteps+1), beam: make([]float32, maxSteps),
		lp: make([]float32, 4*maxSteps), slp: make([]float32, 8*m.d),
	}
	if np != phaseStates {
		t.row = 1
	}
	for v := range t.off {
		t.off[v] = t.phaseAfter(0, v)
	}
	return t
}

// forward relaxes dst[i] with src[i] plus the branch metric at phase lo+i,
// skipping sources under thr, and returns top raised to the best value it
// wrote.
func (t *xtrellis) forward(src, dst []float32, lo int, re, im, bonus, thr, top float32) float32 {
	dst = dst[:len(src)]
	cos, sin := phaseCos[t.row][lo:lo+len(src)], phaseSin[t.row][lo:lo+len(src)]
	if t.logmap {
		for i, a := range src {
			if a < thr {
				continue
			}
			v := logAdd(dst[i], a+re*cos[i]+im*sin[i]+bonus)
			dst[i] = v
			top = max(top, v)
		}
		return top
	}
	for i, a := range src {
		if a < thr {
			continue
		}
		if v := a + re*cos[i] + im*sin[i] + bonus; v > dst[i] {
			dst[i] = v
			top = max(top, v)
		}
	}
	return top
}

// backward relaxes dst[i] with the branch metric plus beta[i], and returns
// best raised to the highest src[i] plus that, skipping the states whose
// forward metric src[i] is under thr, as no likely path passes them.
func (t *xtrellis) backward(src, beta, dst []float32, lo int, re, im, bonus, thr, best float32) float32 {
	beta, dst = beta[:len(src)], dst[:len(src)]
	cos, sin := phaseCos[t.row][lo:lo+len(src)], phaseSin[t.row][lo:lo+len(src)]
	if t.logmap {
		for i, a := range src {
			if a < thr {
				continue
			}
			v := re*cos[i] + im*sin[i] + bonus + beta[i]
			dst[i] = logAdd(dst[i], v)
			best = logAdd(best, a+v)
		}
		return best
	}
	for i, a := range src {
		if a < thr {
			continue
		}
		v := re*cos[i] + im*sin[i] + bonus + beta[i]
		if v > dst[i] {
			dst[i] = v
		}
		if a+v > best {
			best = a + v
		}
	}
	return best
}

// phaseAfter returns the phase state after symbol digit v leaves the window
// at phase p.
func (t *xtrellis) phaseAfter(p, v int) int {
	return ((p+t.step*(2*v-3))%t.np + t.np) % t.np
}

// run detects as trellis.run does, with the carrier phase rot0 before
// symbol 0 as a prior of kappa nats instead of a fixed phase; kappa 0 means
// an unknown phase.
func (t *xtrellis) run(x []complex64, steps int, rot0 complex64, scale, kappa float32, app []float32) {
	m := t.m
	sps, ns := m.sps, m.states
	np := t.np
	n := ns * np
	shift := 4 * m.d
	mask := ns - 1
	alpha := t.alpha[:n]
	t.top[0] = negInf
	for s := range ns {
		var a float32
		for i := range 2 * m.d {
			a += t.slp[4*i+(s>>(2*(2*m.d-1-i)))&3]
		}
		for p := range np {
			alpha[s*np+p] = a - kappa*(1-phaseCos[t.row][p])
		}
		t.top[0] = max(t.top[0], a)
	}
	for k := range steps {
		seg := x[k*sps : (k+1)*sps]
		corr := t.corr[k*m.windows : (k+1)*m.windows]
		var cm float32
		for w := range m.windows {
			tp := m.tpl[w*sps : (w+1)*sps]
			var c complex64
			for j, v := range seg {
				c += v * tp[j]
			}
			corr[w] = c * rot0 * complex(scale, 0)
			cm = max(cm, real(corr[w])*real(corr[w])+imag(corr[w])*imag(corr[w]))
		}
		t.beam[k] = exactBeam * max(1, float32(math.Sqrt(float64(cm))))
		alpha := t.alpha[k*n : (k+1)*n]
		next := t.alpha[(k+1)*n : (k+2)*n]
		for i := range next {
			next[i] = negInf
		}
		thr := t.top[k] - t.beam[k]
		top := negInf
		lp := t.lp[4*k : 4*k+4]
		for w := range m.windows {
			s, s2 := w>>2, w&mask
			re, im := real(corr[w]), imag(corr[w])
			bonus := lp[w&3]
			src := alpha[s*np : (s+1)*np]
			dst := next[s2*np : (s2+1)*np]
			off := t.off[w>>shift]
			top = t.forward(src[:np-off], dst[off:], 0, re, im, bonus, thr, top)
			top = t.forward(src[np-off:], dst[:off], np-off, re, im, bonus, thr, top)
		}
		t.top[k+1] = top
		t.drift(next, ns)
	}
	beta := t.beta
	for i := range beta {
		beta[i] = 0
	}
	for k := steps - 1; k >= 0; k-- {
		alpha := t.alpha[k*n : (k+1)*n]
		corr := t.corr[k*m.windows : (k+1)*m.windows]
		lp := t.lp[4*k : 4*k+4]
		a := app[4*k : 4*k+4]
		for v := range a {
			a[v] = negInf
		}
		for i := range t.next {
			t.next[i] = negInf
		}
		thr := t.top[k] - t.beam[k]
		for w := range m.windows {
			s, s2 := w>>2, w&mask
			re, im := real(corr[w]), imag(corr[w])
			bonus := lp[w&3]
			src := alpha[s*np : (s+1)*np]
			bsrc := beta[s2*np : (s2+1)*np]
			dst := t.next[s*np : (s+1)*np]
			off := t.off[w>>shift]
			best := a[w&3]
			best = t.backward(src[:np-off], bsrc[off:], dst[:np-off], 0, re, im, bonus, thr, best)
			best = t.backward(src[np-off:], bsrc[:off], dst[np-off:], np-off, re, im, bonus, thr, best)

			a[w&3] = best
		}
		beta, t.next = t.next, beta
		t.drift(beta, ns)
	}
	t.beta = beta
}

// drift lets each state take the metric of its neighbor phases at a cost
// of driftPenalty, so that a path follows a carrier phase that drifts by up
// to one phase state per symbol.
func (t *xtrellis) drift(m []float32, ns int) {
	for s := range ns {
		b := m[s*t.np : (s+1)*t.np]
		first, prev := b[0], b[t.np-1]
		for p := range t.np {
			cur := b[p]
			nxt := first
			if p+1 < t.np {
				nxt = b[p+1]
			}
			if t.logmap {
				b[p] = logAdd(cur, logAdd(prev, nxt)-driftPenalty)
			} else {
				b[p] = max(cur, max(prev, nxt)-driftPenalty)
			}
			prev = cur
		}
	}
}
