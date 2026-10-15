package main

import (
	"math"
	"sort"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

// Received is one burst recovered from I/Q.
type Received struct {
	Data    [33]byte
	Seconds float64
	// Distance is the Hamming distance of the SYNC field from Sync, or -1
	// for a burst placed by timing without a SYNC (voice bursts B to F).
	Distance int
	Sync     enums.SyncPattern
	// CACH is the 24 bits before the burst in transmit order. BS is set
	// when the burst is BS-sourced, so that those bits are a CACH.
	CACH    [layer2.CACHBits]bit.Bit
	HasCACH bool
	BS      bool
}

// Timed reports whether the burst was placed by timing instead of a SYNC.
func (r Received) Timed() bool { return r.Distance < 0 }

// Demodulated is the result of Demodulate.
type Demodulated struct {
	Bursts      []Received
	OffsetHz    float64
	DeviationHz float64
}

func sampleOf(r Received) int { return int(math.Round(r.Seconds * basebandRate)) }

// Demodulate recovers DMR bursts from complex samples at rate. Bursts with a
// SYNC pattern are found by search. Voice bursts B to F carry embedded
// signalling instead, so they are cut at 60 ms steps after their burst A.
// The step is measured from the next burst of the same channel when there is
// one, so a slow or fast symbol clock is followed.
func Demodulate(x []complex128, rate float64) Demodulated {
	bb := channelize(x, rate)
	pw := power(bb)
	var peak float64
	for _, v := range pw {
		peak = math.Max(peak, v)
	}
	present := func(t float64) bool {
		lo := int(t * basebandRate)
		hi := min(lo+int(burstSeconds*basebandRate), len(pw))
		if lo < 0 || hi <= lo {
			return false
		}
		var s float64
		for _, v := range pw[lo:hi] {
			s += v
		}
		return s/float64(hi-lo) > peak/4
	}
	coarse := carrierOffset(bb)
	bb = lowpass(shift(bb, coarse), 6000)
	d := discriminate(bb)
	sym := filter(d, rrcTaps())

	phaseBits := make([][]bit.Bit, samplesPerSy)
	for phase := range samplesPerSy {
		for i := phase; i < len(sym); i += samplesPerSy {
			b0, b1 := dibitFromSymbol(sym[i])
			phaseBits[phase] = append(phaseBits[phase], b0, b1)
		}
	}
	// cut returns the burst whose first symbol is at sample s.
	cut := func(s int) (Received, bool) {
		if s < 0 {
			return Received{}, false
		}
		bits := phaseBits[s%samplesPerSy]
		first := 2 * (s / samplesPerSy)
		if first+layer2.BurstBits > len(bits) {
			return Received{}, false
		}
		var b [264]bit.Bit
		copy(b[:], bits[first:first+layer2.BurstBits])
		r := Received{Data: bit.Pack264(b), Seconds: float64(s) / basebandRate}
		if first >= layer2.CACHBits {
			copy(r.CACH[:], bits[first-layer2.CACHBits:first])
			r.HasCACH = true
		}
		return r, true
	}

	// eye returns the mean distance of the burst's symbols at sample s from
	// the nearest decision threshold, which peaks at the best sampling time.
	eye := func(s int) float64 {
		var m float64
		for k := range burstSymbols {
			i := s + k*samplesPerSy
			if i < 0 || i >= len(sym) {
				return 0
			}
			v := math.Abs(sym[i])
			m += math.Min(v, math.Abs(v-2*deviationHz))
		}
		return m / burstSymbols
	}

	var all []Received
	for phase := range samplesPerSy {
		bits := phaseBits[phase]
		for start := 0; ; {
			m, ok := framer.Search(bits, start, -1)
			if !ok {
				break
			}
			start = m.Offset + 1
			first := m.Offset - framer.SyncOffset
			if first < 0 || first%2 != 0 {
				continue
			}
			r, ok := cut(phase + first/2*samplesPerSy)
			if !ok || !present(r.Seconds) {
				continue
			}
			src, _, known := layer2.SyncSourceOf(m.Pattern)
			r.Distance, r.Sync, r.BS = m.Distance, m.Pattern, known && src == layer2.SyncSourceBS
			all = append(all, r)
		}
	}

	res := Demodulated{Bursts: pickBursts(all, eye)}
	res.OffsetHz, res.DeviationHz = measure(res.Bursts, sym)
	res.OffsetHz += coarse
	res.Bursts = append(res.Bursts, embeddedVoice(res.Bursts, cut, present, eye)...)
	sort.Slice(res.Bursts, func(i, j int) bool { return res.Bursts[i].Seconds < res.Bursts[j].Seconds })
	return res
}

// pickBursts keeps one candidate per burst, in time order: of the sampling
// phases with the least SYNC distance, the one with the widest eye.
func pickBursts(all []Received, eye func(int) float64) []Received {
	sort.SliceStable(all, func(i, j int) bool { return all[i].Distance < all[j].Distance })
	var out []Received
	used := make([]bool, len(all))
	for i := range all {
		if used[i] {
			continue
		}
		pick := all[i]
		for j := i + 1; j < len(all); j++ {
			if !used[j] && math.Abs(all[j].Seconds-all[i].Seconds) < burstSeconds {
				used[j] = true
				if all[j].Distance == all[i].Distance && eye(sampleOf(all[j])) > eye(sampleOf(pick)) {
					pick = all[j]
				}
			}
		}
		out = append(out, pick)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seconds < out[j].Seconds })
	return out
}

