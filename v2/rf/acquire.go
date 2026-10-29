package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

const (
	syncSymbols  = framer.SyncBits / 2
	syncEdge     = 2
	syncSpan     = (syncSymbols - 2*syncEdge) * samplesPerSy
	numSources   = 4
	searchFreqs  = 3
	searchStepHz = 200.0
	preBlocks    = 4
	preMetric    = 0.25
)

func searchHzAt(i int) float64 { return float64(i-searchFreqs/2) * searchStepHz }

func sourceAt(i int) layer2.SyncSource {
	switch i {
	case 0:
		return layer2.SyncSourceBS
	case 1:
		return layer2.SyncSourceMS
	case 2:
		return layer2.SyncSourceDirectTS1
	}
	return layer2.SyncSourceDirectTS2
}

func syncSyms(p enums.SyncPattern, out *[syncSymbols]int8) {
	v := uint64(p) << (64 - 2*syncSymbols)
	for k := range syncSymbols {
		d := uint8(v >> 62)
		v <<= 2
		out[k] = symbolOf(unpacked.Bit(d>>1), unpacked.Bit(d&1))
	}
}

// ETSI TS 102 361-1 Table 10.3
func symbolOf(b0, b1 unpacked.Bit) int8 {
	switch b0<<1 | b1 {
	case 0b01:
		return 3
	case 0b00:
		return 1
	case 0b10:
		return -1
	}
	return -3
}

type syncSearch struct {
	dec, sps, span int
	dx             [syncSymbols]float32
	xs             []complex64
	xh             int
	xFirst         int64
	xOK            bool
	full           []float64
	conj           [2*numSources + 1][]complex64
}

func newSyncSearch(coherent bool, dec int) *syncSearch {
	s := &syncSearch{}
	if !coherent {
		return s
	}
	s.dec, s.sps, s.span = dec, samplesPerSy/dec, syncSpan/dec
	s.full = make([]float64, syncSymbols*s.sps)
	s.xs = make([]complex64, 2*s.span)
	for i := range s.conj {
		s.conj[i] = make([]complex64, s.span)
	}
	pulse := newPhasePulse(s.sps)
	s.setH(0.27, &pulse)
	return s
}

func (s *syncSearch) setH(h float64, p *phasePulse) {
	for i := range syncPatterns {
		s.setPatternH(i, h, p)
	}
}

func (s *syncSearch) setPatternH(i int, h float64, p *phasePulse) {
	var syms [syncSymbols]int8
	var in [syncSymbols]int
	syncSyms(syncPatterns[i], &syms)
	for k, a := range syms {
		in[k] = int(a)
	}
	p.refPhaseInto(in[:], s.sps, h, s.full)
	for j, ph := range s.full[syncEdge*s.sps : syncEdge*s.sps+s.span] {
		s.conj[i][j] = complex(float32(math.Cos(ph)), float32(-math.Sin(ph)))
	}
}

func (s *syncSearch) conjOf(p enums.SyncPattern) []complex64 {
	for i, q := range &syncPatterns {
		if q == p {
			return s.conj[i]
		}
	}
	return nil
}

type syncHit struct {
	Metric float32
	Data   bool
	Source int
	Hz     float64
}

func (s *syncSearch) at(r *ring, start int64, nsrc int) syncHit {
	var best syncHit
	x := s.window(r, start/int64(s.dec)-int64(s.sps/2))
	var etot float32
	for _, v := range x {
		etot += real(v)*real(v) + imag(v)*imag(v)
	}
	if etot == 0 || s.screen(x, etot, nsrc) < preMetric {
		return best
	}
	bestFi := 0
	for fi := range searchFreqs {
		for src := range nsrc {
			t := &syncTemplates[s.dec-1][fi*numSources+src]
			var ac, bd, bc, ad float32
			for j, v := range x {
				w := t[j]
				ac += real(v) * real(w)
				bd += imag(v) * imag(w)
				bc += imag(v) * real(w)
				ad += real(v) * imag(w)
			}
			for k, c := range [2]complex64{complex(ac-bd, bc+ad), complex(ac+bd, bc-ad)} {
				if metric := (real(c)*real(c) + imag(c)*imag(c)) / (etot * float32(s.span)); metric > best.Metric {
					best, bestFi = syncHit{Metric: metric, Data: k == 1, Source: src}, fi
				}
			}
		}
	}
	best.Hz = searchHzAt(bestFi)
	if best.Data {
		best.Hz = -best.Hz
	}
	return best
}

func (s *syncSearch) screen(x []complex64, etot float32, nsrc int) float32 {
	n := len(x) / preBlocks
	var best float32
	for src := range nsrc {
		t := &syncTemplates[s.dec-1][searchFreqs/2*numSources+src]
		var mv, md float32
		for b := range preBlocks {
			var ac, bd, bc, ad float32
			for j := b * n; j < (b+1)*n; j++ {
				v, w := x[j], t[j]
				ac += real(v) * real(w)
				bd += imag(v) * imag(w)
				bc += imag(v) * real(w)
				ad += real(v) * imag(w)
			}
			mv += (ac-bd)*(ac-bd) + (bc+ad)*(bc+ad)
			md += (ac+bd)*(ac+bd) + (bc-ad)*(bc-ad)
		}
		best = max(best, mv, md)
	}
	return best / (etot * float32(n))
}

func (s *syncSearch) window(r *ring, first int64) []complex64 {
	n := s.span
	if s.xOK && first == s.xFirst+1 {
		v := r.at(first + int64(n) - 1)
		s.xs[s.xh], s.xs[s.xh+n] = v, v
		s.xh++
		if s.xh == n {
			s.xh = 0
		}
	} else {
		r.gather(first, s.xs[:n])
		copy(s.xs[n:], s.xs[:n])
		s.xh, s.xOK = 0, true
	}
	s.xFirst = first
	return s.xs[s.xh : s.xh+n]
}

func (s *syncSearch) discAt(r *qring, base int64, u, step float32, nsrc int) (rho2 float32, src int, data bool) {
	x := s.dx[:]
	if stride := int(step); u == 0 && float32(stride) == step {
		r.gather(base, stride, x)
	} else {
		for k := range x {
			x[k] = r.lerpFrom(base, u+float32(k)*step)
		}
	}
	var mx, xx float32
	var sxy [numSources]float32
	for k, v := range x {
		mx += v
		xx += v * v
		for i := range sxy {
			sxy[i] += syncDev[i][k] * v
		}
	}
	sxx := xx - mx*mx/syncSymbols
	if sxx <= 0 {
		return 0, 0, false
	}
	var best float32
	for i := range nsrc {
		if q := sxy[i] * sxy[i] / syncDevSq[i]; q > best {
			best, src = q, i
		}
	}
	return best / sxx, src, sxy[src] < 0
}
