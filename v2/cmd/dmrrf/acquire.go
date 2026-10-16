package main

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
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
	// syncSpan is the number of channel samples that the coherent search
	// correlates.
	syncSpan = (syncSymbols - 2*syncEdge) * samplesPerSy
	// numSources is the number of SYNC sources the receiver knows.
	numSources = 4
	// searchFreqs is the number of carrier offsets, searchStepHz apart
	// around the estimate of the spectrum, at which the search correlates.
	searchFreqs  = 3
	searchStepHz = 200.0
	// preBlocks is the number of blocks that the screen of the coherent
	// search combines without phase, and preMetric the screen metric under
	// which the search skips a position.
	preBlocks = 4
	preMetric = 0.25
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
	unpacked.PutUint(b[:], uint64(p))
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
	// dec is the number of channel samples per sample of the channel ring,
	// sps the ring samples per symbol and span the ring samples that the
	// search correlates.
	dec, sps, span int
	voice          [numSources][syncSymbols]int8
	// dev holds the voice SYNC symbols of each source less their mean, and
	// devSq the sum of their squares, for the discriminator correlation.
	dev   [numSources][syncSymbols]float32
	devSq [numSources]float32
	// dx holds the discriminator symbols that discAt correlates.
	dx [syncSymbols]float32
	// tplRot holds the conjugate waveform of the voice SYNC of source src
	// turned by search offset fi at tplRot[fi*numSources+src], for the
	// sources that the receiver searches.
	tplRot [searchFreqs * numSources][]complex64
	// xs holds the window of the last search twice, from xs[xh], and
	// xFirst its first ring sample, when xOK.
	xs     []complex64
	xh     int
	xFirst int64
	xOK    bool
	full   []float64
	// pats holds every SYNC pattern, and conj e^-j*phase of the waveform
	// of each over the correlated span.
	pats [2*numSources + 1]enums.SyncPattern
	conj [2*numSources + 1][]complex64
}

// newSyncSearch returns a search with the symbols of the voice SYNC of each
// source, and when coherent the waveforms of every SYNC pattern at the
// nominal modulation index, sampled at every dec channel samples, and the
// search templates of nsrc sources.
func newSyncSearch(coherent bool, dec, nsrc int) *syncSearch {
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
	s.dec, s.sps, s.span = dec, samplesPerSy/dec, syncSpan/dec
	sps, span := s.sps, s.span
	s.full = make([]float64, syncSymbols*sps)
	s.xs = make([]complex64, 2*span)
	var rot [searchFreqs][]complex64
	for i := range searchFreqs {
		f := searchHzAt(i)
		rot[i] = make([]complex64, span)
		for j := range span {
			p := -2 * math.Pi * f * float64(j*dec) / basebandRate
			rot[i][j] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
		}
	}
	tpl := make([]complex64, span)
	for i := range numSources {
		v, _, _ := sourceAt(i).SyncPatterns()
		_, d, _ := sourceAt(i).SyncPatterns()
		s.pats[2*i], s.pats[2*i+1] = v, d
		if i >= nsrc {
			continue
		}
		syms := make([]int, syncSymbols)
		for k, a := range s.voice[i] {
			syms[k] = int(a)
		}
		ph := refPhase(syms, sps, 0.27)
		for j := range span {
			p := ph[syncEdge*sps+j]
			tpl[j] = complex(float32(math.Cos(p)), float32(-math.Sin(p)))
		}
		for fi := range searchFreqs {
			t := make([]complex64, span)
			for j := range t {
				t[j] = tpl[j] * rot[fi][j]
			}
			s.tplRot[fi*numSources+i] = t
		}
	}
	s.pats[2*numSources] = enums.SyncMSSourcedRC
	for i := range s.conj {
		s.conj[i] = make([]complex64, span)
	}
	pulse := newPhasePulse(sps)
	s.setH(0.27, &pulse)
	return s
}

// setH recomputes the waveforms of the SYNC patterns for modulation index h
// with the phase pulse p.
func (s *syncSearch) setH(h float64, p *phasePulse) {
	var syms [syncSymbols]int8
	var in [syncSymbols]int
	for i, q := range s.pats {
		syncSyms(q, &syms)
		for k, a := range syms {
			in[k] = int(a)
		}
		p.refPhaseInto(in[:], s.sps, h, s.full)
		for j, ph := range s.full[syncEdge*s.sps : syncEdge*s.sps+s.span] {
			s.conj[i][j] = complex(float32(math.Cos(ph)), float32(-math.Sin(ph)))
		}
	}
}

// conjOf returns e^-j*phase of SYNC pattern p over the correlated span, or
// nil for an unknown pattern.
func (s *syncSearch) conjOf(p enums.SyncPattern) []complex64 {
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
	Metric float32
	// Data is true for the data SYNC, false for the voice SYNC.
	Data bool
	// Source indexes sourceAt.
	Source int
	// Hz is the carrier offset that the phases of the blocks show.
	Hz float64
}

// at correlates the samples from channel sample start, a multiple of dec
// at the center of the first correlated SYNC symbol, with every template of
// nsrc sources at each search frequency.
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
			t := s.tplRot[fi*numSources+src]
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

// screen returns the best metric over nsrc sources of the window x of
// energy etot at the middle search offset, its preBlocks blocks combined
// without phase. The combined metric is at least the coherent one at the
// same offset, and an offset turns a block less than the whole span. That
// does not bound it for every window, but on DMR signals from -3 to 40 dB
// C/N at offsets to 450 Hz no position whose coherent metric reached
// lockMetric at any search offset screened under 0.31.
func (s *syncSearch) screen(x []complex64, etot float32, nsrc int) float32 {
	n := len(x) / preBlocks
	var best float32
	for src := range nsrc {
		t := s.tplRot[searchFreqs/2*numSources+src]
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

// window returns the span ring samples from first. The search steps one
// ring sample at a time, so the window of the last search less its first
// sample, and the next ring sample, give the window of the next.
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

// discAt returns the square of the correlation coefficient of the
// discriminator symbols at base+u, base+u+step, ... with the voice SYNC of
// each source, the best source and whether its sign shows the data SYNC.
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
			sxy[i] += s.dev[i][k] * v
		}
	}
	sxx := xx - mx*mx/syncSymbols
	if sxx <= 0 {
		return 0, 0, false
	}
	var best float32
	for i := range nsrc {
		if q := sxy[i] * sxy[i] / s.devSq[i]; q > best {
			best, src = q, i
		}
	}
	return best / sxx, src, sxy[src] < 0
}
