package rf

import "math"

const (
	phaseStates  = 200
	driftPenalty = 3
	exactBeam    = 40
	anchorKappa  = 20
)

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
	row      int
	off      [4]int
	lp       []float32
	slp      []float32
	logmap   bool
	pen      float32
	drifts   int
	jump     float32
	fade     bool
	below    float32
	jk       []float32
}

var jacobian = [128]float32{0.69314718, 0.63259904, 0.57593942, 0.52312326, 0.47407698, 0.42870068, 0.38687101, 0.34844458, 0.31326169, 0.28115014, 0.25192908, 0.22541265, 0.20141328, 0.17974464, 0.16022415, 0.14267506, 0.12692801, 0.11282228, 0.10020656, 0.088939465, 0.078889734, 0.0699362, 0.061967589, 0.054882179, 0.048587352, 0.042999079, 0.038041372, 0.0336457, 0.029750418, 0.026300195, 0.023245464, 0.020541901, 0.018149928, 0.016034256, 0.014163457, 0.01250957, 0.011047745, 0.009755911, 0.0086144838, 0.0076060944, 0.0067153485, 0.0059286084, 0.0052337982, 0.004620229, 0.0040784433, 0.0036000751, 0.0031777265, 0.0028048569, 0.0024756851, 0.002185102, 0.0019285932, 0.0017021703, 0.0015023102, 0.001325901, 0.0011701947, 0.0010327642, 0.00091146645, 0.00080440939, 0.00070992233, 0.00062653039, 0.00055293148, 0.00048797616, 0.0004306498, 0.00038005673, 0.00033540637, 0.00029600092, 0.00026122444, 0.00023053329, 0.00020344767, 0.00017954409, 0.00015844877, 0.00013983185, 0.00012340219, 0.00010890284, 9.6107034e-05, 8.4814638e-05, 7.4849029e-05, 6.6054326e-05, 5.8292965e-05, 5.1443537e-05, 4.5398899e-05, 4.0064495e-05, 3.5356876e-05, 3.1202398e-05, 2.753607e-05, 2.4300536e-05, 2.1445178e-05, 1.8925327e-05, 1.6701561e-05, 1.4739091e-05, 1.3007213e-05, 1.1478834e-05, 1.0130042e-05, 8.9397363e-06, 7.8892937e-06, 6.9622805e-06, 6.1441935e-06}

var jacobianSlope = [128]float32{-0.060548127, -0.05665964, -0.052816153, -0.049046278, -0.0453763, -0.041829675, -0.03842643, -0.035182893, -0.032111555, -0.029221058, -0.026516423, -0.023999378, -0.021668628, -0.019520491, -0.017549098, -0.01574704, -0.014105737, -0.012615718, -0.011267096, -0.010049731, -0.008953534, -0.007968612, -0.007085409, -0.006294828, -0.0055882744, -0.0049577057, -0.004395671, -0.0038952827, -0.0034502223, -0.0030547325, -0.0027035624, -0.0023919735, -0.0021156706, -0.0018707998, -0.0016538864, -0.0014618253, -0.0012918338, -0.0011414271, -0.00100839, -0.0008907458, -0.00078673987, -0.0006948104, -0.0006135693, -0.00054178573, -0.00047836802, -0.00042234873, -0.00037286943, -0.00032917177, -0.00029058312, -0.00025650894, -0.0002264228, -0.0001998602, -0.00017640914, -0.00015570631, -0.0001374305, -0.00012129778, -0.00010705704, -9.448704e-05, -8.339196e-05, -7.359893e-05, -6.49553e-05, -5.7326368e-05, -5.059305e-05, -4.465037e-05, -3.940545e-05, -3.4776487e-05, -3.069114e-05, -2.7085625e-05, -2.3903573e-05, -2.1095315e-05, -1.861692e-05, -1.6429665e-05, -1.4499354e-05, -1.2795805e-05, -1.1292395e-05, -9.96561e-06, -8.794705e-06, -7.761359e-06, -6.8494264e-06, -6.04464e-06, -5.334401e-06, -4.707621e-06, -4.154477e-06, -3.6663278e-06, -3.2355347e-06, -2.8553586e-06, -2.5198497e-06, -2.2237673e-06, -1.9624695e-06, -1.731878e-06, -1.5283795e-06, -1.3487916e-06, -1.1903057e-06, -1.0504427e-06, -9.2701293e-07, -8.180873e-07}

