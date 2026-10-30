package rf

import (
	"math"
	"math/cmplx"
)

const (
	ddWindow     = 12
	runMargin    = 4
	phaseBank    = 8
	fillChunk    = trellisWin + trellisLook
	noRun        = math.MaxInt16
	maxPulseSpan = rrcSpan + 1
)

type symRef struct {
	slot int64
	k    int
}

func (g symRef) add(n int) symRef {
	x := g.k + n
	q := x / slotSymbols
	if x%slotSymbols < 0 {
		q--
	}
	return symRef{slot: g.slot + int64(q), k: x - q*slotSymbols}
}

func (g symRef) diff(h symRef) int {
	return int((g.slot-h.slot)*slotSymbols) + g.k - h.k
}

func (r *engine) anchorSym(g symRef) (int, bool) {
	s := r.slot(g.slot)
	if s.index != g.slot || !s.anchor || g.k < syncFirstSym || g.k >= syncFirstSym+syncSymbols {
		return 0, false
	}
	var syms [syncSymbols]int8
	syncSyms(s.a.pattern, &syms)
	return int(syms[g.k-syncFirstSym]), true
}

func (r *engine) symPrior(g symRef, lp []float32) {
	clear(lp)
	if a, ok := r.anchorSym(g); ok {
		lp[(a+3)/2] = r.opt.KnownPrior
	}
	s := r.slot(g.slot)
	if s.index != g.slot || !s.hasPrior {
		return
	}
	for d := range 4 {
		lp[d] += r.prior[4*g.k+d] / r.llrScale
	}
}

func (r *engine) fill(g symRef, n int, rev bool) {
	r.fillRange(g, 0, n, rev, r.buf)
}

func (r *engine) fillRange(g symRef, i0, i1 int, rev bool, x []complex64) {
	ch := &r.chz.ch
	dir := 1
	if rev {
		dir = -1
	}
	sps := r.model.sps
	sp := float64(dir*samplesPerSy) / float64(sps)
	step := complex64(r.rotAt(sp))
	h0 := g.add(dir * i0)
	t0 := r.symTime(h0.slot, h0.k) - float64(sps/2)*sp
	if rev {
		t0 += sp
	}
	symRot, symStep := complex64(r.rotAt(t0)), complex64(r.rotAt(float64(dir)*r.period/slotSymbols))
	base, u0 := ch.ringAt(t0, 0)
	du, su := float32(float64(dir)*r.period/slotSymbols/float64(ch.dec)), float32(sp/float64(ch.dec))
	var first int64
	var scr []complex64
	for i := i0; i < i1; i++ {
		u := u0 + float32(i-i0)*du
		if (i-i0)%fillChunk == 0 {
			first, scr = r.loadChunk(base, u, du, su, min(fillChunk, i1-i)*sps, rev)
		}
		off := float32(base - first)
		rot := symRot
		symRot *= symStep
		for j := range sps {
			v := ch.interpIn(scr, off+u+float32(j)*su) * rot
			rot *= step
			if rev {
				v = complex(real(v), -imag(v))
			}
			x[(i-i0)*sps+j] = v
		}
	}
}

func (r *engine) loadChunk(base int64, u, du, su float32, n int, rev bool) (int64, []complex64) {
	sps := r.model.sps
	span := float32(n/sps)*abs32(du) + float32(sps)*abs32(su)
	lo := u
	if rev {
		lo = u - span
	}
	first := base + int64(math.Floor(float64(lo))) - sincTaps
	m := min(int(span)+2*sincTaps+2, len(r.scr))
	r.chz.ch.gather(first, r.scr[:m])
	return first, r.scr[:m]
}

func (r *engine) runSamples(k0, k1 int, x []complex64) {
	d := r.model.d
	r.fillRange(r.runG, k0+d, k1+d, r.runDir < 0, x)
}

func (r *engine) runPrior(k int, lp []float32) {
	r.symPrior(r.runG.add(r.runDir*(k+2*r.model.d)), lp)
}

func (r *engine) runKeep(k int, app []float32) {
	r.keepAPP(r.runG, r.runDir, k, r.runSteps, app, r.runLo, r.runHi)
}

