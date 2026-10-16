package rf

import (
	"cmp"
	"math"
	"slices"
)

// ambeSearch is the state of the C0 and C1 search of floorEval.
type ambeSearch struct {
	// pn holds the C1 scrambling mask of each C0 info word.
	pn [1 << softAMBEC0]uint32
	// sym holds the stream symbols with a candidate written in.
	sym []int8
	m   floorModel
	// syms holds the burst symbols that carry C0 or C1 bits, and src the
	// code (1 for C0, 2 for C1, 0 for none) and code bit of each of their
	// two bits.
	syms []int
	src  [][2][2]int
	// best holds the highest model metric seen of each C0 info word.
	best [1 << softAMBEC0]float64
	// found holds the candidates with a higher model metric than the
	// transmitted value, whose metric is base.
	found []ambeCand
	base  float64
	// t0 and t1 are the transmitted C0 and C1 info words.
	t0, t1 uint32
}

// ambeCand is a C0 and C1 info word pair and its model metric.
type ambeCand struct {
	i0, i1 uint32
	m      float64
}

// eval returns the model metric of frame f of voice burst i with C0 info
// word i0 and C1 info word i1 and records it when it beats the
// transmitted value.
func (a *ambeSearch) eval(e *floorEval, i int, sl *softAMBEView, i0, i1 uint32) float64 {
	cw := [3]uint32{0, smallCodeword(sl.c0, i0), smallCodeword(sl.c1, i1) ^ a.pn[i0]}
	tb := &e.truth[i].burst
	base := i*slotSymbols + cachSymbols
	for x, k := range a.syms {
		var v [2]uint8
		for b, s := range a.src[x] {
			if s[0] == 0 {
				p := 2*k + b
				v[b] = tb[p/8] >> (7 - p%8) & 1
			} else {
				v[b] = uint8(cw[s[0]] >> s[1] & 1)
			}
		}
		a.sym[base+k] = dibitSymbol(v[0], v[1])
	}
	m := a.m.metric(a.sym)
	a.best[i0] = max(a.best[i0], m)
	if m > a.base && (i0 != a.t0 || i1 != a.t1) && len(a.found) < 64 {
		a.found = append(a.found, ambeCand{i0, i1, m})
	}
	return m
}

// ascend changes i0, then i1, each to its value of the highest model
// metric with the other fixed, while the metric improves.
func (a *ambeSearch) ascend(e *floorEval, i int, sl *softAMBEView, i0, i1 uint32) {
	top := a.eval(e, i, sl, i0, i1)
	for range 4 {
		changed := false
		for x := range uint32(1) << softAMBEC0 {
			if m := a.eval(e, i, sl, x, i1); m > top {
				top, i0, changed = m, x, true
			}
		}
		for y := range uint32(1) << softAMBEC0 {
			if m := a.eval(e, i, sl, i0, y); m > top {
				top, i1, changed = m, y, true
			}
		}
		if !changed {
			return
		}
	}
}

// setup sets the symbols, the model and the transmitted info words of the
// search of frame f of voice burst i.
func (a *ambeSearch) setup(e *floorEval, i int, sl *softAMBEView) {
	a.sym = append(a.sym[:0], e.f.sym...)
	e.setModel(i, &a.m)
	a.syms, a.src = a.syms[:0], a.src[:0]
	for k := range burstSymbols {
		var s [2][2]int
		has := false
		for b := range 2 {
			p := int16(2*k + b)
			if j := slices.Index(sl.c0.pos[:sl.c0.n], p); j >= 0 {
				s[b], has = [2]int{1, j}, true
			}
			if j := slices.Index(sl.c1.pos[:sl.c1.n], p); j >= 0 {
				s[b], has = [2]int{2, j}, true
			}
		}
		if has {
			a.syms = append(a.syms, k)
			a.src = append(a.src, s)
		}
	}
	tb := e.truth[i].burst
	var tcw0, tcw1 uint32
	for j, p := range sl.c0.pos[:sl.c0.n] {
		tcw0 |= uint32(tb[p/8]>>(7-p%8)&1) << j
	}
	for j, p := range sl.c1.pos[:sl.c1.n] {
		tcw1 |= uint32(tb[p/8]>>(7-p%8)&1) << j
	}
	for x := range uint32(1) << softAMBEC0 {
		if smallCodeword(sl.c0, x) == tcw0 {
			a.t0 = x
		}
		a.best[x] = math.Inf(-1)
	}
	for y := range uint32(1) << softAMBEC0 {
		if smallCodeword(sl.c1, y)^a.pn[a.t0] == tcw1 {
			a.t1 = y
		}
	}
	a.found = a.found[:0]
	a.base = math.Inf(1)
	a.base = a.eval(e, i, sl, a.t0, a.t1)
}