func logAdd(a, b float32) float32 {
	d := a - b
	if d < 0 {
		a, d = b, -d
	}
	x := 8 * d
	if x >= 96 {
		return a
	}
	i := int(x)
	f := x - float32(i)
	return a + jacobian[i&127] + f*jacobianSlope[i&127]
}

func newXTrellis(m *cpmModel, np, maxSteps int) *xtrellis {
	n := m.states * np
	t := &xtrellis{
		m: m, maxSteps: maxSteps, np: np, step: int(math.Round(0.135 * float64(np))),
		alpha: make([]float32, (maxSteps+1)*n), beta: make([]float32, n), next: make([]float32, n),
		corr: make([]complex64, maxSteps*m.windows), top: make([]float32, maxSteps+1), beam: make([]float32, maxSteps),
		lp: make([]float32, 4*maxSteps), slp: make([]float32, 8*m.d),
		pen: driftPenalty, drifts: 1, jk: make([]float32, maxSteps),
	}
	if np != phaseStates {
		t.row = 1
	}
	for v := range t.off {
		t.off[v] = t.phaseAfter(0, v)
	}
	return t
}

func (t *xtrellis) setH(h float64) {
	t.step = int(math.Round(h / 2 * float64(t.np)))
	for v := range t.off {
		t.off[v] = t.phaseAfter(0, v)
	}
}

type xlanes struct {
	re, im, bonus [4]float32
}

func (t *xtrellis) forward(src, d0, d1, d2, d3 []float32, lo int, l *xlanes, thr, top float32, first bool) float32 {
	n := len(src)
	d0, d1, d2, d3 = d0[:n], d1[:n], d2[:n], d3[:n]
	cos, sin := phaseCos[t.row][lo:lo+n], phaseSin[t.row][lo:lo+n]
	re0, re1, re2, re3 := l.re[0], l.re[1], l.re[2], l.re[3]
	im0, im1, im2, im3 := l.im[0], l.im[1], l.im[2], l.im[3]
	b0, b1, b2, b3 := l.bonus[0], l.bonus[1], l.bonus[2], l.bonus[3]
	if first {
		for i, a := range src {
			if a < thr {
				d0[i], d1[i], d2[i], d3[i] = negInf, negInf, negInf, negInf
				continue
			}
			c, s := cos[i], sin[i]
			v0 := a + re0*c + im0*s + b0
			v1 := a + re1*c + im1*s + b1
			v2 := a + re2*c + im2*s + b2
			v3 := a + re3*c + im3*s + b3
			d0[i], d1[i], d2[i], d3[i] = v0, v1, v2, v3
			if v0 > top {
				top = v0
			}
			if v1 > top {
				top = v1
			}
			if v2 > top {
				top = v2
			}
			if v3 > top {
				top = v3
			}
		}
		return top
	}
	if t.logmap {
		for i, a := range src {
			if a < thr {
				continue
			}
			c, s := cos[i], sin[i]
			v0 := logAdd(d0[i], a+re0*c+im0*s+b0)
			v1 := logAdd(d1[i], a+re1*c+im1*s+b1)
			v2 := logAdd(d2[i], a+re2*c+im2*s+b2)
			v3 := logAdd(d3[i], a+re3*c+im3*s+b3)
			d0[i], d1[i], d2[i], d3[i] = v0, v1, v2, v3
			if v0 > top {
				top = v0
			}
			if v1 > top {
				top = v1
			}
			if v2 > top {
				top = v2
			}
			if v3 > top {
				top = v3
			}
		}
		return top
	}
	for i, a := range src {
		if a < thr {
			continue
		}
		c, s := cos[i], sin[i]
		if v := a + re0*c + im0*s + b0; v > d0[i] {
			d0[i] = v
			if v > top {
				top = v
			}
		}
		if v := a + re1*c + im1*s + b1; v > d1[i] {
			d1[i] = v
			if v > top {
				top = v
			}
		}
		if v := a + re2*c + im2*s + b2; v > d2[i] {
			d2[i] = v
			if v > top {
				top = v
			}
		}
		if v := a + re3*c + im3*s + b3; v > d3[i] {
			d3[i] = v
			if v > top {
				top = v
			}
		}
	}
	return top
}

