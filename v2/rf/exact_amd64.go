//go:build amd64 && !tinygo

package rf

func xtHasAVX2() bool

//go:noescape
func xfwdLogAVX2(src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32

//go:noescape
func xfwdFirstAVX2(src, d0, d1, d2, d3, cs, sn *float32, n int, l *xlanes, thr, top float32) float32

//go:noescape
func xbwdLogAVX2(src, e0, e1, e2, e3, dst, cs, sn *float32, n int, l *xlanes, thr float32, best *[4]float32)

//go:noescape
func xdriftAVX2(b, ext *float32, n int, pen float32)

func (t *xtrellis) driftSIMD(b []float32) bool {
	if !t.simd {
		return false
	}
	np := len(b)
	ext := t.ext[:np+2]
	ext[0], ext[np+1] = b[np-1], b[0]
	copy(ext[1:], b)
	xdriftAVX2(&b[0], &ext[0], np, t.pen)
	return true
}

func (t *xtrellis) forwardSIMD(src, d0, d1, d2, d3, cs, sn []float32, l *xlanes, thr, top float32, first bool) (float32, bool) {
	n := len(src)
	if !t.simd || n == 0 || !first && !t.logmap {
		return top, false
	}
	d0, d1, d2, d3, cs, sn = d0[:n], d1[:n], d2[:n], d3[:n], cs[:n], sn[:n]
	if first {
		return xfwdFirstAVX2(&src[0], &d0[0], &d1[0], &d2[0], &d3[0], &cs[0], &sn[0], n, l, thr, top), true
	}
	return xfwdLogAVX2(&src[0], &d0[0], &d1[0], &d2[0], &d3[0], &cs[0], &sn[0], n, l, thr, top), true
}

func (t *xtrellis) backwardSIMD(src, e0, e1, e2, e3, dst, cs, sn []float32, l *xlanes, thr float32, best *[4]float32) bool {
	n := len(src)
	if !t.simd || n == 0 || !t.logmap {
		return false
	}
	e0, e1, e2, e3, dst, cs, sn = e0[:n], e1[:n], e2[:n], e3[:n], dst[:n], cs[:n], sn[:n]
	xbwdLogAVX2(&src[0], &e0[0], &e1[0], &e2[0], &e3[0], &dst[0], &cs[0], &sn[0], n, l, thr, best)
	return true
}