// c0c1Search reports whether the maximum likelihood search over C0 and C1
// of frame f of voice burst i, the genie giving C2 and C3, finds a value
// more likely than the transmitted one by the exact likelihood. It needs
// the genie detection of C0 and C1 in e.llr and e.app. It ranks by the
// model metric: each C0 value with the 8 C1 values of the lowest bit cost
// given that C0, all C1 values for the 16 C0 values that rank best, then
// the best C0 and C1 in turn while the metric improves, from the best
// value and from the transmitted one.
func (e *floorEval) c0c1Search(i, f int) bool {
	v := softAMBESlot(f)
	sl := &v
	a := e.as
	if a == nil {
		a = &ambeSearch{}
		for v := range a.pn {
			a.pn[v] = softAMBEPN(uint32(v))
		}
		e.as = a
	}
	a.setup(e, i, sl)
	n0, _, _ := e.span(i)
	var cond [softSmallMaxN][2]int32
	var partner [softSmallMaxN]int
	for j, p := range sl.c1.pos[:sl.c1.n] {
		partner[j] = slices.Index(sl.c0.pos[:sl.c0.n], p^1)
		if partner[j] < 0 {
			cond[j] = [2]int32{int32(e.llr[p]), int32(e.llr[p])}
			continue
		}
		x := 4 * (i*slotSymbols + cachSymbols + int(p)/2 - n0 - e.g.d)
		ap := e.app[x : x+4]
		if p%2 == 1 {
			cond[j] = [2]int32{int32(satLLR(ap[2] - ap[3])), int32(satLLR(ap[1] - ap[0]))}
		} else {
			cond[j] = [2]int32{int32(satLLR(ap[2] - ap[1])), int32(satLLR(ap[3] - ap[0]))}
		}
	}
	s := &e.dec.small
	b0, b1, top := uint32(0), uint32(0), math.Inf(-1)
	for x := range uint32(1) << softAMBEC0 {
		cw0 := smallCodeword(sl.c0, x)
		for j := range sl.c1.n {
			u := 0
			if partner[j] >= 0 {
				u = int(cw0 >> partner[j] & 1)
			}
			s.l[j] = cond[j][u]
		}
		softTables(s.tab, s.l[:sl.c1.n])
		s.top1.reset(softListMax)
		sl.id1.search(s.tab, a.pn[x], 0, 0, &s.top1)
		for c := range s.top1.n {
			if m := a.eval(e, i, sl, x, s.top1.info[c]); m > top {
				b0, b1, top = x, s.top1.info[c], m
			}
		}
	}
	order := make([]uint32, len(a.best))
	for x := range order {
		order[x] = uint32(x)
	}
	slices.SortFunc(order, func(x, y uint32) int { return cmp.Compare(a.best[y], a.best[x]) })
	for _, x := range order[:16] {
		for y := range uint32(1) << softAMBEC0 {
			if m := a.eval(e, i, sl, x, y); m > top {
				b0, b1, top = x, y, m
			}
		}
	}
	a.ascend(e, i, sl, b0, b1)
	a.ascend(e, i, sl, a.t0, a.t1)
	slices.SortFunc(a.found, func(x, y ambeCand) int { return cmp.Compare(y.m, x.m) })
	tb := e.truth[i].burst
	for _, c := range a.found {
		cand := tb
		softPutSmall(&cand, sl.id0, smallCodeword(sl.c0, c.i0))
		softPutSmall(&cand, sl.id1, smallCodeword(sl.c1, c.i1)^a.pn[c.i0])
		if e.beats(i, &cand) {
			return true
		}
	}
	return false
}