func (r *engine) keepAPP(g symRef, dir, k, steps int, app []float32, lo, hi symRef) {
	d := r.model.d
	if k >= steps-d {
		return
	}
	h := g.add(dir * (k + 2*d))
	if h.diff(lo) < 0 || h.diff(hi) > 0 {
		return
	}
	s := r.slot(h.slot)
	if s.index != h.slot {
		return
	}
	dist := k + 2*d
	st, p := r.symsOf(s)
	if dist < 0 || dist >= int(st.have[p+h.k]) || dist > math.MaxInt16 {
		return
	}
	st.set(p+h.k, app)
	st.have[p+h.k] = int16(dist)
}

func (r *engine) run(g symRef, n int, rev bool, lo, hi symRef) {
	m := r.model
	d := m.d
	n = min(n, r.maxRun)
	if n <= 2*d+1 {
		return
	}
	dir := 1
	if rev {
		dir = -1
	}
	var syms [syncSymbols]int
	nk := 0
	for nk < syncSymbols {
		a, ok := r.anchorSym(g.add(dir * nk))
		if !ok {
			break
		}
		syms[nk] = a
		nk++
	}
	steps := n - 2*d
	for i := range 2 * d {
		r.symPrior(g.add(dir*i), r.slp[4*i:4*i+4])
	}
	scale := float32(2 * r.amp / r.noise)
	if r.ex == nil {
		r.runG, r.runDir, r.runSteps, r.runLo, r.runHi = g, dir, steps, lo, hi
		if nk >= 2*d+2 {
			r.fillRange(g, 0, nk, rev, r.buf)
			rot := m.anchor(r.buf, syms[:nk])
			r.tr.run(r, steps, rot, scale, r.opt.PSPGain)
		} else {
			r.bankRun(steps, scale)
		}
		return
	}
	r.fill(g, n, rev)
	for k := range steps {
		r.symPrior(g.add(dir*(k+2*d)), r.lp[4*k:4*k+4])
	}
	if nk >= 2*d+2 {
		rot := m.anchor(r.buf, syms[:nk])
		r.ex.exec(steps, rot, scale, anchorKappa)
	} else {
		r.bankRun(steps, scale)
	}
	for k := range steps {
		r.keepAPP(g, dir, k, steps, r.ex.app[4*k:4*k+4], lo, hi)
	}
}

type discard struct{ *engine }

func (discard) runKeep(int, []float32) {}

func (r *engine) bankRun(steps int, scale float32) {
	if r.ex != nil {
		r.ex.exec(steps, 1, scale, 0)
		return
	}
	best := float32(negInf)
	bestP := 0
	for p := range phaseBank {
		rot := complex64(cmplx.Exp(complex(0, -2*math.Pi*float64(p)/phaseBank)))
		r.tr.run(discard{r}, steps, rot, scale, r.opt.PSPGain)
		if v := r.tr.best(); v > best {
			best, bestP = v, p
		}
	}
	rot := complex64(cmplx.Exp(complex(0, -2*math.Pi*float64(bestP)/phaseBank)))
	r.tr.run(r, steps, rot, scale, r.opt.PSPGain)
}

func (r *engine) coherentGap(first, j int64, final bool) {
	r.passAt(first + (j-first)/2)
	if r.amp == 0 || r.noise == 0 {
		return
	}
	lo := symRef{slot: first}
	hi := symRef{slot: j, k: slotSymbols - 1}
	left := r.slot(first).anchor && r.slot(first).index == first && (first < j || final)
	right := r.slot(j).anchor && r.slot(j).index == j && (!final || first != j)
	if right {
		hi = symRef{slot: j, k: syncFirstSym + syncSymbols - 1}
	}
	keep := symRef{slot: first}
	if r.slot(first).anchor {
		keep.k = syncFirstSym + syncSymbols
	}
	for g := keep; g.diff(hi) <= 0; g = g.add(1) {
		st, p := r.symsOf(r.slot(g.slot))
		st.have[p+g.k] = noRun
	}
	margin := r.model.d + runMargin
	if left {
		lo = symRef{slot: first, k: syncFirstSym}
	}
	span := hi.diff(lo) + 1
	if left && right {
		span = min(span, (span+1)/2+trellisLook)
	}
	if left {
		r.run(lo, span+margin, false, lo, hi)
	}
	if right {
		r.run(hi, span+margin, true, lo, hi)
	}
	if !left && !right {
		r.run(lo, hi.diff(lo)+1+margin, false, lo, hi)
	}
	if r.opt.TrackCarrier && !r.refining && !r.warm && !r.frozen {
		r.trackCarrier(lo, hi)
	}
}