func (t *xtrellis) backward(src, e0, e1, e2, e3, dst []float32, lo int, l *xlanes, thr float32, best *[4]float32) {
	n := len(src)
	e0, e1, e2, e3, dst = e0[:n], e1[:n], e2[:n], e3[:n], dst[:n]
	cos, sin := phaseCos[t.row][lo:lo+n], phaseSin[t.row][lo:lo+n]
	re0, re1, re2, re3 := l.re[0], l.re[1], l.re[2], l.re[3]
	im0, im1, im2, im3 := l.im[0], l.im[1], l.im[2], l.im[3]
	b0, b1, b2, b3 := l.bonus[0], l.bonus[1], l.bonus[2], l.bonus[3]
	p0, p1, p2, p3 := best[0], best[1], best[2], best[3]
	if t.logmap {
		for i, a := range src {
			if a < thr {
				dst[i] = negInf
				continue
			}
			c, s := cos[i], sin[i]
			v0 := re0*c + im0*s + b0 + e0[i]
			v1 := re1*c + im1*s + b1 + e1[i]
			v2 := re2*c + im2*s + b2 + e2[i]
			v3 := re3*c + im3*s + b3 + e3[i]
			dst[i] = logAdd(logAdd(logAdd(v0, v1), v2), v3)
			p0 = logAdd(p0, a+v0)
			p1 = logAdd(p1, a+v1)
			p2 = logAdd(p2, a+v2)
			p3 = logAdd(p3, a+v3)
		}
		best[0], best[1], best[2], best[3] = p0, p1, p2, p3
		return
	}
	for i, a := range src {
		if a < thr {
			dst[i] = negInf
			continue
		}
		c, s := cos[i], sin[i]
		v0 := re0*c + im0*s + b0 + e0[i]
		v1 := re1*c + im1*s + b1 + e1[i]
		v2 := re2*c + im2*s + b2 + e2[i]
		v3 := re3*c + im3*s + b3 + e3[i]
		x := v0
		if v1 > x {
			x = v1
		}
		if v2 > x {
			x = v2
		}
		if v3 > x {
			x = v3
		}
		if a+v0 > p0 {
			p0 = a + v0
		}
		if a+v1 > p1 {
			p1 = a + v1
		}
		if a+v2 > p2 {
			p2 = a + v2
		}
		if a+v3 > p3 {
			p3 = a + v3
		}
		dst[i] = x
	}
	best[0], best[1], best[2], best[3] = p0, p1, p2, p3
}