// trellisSearch is the state of the rate 3/4 search of floorEval.
type trellisSearch struct {
	sym []int8
	m   floorModel
	// t holds the inputs of the current path, the state after step k
	// being t[k].
	t    [softTrellisSteps]int
	base float64
	// found holds the paths with a higher model metric than the
	// transmitted one, whose metric is base.
	found [][softTrellisSteps]int
}

// setStep writes the codeword bits of step k of path a.t into the stream
// symbols of slot i.
func (a *trellisSearch) setStep(tr *softTrellis, i, k int) {
	state := 0
	if k > 0 {
		state = a.t[k-1]
	}
	lab := tr.label[state][a.t[k]]
	base := i*slotSymbols + cachSymbols
	for b, j := range tr.step[k] {
		p := int(tr.pos[j])
		n := base + p/2
		v := a.sym[n]
		one := lab>>(3-b)&1 != 0
		if p%2 == 0 && (v < 0) != one || p%2 == 1 && (v == 3 || v == -3) != one {
			a.sym[n] = flipSymbol(v, p%2)
		}
	}
}

// flipSymbol returns symbol value v with dibit bit b (0 or 1) flipped.
func flipSymbol(v int8, b int) int8 {
	if b == 0 {
		return -v
	}
	if v > 0 {
		return 4 - v
	}
	return -4 - v
}

// change sets the inputs of steps k to k+len(in)-1 of the path to in and
// old to the inputs they replace, and writes the changed steps.
func (a *trellisSearch) change(tr *softTrellis, i, k int, in, old []int) {
	for x, v := range in {
		old[x] = a.t[k+x]
		a.t[k+x] = v
	}
	for s := k; s <= min(k+len(in), softTrellisSteps-1); s++ {
		a.setStep(tr, i, s)
	}
}

// climb changes one or two inputs of the path at a time, keeping each
// change that raises the model metric, for up to sweeps passes over the
// path, and records the paths that beat the transmitted one.
func (a *trellisSearch) climb(tr *softTrellis, i int, sweeps int) {
	cur := a.m.metric(a.sym)
	var in, old [2]int
	for range sweeps {
		improved := false
		for k := range softTrellisSteps - 1 {
			for l := 1; l <= 2 && k+l <= softTrellisSteps-1; l++ {
				for c := range 8 << (3 * (l - 1)) {
					in[0], in[1] = c&7, c>>3
					if in[0] == a.t[k] || l == 2 && in[1] == a.t[k+1] {
						continue
					}
					a.change(tr, i, k, in[:l], old[:l])
					m := a.m.metric(a.sym)
					if m > a.base && len(a.found) < 64 {
						a.found = append(a.found, a.t)
					}
					if m > cur {
						cur, improved = m, true
						continue
					}
					a.change(tr, i, k, old[:l], in[:l])
				}
			}
		}
		if !improved {
			return
		}
	}
}

// rate34Search reports whether the maximum likelihood search over the
// rate 3/4 paths of data burst i finds a path more likely than the
// transmitted one by the exact likelihood. From the transmitted path it
// changes one or two inputs at a time while the model metric improves.
func (e *floorEval) rate34Search(i int) bool {
	tr := (&softTrellisTables)
	a := e.ts
	if a == nil {
		a = &trellisSearch{}
		e.ts = a
	}
	a.sym = append(a.sym[:0], e.f.sym...)
	e.setModel(i, &a.m)
	tb := e.truth[i].burst
	want, _ := tb.Rate34Payload()
	a.t = [softTrellisSteps]int{}
	for k := range softTrellisSteps - 1 {
		for b := range 3 {
			x := 3*k + b
			a.t[k] = a.t[k]<<1 | int(want[x/8]>>(7-x%8)&1)
		}
	}
	tt := a.t
	a.base = a.m.metric(a.sym)
	a.found = a.found[:0]
	a.climb(tr, i, 2)
	for _, t := range a.found {
		if t == tt {
			continue
		}
		cand := tb
		for k := range softTrellisSteps {
			state := 0
			if k > 0 {
				state = t[k-1]
			}
			lab := tr.label[state][t[k]]
			for b, j := range tr.step[k] {
				softPut(&cand, tr.pos[j], lab>>(3-b)&1 != 0)
			}
		}
		if e.beats(i, &cand) {
			return true
		}
	}
	return false
}
