package main

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

// Layout of the coherent SYNC search.
const (
	// syncSymbols is the number of SYNC symbols.
	syncSymbols = framer.SyncBits / 2
	// syncEdge is the number of SYNC symbols at each end that the search
	// leaves out, as the unknown symbols next to them change their waveform.
	syncEdge = 2
	// syncSpan is the number of samples that the coherent search correlates.
	syncSpan = (syncSymbols - 2*syncEdge) * samplesPerSy
	// numSources is the number of SYNC sources the receiver knows.
	numSources = 4
	// searchFreqs is the number of carrier offsets, searchStepHz apart
	// around the estimate of the spectrum, at which the search correlates.
	searchFreqs  = 3
	searchStepHz = 200.0
)

// searchHzAt returns search offset i.
func searchHzAt(i int) float64 { return float64(i-searchFreqs/2) * searchStepHz }

// sources lists the SYNC sources in the order of the templates.
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

// syncSyms returns the 24 symbols of the SYNC pattern p.
func syncSyms(p enums.SyncPattern, out *[syncSymbols]int8) {
	var b [framer.SyncBits]bit.Bit
	bit.PutUint(b[:], uint64(p))
	for k := range syncSymbols {
		out[k] = symbolOf(b[2*k], b[2*k+1])
	}
}