// measure returns the residual carrier offset and the outer deviation over
// the SYNC fields of bursts.
func measure(bursts []Received, sym []float64) (offset, deviation float64) {
	var resid, outer float64
	var nr, no int
	for _, r := range bursts {
		ideal := symbolsFromBurst(r.Data)
		first := sampleOf(r)
		for k := 54; k < 78; k++ {
			idx := first + k*samplesPerSy
			if idx >= len(sym) {
				break
			}
			resid += sym[idx] - float64(ideal[k])*deviationHz
			nr++
			if ideal[k] == 3 || ideal[k] == -3 {
				outer += math.Abs(sym[idx])
				no++
			}
		}
	}
	if nr > 0 {
		offset = resid / float64(nr)
	}
	if no > 0 {
		deviation = outer / float64(no)
	}
	return offset, deviation
}

// embeddedVoice cuts voice bursts B to F after each voice burst A in found.
// Each is cut at the sample within half a symbol of the expected time that
// has the widest eye.
func embeddedVoice(found []Received, cut func(int) (Received, bool), present func(float64) bool, eye func(int) float64) []Received {
	sample := func(r Received) float64 { return float64(sampleOf(r)) }
	occupied := func(s float64) bool {
		for _, r := range found {
			if math.Abs(sample(r)-s) < burstSeconds/2*basebandRate {
				return true
			}
		}
		return false
	}
	var out []Received
	for i, a := range found {
		if _, k, _ := layer2.SyncSourceOf(a.Sync); k != layer2.KindVoice {
			continue
		}
		sa := sample(a)
		step := float64(frameSamples)
		for _, b := range found[i+1:] {
			dt := sample(b) - sa
			k := math.Round(dt / frameSamples)
			if k >= 1 && k <= layer2.VoiceBurstsPerSuperframe && math.Abs(dt-k*frameSamples) < 3*samplesPerSy {
				step = dt / k
				break
			}
		}
		for k := 1; k < layer2.VoiceBurstsPerSuperframe; k++ {
			s := sa + float64(k)*step
			if occupied(s) {
				break
			}
			at := int(math.Round(s))
			for d := -samplesPerSy / 2; d <= samplesPerSy/2; d++ {
				if eye(int(math.Round(s))+d) > eye(at) {
					at = int(math.Round(s)) + d
				}
			}
			r, ok := cut(at)
			if !ok || !present(r.Seconds) {
				break
			}
			r.Distance, r.Sync, r.BS = -1, enums.SyncNone, a.BS
			out = append(out, r)
		}
	}
	return out
}
