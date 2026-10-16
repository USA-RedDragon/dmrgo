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

// fill aligns n symbols from g into r.buf, forward in time or backward (time
// reversed and conjugated, which is the waveform of the reversed symbols).
// The discrete phase of the model sums the frequency up to and including
// each sample, so the reversed waveform matches it one model sample
// earlier, and fill takes the reversed samples there.
func (r *Receiver) fill(g symRef, n int, rev bool) {
	ch := &r.chz.ch
	dir := 1
	if rev {
		dir = -1
	}
	sps := r.model.sps
	sp := float64(dir * samplesPerSy / sps)
	step := complex64(r.rotAt(sp))
	for i := range n {
		h := g.add(dir * i)
		t := r.symTime(h.slot, h.k) - float64(sps/2)*sp
		if rev {
			t += sp
		}
		rot := complex64(r.rotAt(t))
		for j := range sps {
			v := ch.interp(t+float64(j)*sp) * rot
			rot *= step
			if rev {
				v = complex(real(v), -imag(v))
			}
			r.buf[i*sps+j] = v
		}
	}
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
	r.fill(g, n, rev)
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
	for k := range steps {
		r.symPrior(g.add(dir*(k+2*d)), r.lp[4*k:4*k+4])
	}
	for i := range 2 * d {
		r.symPrior(g.add(dir*i), r.slp[4*i:4*i+4])
	}
	scale := float32(2 * r.amp / r.noise)
	if nk >= 2*d+2 {
		rot, _, _ := m.anchor(r.buf, syms[:nk])
		r.detect(r.buf[d*sps:], steps, rot, scale, r.opt.PSPGain, r.tapp)
	} else {
		r.bankRun(steps, scale)
	}
	for k := range steps - d {
		h := g.add(dir * (k + 2*d))
		if h.diff(lo) < 0 || h.diff(hi) > 0 {
			continue
		}
		s := r.slot(h.slot)
		if s.index != h.slot {
			continue
		}
		dist := k + 2*d
		have := r.haveOf(s)
		if dist < 0 || dist >= int(have[h.k]) || dist > math.MaxInt16 {
			continue
		}
		copy(r.appOf(s)[4*h.k:4*h.k+4], r.tapp[4*k:4*k+4])
		have[h.k] = int16(dist)
	}
}

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
		r.detect(r.buf[d*sps:], steps, rot, scale, r.opt.PSPGain, r.tapp)
		if v := r.detBest(steps); v > best {
			best, bestP = v, p
		}
	}
	rot := complex64(cmplx.Exp(complex(0, -2*math.Pi*float64(bestP)/phaseBank)))
	r.detect(r.buf[d*sps:], steps, rot, scale, r.opt.PSPGain, r.tapp)
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
// over windows of ddWindow symbols.
func (r *Receiver) trackCarrier(lo, hi symRef) {
	n := hi.diff(lo) + 1
	if n < 3*ddWindow {
		return
	}
	n = min(n, r.maxRun)
	syms := r.ddSyms[:n]
	for i := range n {
		h := lo.add(i)
		s := r.slot(h.slot)
		a := r.appOf(s)[4*h.k : 4*h.k+4]
		best := 0
		for v := 1; v < 4; v++ {
			if a[v] > a[best] {
				best = v
			}
		}
		syms[i] = 2*best - 3
	}
	r.pulse.refPhaseInto(syms, samplesPerSy, r.h, r.ddPh[:n*samplesPerSy])
	ch := &r.chz.ch
	var z complex128
	var prev complex128
	for w := 1; w+ddWindow <= n-1; w += ddWindow {
		if s := r.slot(lo.add(w).slot); !s.present || !r.slot(lo.add(w+ddWindow-1).slot).present {
			prev = 0
			continue
		}
		var c complex128
		for i := w; i < w+ddWindow; i++ {
			h := lo.add(i)
			t := r.symTime(h.slot, h.k)
			for j := range samplesPerSy {
				ts := t + float64(j-samplesPerSy/2)
				p := r.ddPh[i*samplesPerSy+j]
				c += complex128(ch.interp(ts)) * r.rotAt(ts) * complex(math.Cos(p), -math.Sin(p))
			}
		}
		z += c * cmplx.Conj(prev)
		prev = c
	}
	if z == 0 {
		return
	}
	hz := cmplx.Phase(z) * basebandRate / (2 * math.Pi * ddWindow * samplesPerSy)
	g := 1 / (1 + r.fw)
	r.fw = math.Min(freqForget*r.fw+1, 1/(1-freqForget))
	r.freq += math.Max(g, 0.2) * hz
}
