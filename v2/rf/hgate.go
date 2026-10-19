package rf

import "slices"

// Grid and bootstrap of options.HGate.
const (
	// hGateLo and hGateStep set the modulation indexes that options.HGate
	// scores: hGateN of them from hGateLo.
	hGateLo   = 0.21
	hGateStep = 0.0025
	hGateN    = 49
	// hGateBoot is the number of bootstrap resamples of the anchors.
	hGateBoot = 400
	// hGateMin is the fewest anchors of an estimate by default.
	hGateMin = 4
)

// hGate holds the SYNC scores of each anchor at each modulation index of
// the grid, and the bootstrap estimates.
type hGate struct {
	sc   []float32
	tot  [hGateN]float64
	est  []float64
	cnt  []uint16
	rng  uint64
	nmax int
	// keep holds the SYNC waveforms of the search while gateH scores.
	keep []complex64
}

// newHGate returns the buffers for up to slots anchors and the SYNC
// waveforms of search s.
func newHGate(slots int, s *syncSearch) *hGate {
	return &hGate{
		sc: make([]float32, slots*hGateN), est: make([]float64, hGateBoot), cnt: make([]uint16, slots),
		nmax: slots, keep: make([]complex64, len(s.conj)*s.span),
	}
}

// best returns the grid index of the largest total of the scores of the n
// anchors, each weighted by its count in w, or by 1 when w is nil.
func (g *hGate) best(n int, w []uint16) int {
	clear(g.tot[:])
	for i := range n {
		f := 1.0
		if w != nil {
			if w[i] == 0 {
				continue
			}
			f = float64(w[i])
		}
		row := g.sc[i*hGateN : (i+1)*hGateN]
		for k, v := range row {
			g.tot[k] += f * float64(v)
		}
	}
	b := 0
	for k, v := range g.tot {
		if v > g.tot[b] {
			b = k
		}
	}
	return b
}

// gateH estimates the modulation index from the SYNC fields of the anchors
// of the first pass from slot lo to hi, at the timing fit and the carrier
// of the second pass, and sets it when the bootstrap interval of the
// estimate is more than options.HGate away from 0.27.
func (r *engine) gateH(lo, hi int64) {
	g := r.hg
	saved := r.freq
	for i, c := range &r.search.conj {
		copy(g.keep[i*r.search.span:], c)
	}
	n := 0
	for j := lo; j < hi && n < g.nmax; j++ {
		if s := r.slot(j); s.index == j && s.anchor && r.pass(j).anchor {
			r.freq = r.pass(j).smHz
			e := r.syncSamples(s, 0)
			for k := range hGateN {
				r.search.setH(hGateLo+float64(k)*hGateStep, &r.pulse)
				g.sc[n*hGateN+k] = float32(r.syncCorr(s.a.pattern, e).metric)
			}
			n++
		}
	}
	r.freq = saved
	for i, c := range &r.search.conj {
		copy(c, g.keep[i*r.search.span:])
	}
	need := r.opt.HGateMin
	if need == 0 {
		need = hGateMin
	}
	if n < need {
		return
	}
	est, ciLo, ciHi := g.interval(n)
	if ciHi >= cohIndex-r.opt.HGate && ciLo <= cohIndex+r.opt.HGate {
		return
	}
	r.setH(est)
}

// interval returns the modulation index of the best total score of the n
// anchors, and the 95% interval of the same estimate over hGateBoot
// resamples of the anchors.
func (g *hGate) interval(n int) (est, lo, hi float64) {
	g.rng = 0x9e3779b97f4a7c15
	for b := range g.est {
		clear(g.cnt[:n])
		for range n {
			g.rng ^= g.rng << 13
			g.rng ^= g.rng >> 7
			g.rng ^= g.rng << 17
			g.cnt[g.rng%uint64(n)]++
		}
		g.est[b] = hGateLo + float64(g.best(n, g.cnt))*hGateStep
	}
	slices.Sort(g.est)
	k := len(g.est) / 40
	return hGateLo + float64(g.best(n, nil))*hGateStep, g.est[k], g.est[len(g.est)-1-k]
}
