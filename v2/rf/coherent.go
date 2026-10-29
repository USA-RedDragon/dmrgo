package rf

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

type cpmModel struct {
	d, sps  int
	states  int
	windows int
	tpl     []complex64
	step    [4]complex64
	pp      phasePulse
	res     int
	h       float64
}

func newCPMModel(d, sps int) *cpmModel {
	res := sps
	if sps == 2 {
		res = samplesPerSy
	}
	m := &cpmModel{d: d, sps: sps, states: 1 << (4 * d), windows: 1 << (4*d + 2), pp: newPhasePulse(res), res: res}
	m.tpl = make([]complex64, m.windows*sps)
	m.setH(cohIndex)
	return m
}

func (m *cpmModel) setH(h float64) {
	for w := range m.windows {
		m.setWindow(w, h)
	}
	m.setStep(h)
}

func (m *cpmModel) setOuterH(h float64) {
	for w := range m.windows {
		outer := true
		for i := range 2*m.d + 1 {
			if digit := (w >> (2 * i)) & 3; digit != 0 && digit != 3 {
				outer = false
			}
		}
		if outer {
			m.setWindow(w, h)
		}
	}
	m.setStep(h)
}

func (m *cpmModel) setWindow(w int, h float64) {
	sps, d := m.sps, m.d
	for j := range sps {
		off := (j - sps/2) * (m.res / sps)
		var ph float64
		for i := -d; i <= d; i++ {
			digit := (w >> (2 * (d - i))) & 3
			ph += float64(2*digit-3) * m.pulse(off-i*m.res, h)
		}
		m.tpl[w*sps+j] = complex64(complex(math.Cos(ph), -math.Sin(ph)))
	}
}

func (m *cpmModel) setStep(h float64) {
	for v := range 4 {
		ph := h * math.Pi * float64(2*v-3)
		m.step[v] = complex64(complex(math.Cos(ph), -math.Sin(ph)))
	}
	m.h = h
}

func (m *cpmModel) pulse(j int, h float64) float64 {
	n := m.pp.n
	i := j + n/2
	switch {
	case i < 0:
		return 0
	case i >= n:
		return h * math.Pi
	}
	return h * math.Pi * pulseQ[m.pp.row][i+1]
}

type trellisSource interface {
	runSamples(k0, k1 int, x []complex64)
	runPrior(k int, lp []float32)
	runKeep(k int, app []float32)
}

type trellis struct {
	m         *cpmModel
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
	saveA     []float32
	saveR     []complex64
	last      []float32
	app       [4]float32
	slp       []float32
	rows      []complex64
	rowsOK    bool
}

func newTrellis(m *cpmModel, maxSteps, win, look int) *trellis {
	win = min(win, maxSteps)
	look = min(look, maxSteps-win)
	n := win + look
	return &trellis{
		m: m, win: win, look: look, x: make([]complex64, n*m.sps), lp: make([]float32, 4*n),
		alpha: make([]float32, (n+1)*m.states), beta: make([]float32, m.states), next: make([]float32, m.states),
		rot: make([]complex64, m.states), nrot: make([]complex64, m.states),
		bm: make([]float32, n*m.windows), corr: make([]complex64, m.windows),
		saveA: make([]float32, m.states), saveR: make([]complex64, m.states), last: make([]float32, m.states),
		slp: make([]float32, 8*m.d),
	}
}

const negInf = float32(-1e30)

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

func (t *trellis) forward(j int, lp []float32, scale, mu float32) {
	m := t.m
	ns := m.states
	shift := 4 * m.d
	corr := t.corr[:m.windows]
	if t.rows != nil {
		corr = t.rows[j*m.windows : (j+1)*m.windows]
	}
	if t.rows == nil || !t.rowsOK {
		t.correlate(j, corr)
	}
	alpha := t.alpha[j*ns : (j+1)*ns]
	next := t.alpha[(j+1)*ns : (j+2)*ns]
	bm := t.bm[j*m.windows : (j+1)*m.windows]
	rot, nrot, lp4 := t.rot[:ns], t.nrot[:ns], lp[:4]
	for s2 := range next {
		bonus := lp4[s2&3]
		best, bd := negInf, 0
		var bro, bc complex64
		for d0 := range 4 {
			w := d0<<shift | s2
			ro, c := rot[w>>2], corr[w]
			v := scale*(real(ro)*real(c)-imag(ro)*imag(c)) + bonus
			bm[w] = v
			if a := alpha[w>>2] + v; a > best {
				best, bd, bro, bc = a, d0, ro, c
			}
		}
		next[s2] = best
		r := bro * m.step[bd&3]
		if mu != 0 && best > negInf {
			c := bro * bc
			if a := sqrtf(real(c)*real(c) + imag(c)*imag(c)); a > 0 {
				e := mu * imag(c) / a
				r *= complex(1, -e)
			}
		}
		nrot[s2] = r
	}
	t.rot, t.nrot = t.nrot, t.rot
}

func (t *trellis) correlate(j int, corr []complex64) {
	m := t.m
	sps := m.sps
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
		corr[w] = complex(ac-bd, ad+bc)
		corr[m.windows-1-w] = complex(ac+bd, bc-ad)
	}
}

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
		a0, a1, a2, a3 := negInf, negInf, negInf, negInf
		next := t.next[:ns]
		for s := range next {
			w := s << 2
			q := bm[w : w+4 : w+4]
			bt := beta[w&mask : w&mask+4 : w&mask+4]
			b0, b1, b2, b3 := q[0]+bt[0], q[1]+bt[1], q[2]+bt[2], q[3]+bt[3]
			next[s] = max(max(b0, b1), max(b2, b3))
			as := alpha[s]
			a0, a1, a2, a3 = max(a0, as+b0), max(a1, as+b1), max(a2, as+b2), max(a3, as+b3)
		}
		a[0], a[1], a[2], a[3] = a0, a1, a2, a3
		beta, t.next = t.next, beta
		if k < b1 {
			src.runKeep(k, a)
		}
	}
	t.beta = beta
}

func (t *trellis) best() float32 {
	b := negInf
	for _, v := range t.last {
		b = max(b, v)
	}
	return b
}

type phasePulse struct {
	row  int
	n    int
	half int
	span int
}

func newPhasePulse(sps int) phasePulse {
	p := phasePulse{row: samplesPerSy/sps - 1, n: 2*rrcSpan*sps + 1}
	p.half = p.n / 2
	p.span = p.half/sps + 1
	return p
}

func (p *phasePulse) refPhaseInto(syms []int, sps int, h float64, out []float64) {
	var done float64
	n := p.n
	q := &pulseQ[p.row]
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
					ph += h * math.Pi * float64(syms[i]) * q[idx+1]
				}
			}
			out[k*sps+j] = ph
		}
	}
}

func dibitOfDigit(v int) (unpacked.Bit, unpacked.Bit) {
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
	if c == 0 {
		return 1
	}
	return complex64(cmplx.Conj(c) / complex(cmplx.Abs(c), 0))
}
