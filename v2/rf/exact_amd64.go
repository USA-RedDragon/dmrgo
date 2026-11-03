//go:build amd64 && !tinygo

package rf

func xtISA() uint8

//go:noescape
func xfwdLogAVX2(src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32

//go:noescape
func xfwdFirstAVX2(src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32

//go:noescape
func xbwdLogAVX2(src, e0, e1, e2, e3, dst, cs, sn *float32, n int, l *xlanes, thr float32, best *[4]float32)

//go:noescape
func xfwdLog512(src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32

//go:noescape
func xbwdLog512(src, e0, e1, e2, e3, dst, cs, sn *float32, n int, l *xlanes, thr float32, best *[4]float32)

//go:noescape
func xcorrAVX2(seg *complex64, sps int, tre, tim *float32, nw int, rr, ri, scale float32, corr *complex64) float32

func (t *xtrellis) corrPrep() bool {
	m := t.m
	if t.isa == 0 || m.windows%4 != 0 {
		return false
	}
	nw, sps := m.windows, m.sps
	for w := range nw {
		for j, v := range m.tpl[w*sps : (w+1)*sps] {
			t.tre[j*nw+w], t.tim[j*nw+w] = real(v), imag(v)
		}
	}
	return true
}

func (t *xtrellis) corrSIMD(seg, corr []complex64, rot complex64, scale float32) float32 {
	nw := t.m.windows
	corr = corr[:nw]
	return xcorrAVX2(&seg[0], len(seg), &t.tre[0], &t.tim[0], nw, real(rot), imag(rot), scale, &corr[0])
}

//go:noescape
func xdriftAVX2(m *float32, ns, np int, ext *float32, pen float32)

func (t *xtrellis) driftSIMD(m []float32) bool {
	if t.isa == 0 || len(m) == 0 {
		return false
	}
	np := t.np
	xdriftAVX2(&m[0], len(m)/np, np, &t.ext[0], t.pen)
	return true
}

func (t *xtrellis) forwardSIMD(alpha, next []float32, corr []complex64, lp []float32, thr float32) float32 {
	m := t.m
	ns, np, mask, shift := m.states, t.np, m.states-1, 4*m.d
	cs, sn := phaseCos[t.row][:np], phaseSin[t.row][:np]
	alpha, next, corr, lp = alpha[:ns*np], next[:ns*np], corr[:4*ns], lp[:4]
	top := negInf
	var l xlanes
	for s := range ns {
		var r [4]int
		for d := range 4 {
			w := 4*s + d
			l.re[d], l.im[d], l.bonus[d] = real(corr[w]), imag(corr[w]), lp[d]
			r[d] = (w & mask) * np
		}
		src := alpha[s*np : (s+1)*np]
		v := (4 * s) >> shift
		off := t.off[v]
		if n := np - off; n > 0 {
			top = t.forwardCall(v == 0, &src[0], &next[r[0]+off], &next[r[1]+off], &next[r[2]+off], &next[r[3]+off], &cs[0], &sn[0], n, &l, thr, top)
		}
		if off > 0 {
			top = t.forwardCall(v == 0, &src[np-off], &next[r[0]], &next[r[1]], &next[r[2]], &next[r[3]], &cs[np-off], &sn[np-off], off, &l, thr, top)
		}
	}
	return top
}

func (t *xtrellis) forwardCall(first bool, src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32 {
	switch {
	case first:
		return xfwdFirstAVX2(src, d0, d1, d2, d3, cs, sn, n, l, thr, top)
	case t.isa > 1:
		return xfwdLog512(src, d0, d1, d2, d3, cs, sn, n, l, thr, top)
	}
	return xfwdLogAVX2(src, d0, d1, d2, d3, cs, sn, n, l, thr, top)
}

func (t *xtrellis) backwardSIMD(alpha, beta, out []float32, corr []complex64, lp []float32, thr float32, best *[4]float32) {
	m := t.m
	ns, np, mask, shift := m.states, t.np, m.states-1, 4*m.d
	cs, sn := phaseCos[t.row][:np], phaseSin[t.row][:np]
	alpha, beta, out, corr, lp = alpha[:ns*np], beta[:ns*np], out[:ns*np], corr[:4*ns], lp[:4]
	var l xlanes
	for s := range ns {
		var r [4]int
		for d := range 4 {
			w := 4*s + d
			l.re[d], l.im[d], l.bonus[d] = real(corr[w]), imag(corr[w]), lp[d]
			r[d] = (w & mask) * np
		}
		src, dst := alpha[s*np:(s+1)*np], out[s*np:(s+1)*np]
		off := t.off[(4*s)>>shift]
		if n := np - off; n > 0 {
			t.backwardCall(&src[0], &beta[r[0]+off], &beta[r[1]+off], &beta[r[2]+off], &beta[r[3]+off], &dst[0], &cs[0], &sn[0], n, &l, thr, best)
		}
		if off > 0 {
			t.backwardCall(&src[np-off], &beta[r[0]], &beta[r[1]], &beta[r[2]], &beta[r[3]], &dst[np-off], &cs[np-off], &sn[np-off], off, &l, thr, best)
		}
	}
}

func (t *xtrellis) backwardCall(src, e0, e1, e2, e3, dst, cs, sn *float32, n int, l *xlanes, thr float32, best *[4]float32) {
	if t.isa > 1 {
		xbwdLog512(src, e0, e1, e2, e3, dst, cs, sn, n, l, thr, best)
		return
	}
	xbwdLogAVX2(src, e0, e1, e2, e3, dst, cs, sn, n, l, thr, best)
}
