package main

import "math"

// Layout of the exact-phase trellis.
const (
	// phaseStates is the number of carrier phases of the exact-phase
	// trellis: a symbol a turns the phase by phaseStep*a of them, which is
	// 0.27*pi*a.
	phaseStates = 200
	phaseStep   = 27
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
// the carrier phase: its state is the window of 2d symbols and one of
// phaseStates phases, so paths merge only with the same phase. The
// modulation index must be 0.27. Its buffers hold up to maxSteps symbol
// intervals.
type xtrellis struct {
	m        *cpmModel
	maxSteps int
	alpha    []float32
	beta     []float32
	next     []float32
	corr     []complex64
	top      []float32
	beam     []float32
	cos, sin [phaseStates]float32
	// off holds the phase states that each leaving symbol digit turns.
	off [4]int
	// lp and slp hold the digit priors as in trellis.
	lp  []float32
	slp []float32
}

// newXTrellis returns an exact-phase trellis over model m for runs of up to
// maxSteps symbol intervals.
func newXTrellis(m *cpmModel, maxSteps int) *xtrellis {
	n := m.states * phaseStates
	t := &xtrellis{
		m: m, maxSteps: maxSteps,
		alpha: make([]float32, (maxSteps+1)*n), beta: make([]float32, n), next: make([]float32, n),
		corr: make([]complex64, maxSteps*m.windows), top: make([]float32, maxSteps+1), beam: make([]float32, maxSteps),
		lp: make([]float32, 4*maxSteps), slp: make([]float32, 8*m.d),
	}
	for p := range phaseStates {
		t.cos[p] = float32(math.Cos(2 * math.Pi * float64(p) / phaseStates))
		t.sin[p] = float32(math.Sin(2 * math.Pi * float64(p) / phaseStates))
	}
	for v := range t.off {
		t.off[v] = phaseAfter(0, v)
	}
	return t
}

// forward relaxes dst[i] with src[i] plus the branch metric at phase cos[i],
// sin[i], skipping sources under thr, and returns top raised to the best
// value it wrote.
func (t *xtrellis) forward(src, dst, cos, sin []float32, re, im, bonus, thr, top float32) float32 {
	dst = dst[:len(src)]
	cos, sin = cos[:len(src)], sin[:len(src)]
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
func (t *xtrellis) backward(src, beta, dst, cos, sin []float32, re, im, bonus, thr, best float32) float32 {
	beta, dst = beta[:len(src)], dst[:len(src)]
	cos, sin = cos[:len(src)], sin[:len(src)]
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
func phaseAfter(p, v int) int {
	return ((p+phaseStep*(2*v-3))%phaseStates + phaseStates) % phaseStates
}

// run detects as trellis.run does, with the carrier phase rot0 before
// symbol 0 as a prior of kappa nats instead of a fixed phase; kappa 0 means
// an unknown phase.
func (t *xtrellis) run(x []complex64, steps int, rot0 complex64, scale, kappa float32, app []float32) {
	m := t.m
	sps, ns := m.sps, m.states
	n := ns * phaseStates
	shift := 4 * m.d
	mask := ns - 1
	alpha := t.alpha[:n]
	t.top[0] = negInf
	for s := range ns {
		var a float32
		for i := range 2 * m.d {
			a += t.slp[4*i+(s>>(2*(2*m.d-1-i)))&3]
		}
		for p := range phaseStates {
			alpha[s*phaseStates+p] = a - kappa*(1-t.cos[p])
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
			src := alpha[s*phaseStates : (s+1)*phaseStates]
			dst := next[s2*phaseStates : (s2+1)*phaseStates]
			off := t.off[w>>shift]
			top = t.forward(src[:phaseStates-off], dst[off:], t.cos[:phaseStates-off], t.sin[:phaseStates-off], re, im, bonus, thr, top)
			top = t.forward(src[phaseStates-off:], dst[:off], t.cos[phaseStates-off:], t.sin[phaseStates-off:], re, im, bonus, thr, top)
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
			src := alpha[s*phaseStates : (s+1)*phaseStates]
			bsrc := beta[s2*phaseStates : (s2+1)*phaseStates]
			dst := t.next[s*phaseStates : (s+1)*phaseStates]
			off := t.off[w>>shift]
			best := a[w&3]
			best = t.backward(src[:phaseStates-off], bsrc[off:], dst[:phaseStates-off], t.cos[:phaseStates-off], t.sin[:phaseStates-off], re, im, bonus, thr, best)
			best = t.backward(src[phaseStates-off:], bsrc[:off], dst[phaseStates-off:], t.cos[phaseStates-off:], t.sin[phaseStates-off:], re, im, bonus, thr, best)
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
		b := m[s*phaseStates : (s+1)*phaseStates]
		first, prev := b[0], b[phaseStates-1]
		for p := range phaseStates {
			cur := b[p]
			nxt := first
			if p+1 < phaseStates {
				nxt = b[p+1]
			}
			b[p] = max(cur, max(prev, nxt)-driftPenalty)
			prev = cur
		}
	}
}

// best returns the highest path metric at the end of the last run of that
// many steps.
func (t *xtrellis) best(steps int) float32 {
	n := t.m.states * phaseStates
	b := negInf
	for _, v := range t.alpha[steps*n : (steps+1)*n] {
		b = max(b, v)
	}
	return b
}
