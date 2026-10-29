package rf

import "slices"

const (
	hGateLo   = 0.21
	hGateStep = 0.0025
	hGateN    = 49
	hGateBoot = 400
	hGateMin  = 4
)

type hGate struct {
	sc   []float32
	tot  [hGateN]float64
	est  []float64
	cnt  []uint16
	rng  uint64
	nmax int
	keep []complex64
}

func newHGate(slots int, s *syncSearch) *hGate {
	return &hGate{
		sc: make([]float32, slots*hGateN), est: make([]float64, hGateBoot), cnt: make([]uint16, slots),
		nmax: slots, keep: make([]complex64, len(s.conj)*s.span),
	}
}

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
			pi := slices.Index(syncPatterns[:], s.a.pattern)
			for k := range hGateN {
				if pi < 0 {
					r.search.setH(hGateLo+float64(k)*hGateStep, &r.pulse)
				} else {
					r.search.setPatternH(pi, hGateLo+float64(k)*hGateStep, &r.pulse)
				}
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