func (r *engine) coherentBurst(j int64) {
	r.passAt(j)
	if r.amp == 0 || r.noise == 0 {
		return
	}
	s := r.slot(j)
	st, p := r.symsOf(s)
	for i := range slotSymbols {
		st.have[p+i] = noRun
	}
	lo := symRef{slot: j, k: cachSymbols}
	hi := symRef{slot: j, k: slotSymbols - 1}
	margin := r.model.d + runMargin
	if s.anchor {
		a := symRef{slot: j, k: syncFirstSym}
		r.run(a, hi.diff(a)+1+margin, false, lo, hi)
		b := symRef{slot: j, k: syncFirstSym + syncSymbols - 1}
		r.run(b, b.diff(lo)+1+margin, true, lo, hi)
	} else {
		r.run(lo.add(-margin), hi.diff(lo)+1+2*margin, false, lo, hi)
	}
	if r.opt.TrackCarrier && !r.refining && !r.warm && !r.frozen {
		r.trackCarrier(lo, hi)
	}
}

func (r *engine) trackCarrier(lo, hi symRef) {
	n := hi.diff(lo) + 1
	if n < 3*ddWindow {
		return
	}
	n = min(n, r.maxRun)
	sps, dec := r.search.sps, r.search.dec
	p := &r.pulse
	hpi := float32(r.h * math.Pi)
	nq, q32 := p.n, &pulseQ32[p.row]
	ch := &r.chz.ch
	var z, prev complex64
	var done float32
	doneK := 0
	var decs [ddWindow + 2*maxPulseSpan + 1]float32
	for w := 1; w+ddWindow <= n-1; w += ddWindow {
		if s := r.slot(lo.add(w).slot); !s.present || !r.slot(lo.add(w+ddWindow-1).slot).present {
			prev = 0
			continue
		}
		m0, m1 := max(w-p.span, 0), min(w+ddWindow-1+p.span, n-1)
		for m := m0; m <= m1; m++ {
			decs[m-m0] = hpi * float32(r.decision(lo.add(m)))
		}
		h0 := lo.add(w)
		tw := r.symTime(h0.slot, h0.k)
		rotW := complex64(r.rotAt(tw))
		wHz := float32(-2 * math.Pi * r.freq / basebandRate)
		base, uw := ch.ringAt(tw, sps)
		first := base - sincTaps
		scr := r.scr[:min(ddWindow*sps+sps+2*sincTaps+4, len(r.scr))]
		ch.gather(first, scr)
		uw += sincTaps
		du := float32(r.period / slotSymbols / float64(dec))
		dt := float32(r.period / slotSymbols)
		var c complex64
		for i := w; i < w+ddWindow; i++ {
			for ; doneK <= i-p.span-1; doneK++ {
				done += hpi * float32(r.decision(lo.add(doneK)))
			}
			ui := uw + float32(i-w)*du
			ti := float32(i-w) * dt
			for j := range sps {
				tt := i*sps + j - sps/2
				ph := done
				for m := max(i-p.span, 0); m <= min(i+p.span, n-1); m++ {
					switch idx := tt - m*sps + p.half; {
					case idx >= nq:
						ph += decs[m-m0]
					case idx >= 0:
						ph += decs[m-m0] * q32[idx+1]
					}
				}
				rs, rc := sincosf(wHz * (ti + float32((j-sps/2)*dec)))
				ps, pc := sincosf(ph)
				c += ch.interpIn(scr, ui+float32(j-sps/2)) * rotW * complex(rc, rs) * complex(pc, -ps)
			}
		}
		z += c * complex(real(prev), -imag(prev))
		prev = c
	}
	if z == 0 {
		return
	}
	hz := float64(atan2f(imag(z), real(z))) * basebandRate / (2 * math.Pi * ddWindow * samplesPerSy)
	r.passCarrier(lo, hi, r.freq+hz)
	g := 1 / (1 + r.fw)
	r.fw = math.Min(freqForget*r.fw+1, 1/(1-freqForget))
	r.freq += math.Max(g, 0.2) * hz
}

func (r *engine) decision(h symRef) int {
	st, p := r.symsOf(r.slot(h.slot))
	return 2*int(st.digit[p+h.k]) - 3
}
