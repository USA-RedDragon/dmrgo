package main

import (
	"math"
	"math/cmplx"
)

// Limits of the coherent detection between anchors.
const (
	// ddWindow is the window of the decision-directed carrier estimate, in
	// symbols.
	ddWindow = 12
	// runMargin is the number of symbols a trellis run goes past the last
	// symbol it detects, so that the APPs of that symbol see its whole
	// waveform.
	runMargin = 4
	// phaseBank is the number of carrier phases tried where no anchor gives
	// one.
	phaseBank = 8
	// noRun is the run distance of a symbol that no trellis run set.
	noRun = math.MaxInt16
	// maxPulseSpan is the most symbols on each side of a symbol that its
	// phase pulse reaches.
	maxPulseSpan = rrcSpan + 1
)

// symRef is a symbol of a stream: slot and symbol within the slot.
type symRef struct {
	slot int64
	k    int
}

// add returns the symbol n symbols after g.
func (g symRef) add(n int) symRef {
	x := int64(g.k) + int64(n)
	s := g.slot + floorDiv(x, slotSymbols)
	return symRef{slot: s, k: int(x - (s-g.slot)*slotSymbols)}
}

// floorDiv returns a/b rounded toward minus infinity.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// diff returns the number of symbols from h to g.
func (g symRef) diff(h symRef) int {
	return int((g.slot-h.slot)*slotSymbols) + g.k - h.k
}

// anchorSym returns the SYNC symbol of an anchor at stream symbol g.
func (r *Receiver) anchorSym(g symRef) (int, bool) {
	s := r.slot(g.slot)
	if s.index != g.slot || !s.anchor || g.k < syncFirstSym || g.k >= syncFirstSym+syncSymbols {
		return 0, false
	}
	var syms [syncSymbols]int8
	syncSyms(s.a.pattern, &syms)
	return int(syms[g.k-syncFirstSym]), true
}

// symPrior writes the log priors of the four digits of stream symbol g:
// Options.KnownPrior for the expected symbol of an anchor's SYNC, plus the bit
// priors that a refinement set on its slot.
func (r *Receiver) symPrior(g symRef, lp []float32) {
	clear(lp)
	if a, ok := r.anchorSym(g); ok {
		lp[(a+3)/2] = r.opt.KnownPrior
	}
	s := r.slot(g.slot)
	if s.index != g.slot || !s.hasPrior {
		return
	}
	for d := range 4 {
		lp[d] += r.prior[4*g.k+d]
	}
}

// fill aligns n symbols from g into r.buf, as fillRange.
func (r *Receiver) fill(g symRef, n int, rev bool) {
	r.fillRange(g, 0, n, rev, r.buf)
}

// fillRange aligns the symbols i0 to i1-1 from g into x, forward in time or
// backward (time reversed and conjugated, which is the waveform of the
// reversed symbols). The discrete phase of the model sums the frequency up
// to and including each sample, so the reversed waveform matches it one
// model sample earlier, and fillRange takes the reversed samples there.
func (r *Receiver) fillRange(g symRef, i0, i1 int, rev bool, x []complex64) {
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
	for i := i0; i < i1; i++ {
		h := g.add(dir * i)
		t := r.symTime(h.slot, h.k) - float64(sps/2)*sp
		if rev {
			t += sp
		}
		rot := symRot
		symRot *= symStep
		for j := range sps {
			v := ch.interp(t+float64(j)*sp) * rot
			rot *= step
			if rev {
				v = complex(real(v), -imag(v))
			}
			x[(i-i0)*sps+j] = v
		}
	}
}

// runSamples writes the samples of the intervals k0 to k1-1 of the current
// trellis run into x.
func (r *Receiver) runSamples(k0, k1 int, x []complex64) {
	d := r.model.d
	r.fillRange(r.runG, k0+d, k1+d, r.runDir < 0, x)
}

// runPrior writes the log priors of the newest symbol of interval k of the
// current trellis run.
func (r *Receiver) runPrior(k int, lp []float32) {
	r.symPrior(r.runG.add(r.runDir*(k+2*r.model.d)), lp)
}

// runKeep keeps the log-APPs of interval k of the current trellis run for
// its symbol when no run nearer that symbol set them.
func (r *Receiver) runKeep(k int, app []float32) {
	r.keepAPP(r.runG, r.runDir, k, r.runSteps, app, r.runLo, r.runHi)
}

// keepAPP keeps app, the log-APPs of interval k of a run of steps
// intervals from g in direction dir, for its symbol between lo and hi when
// no run nearer that symbol set them.
func (r *Receiver) keepAPP(g symRef, dir, k, steps int, app []float32, lo, hi symRef) {
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
	have := r.haveOf(s)
	if dist < 0 || dist >= int(have[h.k]) || dist > math.MaxInt16 {
		return
	}
	copy(r.appOf(s)[4*h.k:4*h.k+4], app)
	have[h.k] = int16(dist)
}