func (t *xtrellis) phaseAfter(p, v int) int {
	return ((p+t.step*(2*v-3))%t.np + t.np) % t.np
}

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
	var mean float32
	for k := range steps {
		seg := x[k*sps : (k+1)*sps]
		corr := t.corr[k*m.windows : (k+1)*m.windows]
		var cm float32
		for w := 0; w < m.windows; w += 2 {
			tp0, tp1 := m.tpl[w*sps:(w+1)*sps], m.tpl[(w+1)*sps:(w+2)*sps]
			tp0, tp1 = tp0[:len(seg)], tp1[:len(seg)]
			var c0, c1 complex64
			for j, v := range seg {
				c0 += v * tp0[j]
				c1 += v * tp1[j]
			}
			c0 = c0 * rot0 * complex(scale, 0)
			c1 = c1 * rot0 * complex(scale, 0)
			corr[w], corr[w+1] = c0, c1
			cm = max(cm, real(c0)*real(c0)+imag(c0)*imag(c0), real(c1)*real(c1)+imag(c1)*imag(c1))
		}
		a := float32(math.Sqrt(float64(cm)))
		t.beam[k] = exactBeam * max(1, a)
		t.jk[k] = a
		mean += a
	}
	t.jumpCosts(steps, mean)
	for k := range steps {
		alpha := t.alpha[k*n : (k+1)*n]
		next := t.alpha[(k+1)*n : (k+2)*n]
		corr := t.corr[k*m.windows : (k+1)*m.windows]
		thr := t.top[k] - t.beam[k]
		top := negInf
		lp := t.lp[4*k : 4*k+4]
		var l xlanes
		var dst [4][]float32
		for s := range ns {
			for d := range 4 {
				w := 4*s + d
				l.re[d], l.im[d], l.bonus[d] = real(corr[w]), imag(corr[w]), lp[d]
				s2 := w & mask
				dst[d] = next[s2*np : (s2+1)*np]
			}
			src := alpha[s*np : (s+1)*np]
			v := (4 * s) >> shift
			off := t.off[v]
			top = t.forward(src[:np-off], dst[0][off:], dst[1][off:], dst[2][off:], dst[3][off:], 0, &l, thr, top, v == 0)
			top = t.forward(src[np-off:], dst[0][:off], dst[1][:off], dst[2][:off], dst[3][:off], np-off, &l, thr, top, v == 0)
		}
		t.top[k+1] = top
		for range t.drifts {
			t.drift(next, ns, t.jk[k])
		}
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
		thr := t.top[k] - t.beam[k]
		var l xlanes
		var e [4][]float32
		var best [4]float32
		copy(best[:], a)
		for s := range ns {
			for d := range 4 {
				w := 4*s + d
				l.re[d], l.im[d], l.bonus[d] = real(corr[w]), imag(corr[w]), lp[d]
				s2 := w & mask
				e[d] = beta[s2*np : (s2+1)*np]
			}
			src := alpha[s*np : (s+1)*np]
			dst := t.next[s*np : (s+1)*np]
			off := t.off[(4*s)>>shift]
			t.backward(src[:np-off], e[0][off:], e[1][off:], e[2][off:], e[3][off:], dst[:np-off], 0, &l, thr, &best)
			t.backward(src[np-off:], e[0][:off], e[1][:off], e[2][:off], e[3][:off], dst[np-off:], np-off, &l, thr, &best)
		}
		copy(a, best[:])
		beta, t.next = t.next, beta
		for range t.drifts {
			t.drift(beta, ns, t.jk[max(k-1, 0)])
		}
	}
	t.beta = beta
}

func (t *xtrellis) jumpCosts(steps int, sum float32) {
	jk := t.jk[:steps]
	if !t.fade || sum <= 0 {
		for k := range jk {
			jk[k] = t.jump
		}
		return
	}
	mean := sum / float32(steps)
	for k := range jk {
		a := jk[k]
		if k+1 < steps {
			a = min(a, jk[k+1])
		}
		a /= mean
		switch {
		case t.below > 0 && a >= t.below:
			jk[k] = 0
		default:
			jk[k] = max(t.jump*a*a, 1e-3)
		}
	}
}

func (t *xtrellis) drift(m []float32, ns int, jump float32) {
	for s := range ns {
		b := m[s*t.np : (s+1)*t.np]
		first, prev := b[0], b[t.np-1]
		for p := range t.np {
			cur := b[p]
			nxt := first
			if p+1 < t.np {
				nxt = b[p+1]
			}
			b[p] = max(cur, max(prev, nxt)-t.pen)
			prev = cur
		}
		if jump > 0 {
			top := negInf
			for _, v := range b {
				top = max(top, v)
			}
			top -= jump
			for p, v := range b {
				b[p] = max(v, top)
			}
		}
	}
}