// symbolOf returns the symbol of a dibit (ETSI TS 102 361-1 Table 10.3).
func symbolOf(b0, b1 bit.Bit) int8 {
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

// syncSearch finds SYNC patterns in the channel ring by coherent correlation
// with the waveform of the voice SYNC of each source over syncBlocks blocks,
// combined without phase. The data SYNC of a source has the negated symbols
// of its voice SYNC, so its waveform is the conjugate and the same products
// give both correlations.
type syncSearch struct {
	voice [numSources][syncSymbols]int8
	// dev holds the voice SYNC symbols of each source less their mean, and
	// devSq the sum of their squares, for the discriminator correlation.
	dev   [numSources][syncSymbols]float32
	devSq [numSources]float32
	tpl   [numSources][]complex64
	rot   [searchFreqs][]complex64
	x     []complex64
	full  []float64
	// pats and phases hold the waveform phase of every SYNC pattern over the
	// correlated span.
	pats   [2*numSources + 1]enums.SyncPattern
	phases [2*numSources + 1][]float64
	// conj holds e^-j*phase of each pattern over the correlated span.
	conj [2*numSources + 1][]complex128
}

// newSyncSearch returns a search with the symbols of the voice SYNC of each
// source, and when coherent the waveforms of every SYNC pattern at the
// nominal modulation index.
func newSyncSearch(coherent bool) *syncSearch {
	s := &syncSearch{}
	for i := range numSources {
		v, _, _ := sourceAt(i).SyncPatterns()
		syncSyms(v, &s.voice[i])
		var mp float32
		for _, a := range s.voice[i] {
			mp += float32(a)
		}
		mp /= syncSymbols
		for k, a := range s.voice[i] {
			s.dev[i][k] = float32(a) - mp
			s.devSq[i] += s.dev[i][k] * s.dev[i][k]
		}
	}
	if !coherent {
		return s
	}
	s.x, s.full = make([]complex64, syncSpan), make([]float64, syncSymbols*samplesPerSy)
	for i := range searchFreqs {
		f := searchHzAt(i)
		s.rot[i] = make([]complex64, syncSpan)
		for j := range syncSpan {
			p := -2 * math.Pi * f * float64(j) / basebandRate
			s.rot[i][j] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
		}
	}
	for i := range numSources {
		v, _, _ := sourceAt(i).SyncPatterns()
		syms := make([]int, syncSymbols)
		for k, a := range s.voice[i] {
			syms[k] = int(a)
		}
		ph := refPhase(syms, samplesPerSy, 0.27)
		_, d, _ := sourceAt(i).SyncPatterns()
		s.pats[2*i], s.pats[2*i+1] = v, d
		s.phases[2*i] = ph[syncEdge*samplesPerSy : syncEdge*samplesPerSy+syncSpan]
		s.phases[2*i+1] = make([]float64, syncSpan)
		for j, x := range s.phases[2*i] {
			s.phases[2*i+1][j] = -x
		}
		s.tpl[i] = make([]complex64, syncSpan)
		for j := range syncSpan {
			p := ph[syncEdge*samplesPerSy+j]
			s.tpl[i][j] = complex(float32(math.Cos(p)), float32(-math.Sin(p)))
		}
	}
	var rc [syncSymbols]int8
	syncSyms(enums.SyncMSSourcedRC, &rc)
	syms := make([]int, syncSymbols)
	for k, a := range rc {
		syms[k] = int(a)
	}
	s.pats[2*numSources] = enums.SyncMSSourcedRC
	s.phases[2*numSources] = refPhase(syms, samplesPerSy, 0.27)[syncEdge*samplesPerSy : syncEdge*samplesPerSy+syncSpan]
	s.setConj()
	return s
}

// setH recomputes the waveform phases of the SYNC patterns for modulation
// index h with the phase pulse p.
func (s *syncSearch) setH(h float64, p *phasePulse) {
	var syms [syncSymbols]int8
	var in [syncSymbols]int
	for i, q := range s.pats {
		syncSyms(q, &syms)
		for k, a := range syms {
			in[k] = int(a)
		}
		p.refPhaseInto(in[:], samplesPerSy, h, s.full)
		copy(s.phases[i], s.full[syncEdge*samplesPerSy:syncEdge*samplesPerSy+syncSpan])
	}
	s.setConj()
}

// setConj computes conj from phases.
func (s *syncSearch) setConj() {
	for i, ph := range s.phases {
		if s.conj[i] == nil {
			s.conj[i] = make([]complex128, syncSpan)
		}
		for j, p := range ph {
			s.conj[i][j] = complex(math.Cos(p), -math.Sin(p))
		}
	}
}

// conjOf returns e^-j*phase of SYNC pattern p over the correlated span, or
// nil for an unknown pattern.
func (s *syncSearch) conjOf(p enums.SyncPattern) []complex128 {
	for i, q := range s.pats {
		if q == p {
			return s.conj[i]
		}
	}
	return nil
}

// syncHit is one coherent SYNC correlation.
type syncHit struct {
	// Metric is the squared normalized correlation, 0 to 1.
	Metric float64
	// Data is true for the data SYNC, false for the voice SYNC.
	Data bool
	// Source indexes sourceAt.
	Source int
	// Hz is the carrier offset that the phases of the blocks show.
	Hz float64
}

// at correlates the samples from start, the center of the first correlated
// SYNC symbol, with every template of nsrc sources at each search
// frequency.
func (s *syncSearch) at(r *ring, start int64, nsrc int) syncHit {
	var best syncHit
	var etot float32
	for j := range syncSpan {
		v := r.at(start - samplesPerSy/2 + int64(j))
		s.x[j] = v
		etot += real(v)*real(v) + imag(v)*imag(v)
	}
	if etot == 0 {
		return best
	}
	for fi, rot := range s.rot {
		for src := range nsrc {
			t := s.tpl[src]
			var ac, bd, bc, ad float32
			for j, v := range s.x {
				w := t[j] * rot[j]
				ac += real(v) * real(w)
				bd += imag(v) * imag(w)
				bc += imag(v) * real(w)
				ad += real(v) * imag(w)
			}
			for k, c := range [2]complex64{complex(ac-bd, bc+ad), complex(ac+bd, bc-ad)} {
				metric := float64((real(c)*real(c) + imag(c)*imag(c)) / (etot * syncSpan))
				if metric > best.Metric {
					hz := searchHzAt(fi)
					if k == 1 {
						hz = -hz
					}
					best = syncHit{Metric: metric, Data: k == 1, Source: src, Hz: hz}
				}
			}
		}
	}
	return best
}

// discAt returns the correlation coefficient of the discriminator symbols
// at t, t+step, ... with the voice SYNC of each source, the best source and
// whether its sign shows the data SYNC.
func (s *syncSearch) discAt(r *qring, t, step float64, nsrc int) (rho float32, src int, data bool) {
	base := int64(math.Floor(t))
	u, du := float32(t-float64(base)), float32(step)
	var x [syncSymbols]float32
	var mx float32
	for k := range syncSymbols {
		x[k] = r.lerpFrom(base, u+float32(k)*du)
		mx += x[k]
	}
	mx /= syncSymbols
	var sxx float32
	for k := range x {
		x[k] -= mx
		sxx += x[k] * x[k]
	}
	if sxx == 0 {
		return 0, 0, false
	}
	var best, bestSxy float32
	for i := range nsrc {
		var sxy float32
		for k, p := range s.dev[i] {
			sxy += p * x[k]
		}
		if q := sxy * sxy / s.devSq[i]; q > best {
			best, bestSxy, src = q, sxy, i
		}
	}
	return sqrtf(best / sxx), src, bestSxy < 0
}