// run detects n symbols from g (forward or backward) with the trellis,
// starting from the known symbols at g when there are enough, and keeps the
// APPs of symbols nearer this start than the ones they have.
func (r *Receiver) run(g symRef, n int, rev bool, lo, hi symRef) {
	m := r.model
	d, sps := m.d, m.sps
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
	if r.xt == nil {
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
		r.xt.run(r.buf[d*sps:], steps, rot, scale, anchorKappa, r.tapp)
	} else {
		r.bankRun(steps, scale)
	}
	for k := range steps {
		r.keepAPP(g, dir, k, steps, r.tapp[4*k:4*k+4], lo, hi)
	}
}

// discard drops the log-APPs of a trellis run.
type discard struct{ *Receiver }

// runKeep drops app.
func (discard) runKeep(int, []float32) {}

// bankRun runs the trellis from an unknown state with phaseBank carrier
// phases and keeps the APPs of the run whose best path scores highest. The
// exact-phase trellis takes an unknown phase in one run.
func (r *Receiver) bankRun(steps int, scale float32) {
	sps, d := r.model.sps, r.model.d
	if r.xt != nil {
		r.xt.run(r.buf[d*sps:], steps, 1, scale, 0, r.tapp)
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

// coherentGap detects the symbols of slots first to j: forward from the
// anchor of slot first, if any, and backward from the anchor of slot j, if
// any, or from the first slot with phaseBank carrier phases when neither
// has one.
func (r *Receiver) coherentGap(first, j int64, final bool) {
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
		r.haveOf(r.slot(g.slot))[g.k] = noRun
	}
	margin := r.model.d + runMargin
	if left {
		lo = symRef{slot: first, k: syncFirstSym}
		r.run(lo, hi.diff(lo)+1+margin, false, lo, hi)
	}
	if right {
		r.run(hi, hi.diff(lo)+1+margin, true, lo, hi)
	}
	if !left && !right {
		r.run(lo, hi.diff(lo)+1+margin, false, lo, hi)
	}
	if r.opt.TrackCarrier {
		r.trackCarrier(lo, hi)
	}
}

// coherentBurst detects the burst of slot j alone, for a stream whose
// carrier is off between bursts: forward and backward from its SYNC, or
// with phaseBank carrier phases when it has none.
func (r *Receiver) coherentBurst(j int64) {
	if r.amp == 0 || r.noise == 0 {
		return
	}
	s := r.slot(j)
	for i := range r.haveOf(s) {
		r.haveOf(s)[i] = noRun
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
	if r.opt.TrackCarrier {
		r.trackCarrier(lo, hi)
	}
}

// trackCarrier updates the carrier estimate from the phase drift of the
// received symbols of lo to hi against the waveform of their decisions,
// over windows of ddWindow symbols. The waveform phase of a sample sums the
// phase pulse of the decisions within the span of the pulse and the whole
// phase steps of the decisions before it, as refPhaseInto does for the
// symbols from lo.
func (r *Receiver) trackCarrier(lo, hi symRef) {
	n := hi.diff(lo) + 1
	if n < 3*ddWindow {
		return
	}
	n = min(n, r.maxRun)
	sps, dec := r.search.sps, r.search.dec
	p := &r.pulse
	hpi := float32(r.h * math.Pi)
	nq := len(p.q) - 1
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
		var c complex64
		for i := w; i < w+ddWindow; i++ {
			for ; doneK <= i-p.span-1; doneK++ {
				done += hpi * float32(r.decision(lo.add(doneK)))
			}
			h := lo.add(i)
			t := r.symTime(h.slot, h.k)
			for j := range sps {
				ts := t + float64((j-sps/2)*dec)
				tt := i*sps + j - sps/2
				ph := done
				for m := max(i-p.span, 0); m <= min(i+p.span, n-1); m++ {
					switch idx := tt - m*sps + p.half; {
					case idx >= nq:
						ph += decs[m-m0]
					case idx >= 0:
						ph += decs[m-m0] * p.q32[idx+1]
					}
				}
				rs, rc := sincosf(wHz * float32(ts-tw))
				ps, pc := sincosf(ph)
				c += ch.interp(ts) * rotW * complex(rc, rs) * complex(pc, -ps)
			}
		}
		z += c * complex(real(prev), -imag(prev))
		prev = c
	}
	if z == 0 {
		return
	}
	hz := float64(atan2f(imag(z), real(z))) * basebandRate / (2 * math.Pi * ddWindow * samplesPerSy)
	g := 1 / (1 + r.fw)
	r.fw = math.Min(freqForget*r.fw+1, 1/(1-freqForget))
	r.freq += math.Max(g, 0.2) * hz
}

// decision returns the symbol, -3 to 3, with the highest log-APP of stream
// symbol h.
func (r *Receiver) decision(h symRef) int {
	a := r.appOf(r.slot(h.slot))[4*h.k : 4*h.k+4]
	best := 0
	for v := 1; v < 4; v++ {
		if a[v] > a[best] {
			best = v
		}
	}
	return 2*best - 3
}
