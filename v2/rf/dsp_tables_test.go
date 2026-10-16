package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// dspTables holds the filter, window, waveform and phase tables of
// tables_gen.go as deriveDSP derives them.
type dspTables struct {
	interp    [interpPhases + 1][sincTaps]float32
	chanTaps  [chanTaps]float32
	half      [halfTaps]float32
	pre       [preTaps]float32
	rrc       [rrcTaps]float32
	halfQ15   [halfSide + 1]int16
	preQ15    [preTaps/2 + 1]int16
	rrcQ15    [rrcTaps/2 + 1]int16
	twiddles  [fftSize / 2]complex64
	window    [fftSize/2 + 1]float32
	dev       [numSources][syncSymbols]float32
	devSq     [numSources]float32
	patterns  [2*numSources + 1]enums.SyncPattern
	templates [2][searchFreqs * numSources][syncSpan]complex64
	pulse     [2][2*rrcSpan*samplesPerSy + 2]float64
	pulse32   [2][2*rrcSpan*samplesPerSy + 2]float32
	cos, sin  [2][phaseStates]float32
}

// deriveDSP derives the tables of dspTables.
func deriveDSP() *dspTables {
	t := &dspTables{}
	for p := range interpPhases + 1 {
		fr := float64(p) / interpPhases
		for k := range sincTaps {
			u := float64(k-sincTaps/2+1) - fr
			w := 0.42 + 0.5*math.Cos(math.Pi*u/(sincTaps/2)) + 0.08*math.Cos(2*math.Pi*u/(sincTaps/2))
			if u != 0 {
				w *= math.Sin(math.Pi*u) / (math.Pi * u)
			}
			t.interp[p][k] = float32(w)
		}
	}
	copy(t.chanTaps[:], kaiserLowpass(chanTaps, (chanPassHz+chanStopHz)/2/basebandRate, 7))
	copy(t.half[:], kaiserLowpass(halfTaps, 0.25, halfBeta))
	copy(t.pre[:], kaiserLowpass(preTaps, discCutHz/discRate, 5))
	for i, v := range rrcTapsSpan(discSPS, discSpan) {
		t.rrc[i] = float32(v)
	}
	q15 := func(v float32) int16 { return int16(math.Round(float64(v) * 32768)) }
	t.halfQ15[0] = q15(t.half[halfTaps/2])
	for k := range halfSide {
		t.halfQ15[k+1] = q15(t.half[halfTaps/2-(2*k+1)])
	}
	for i := range t.preQ15 {
		t.preQ15[i] = q15(t.pre[i])
	}
	for i := range t.rrcQ15 {
		t.rrcQ15[i] = q15(t.rrc[i])
	}
	for i := range t.twiddles {
		p := -2 * math.Pi * float64(i) / fftSize
		im := float32(math.Sin(p))
		if im == 0 {
			// A constant has no negative zero, and the sign of a zero
			// twiddle part does not change the power spectrum.
			im = 0
		}
		t.twiddles[i] = complex(float32(math.Cos(p)), im)
	}
	for i := range t.window {
		t.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize))
	}
	for i := range numSources {
		v, d, _ := sourceAt(i).SyncPatterns()
		t.patterns[2*i], t.patterns[2*i+1] = v, d
		var voice [syncSymbols]int8
		syncSyms(v, &voice)
		var mp float32
		for _, a := range voice {
			mp += float32(a)
		}
		mp /= syncSymbols
		for k, a := range voice {
			t.dev[i][k] = float32(a) - mp
			t.devSq[i] += t.dev[i][k] * t.dev[i][k]
		}
		for row, dec := range []int{1, 2} {
			t.syncTemplates(row, dec, i, &voice)
		}
	}
	t.patterns[2*numSources] = enums.SyncMSSourcedRC
	for row := range 2 {
		sps := samplesPerSy / (row + 1)
		taps := rrcTapsAt(float64(sps))
		for i, v := range taps {
			t.pulse[row][i+1] = t.pulse[row][i] + v
			t.pulse32[row][i+1] = float32(t.pulse[row][i+1])
		}
	}
	for row, np := range []int{phaseStates, 52} {
		for p := range np {
			t.cos[row][p] = float32(math.Cos(2 * math.Pi * float64(p) / float64(np)))
			t.sin[row][p] = float32(math.Sin(2 * math.Pi * float64(p) / float64(np)))
		}
	}
	return t
}

// syncTemplates sets the templates of source src, whose voice SYNC has the
// symbols voice, in t.templates at the given row, for the channel ring at
// every dec channel samples.
func (t *dspTables) syncTemplates(row, dec, src int, voice *[syncSymbols]int8) {
	sps, span := samplesPerSy/dec, syncSpan/dec
	syms := make([]int, syncSymbols)
	for k, a := range voice {
		syms[k] = int(a)
	}
	ph := refPhase(syms, sps, 0.27)
	tpl := make([]complex64, span)
	for j := range span {
		p := ph[syncEdge*sps+j]
		tpl[j] = complex(float32(math.Cos(p)), float32(-math.Sin(p)))
	}
	for fi := range searchFreqs {
		f := searchHzAt(fi)
		for j := range span {
			p := -2 * math.Pi * f * float64(j*dec) / basebandRate
			t.templates[row][fi*numSources+src][j] = tpl[j] * complex(float32(math.Cos(p)), float32(math.Sin(p)))
		}
	}
}

// rrcTapsAt returns rrcTaps for sps samples per symbol.
func rrcTapsAt(sps float64) []float64 {
	return rrcTapsSpan(sps, rrcSpan)
}

// rrcTapsSpan returns rrcTaps for sps samples per symbol, span symbols on
// each side.
func rrcTapsSpan(sps float64, span int) []float64 {
	n := int(2*float64(span)*sps) + 1
	h := make([]float64, n)
	var c float64
	for i := range h {
		t := float64(i-n/2) / sps
		switch {
		case t == 0:
			h[i] = 1 - rrcAlpha + 4*rrcAlpha/math.Pi
		case math.Abs(math.Abs(4*rrcAlpha*t)-1) < 1e-9:
			h[i] = rrcAlpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*rrcAlpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*rrcAlpha)))
		default:
			h[i] = (math.Sin(math.Pi*t*(1-rrcAlpha)) + 4*rrcAlpha*t*math.Cos(math.Pi*t*(1+rrcAlpha))) /
				(math.Pi * t * (1 - (4*rrcAlpha*t)*(4*rrcAlpha*t)))
		}
		c += h[i]
	}
	for i := range h {
		h[i] /= c
	}
	return h
}

// refPhase returns the phase at each sample of the symbols syms when
// symbols before and after them are 0: n*sps samples, sample j of symbol k
// at offset j-sps/2 from its center, phase 0 before the first symbol.
func refPhase(syms []int, sps int, h float64) []float64 {
	taps := rrcTapsAt(float64(sps))
	half := len(taps) / 2
	q := make([]float64, len(taps)+1)
	for i, v := range taps {
		q[i+1] = q[i] + v
	}
	span := half/sps + 1
	out := make([]float64, len(syms)*sps)
	var done float64
	for k := range syms {
		if i := k - span - 1; i >= 0 {
			done += h * math.Pi * float64(syms[i])
		}
		for j := range sps {
			t := k*sps + j - sps/2
			ph := done
			for i := max(k-span, 0); i <= min(k+span, len(syms)-1); i++ {
				idx := t - i*sps + half
				switch {
				case idx >= len(taps):
					ph += h * math.Pi * float64(syms[i])
				case idx >= 0:
					ph += h * math.Pi * float64(syms[i]) * q[idx+1]
				}
			}
			out[k*sps+j] = ph
		}
	}
	return out
}

// kaiserLowpass returns a Kaiser windowed sinc low-pass of n taps with
// cutoff fc (a fraction of the sample rate) and unity DC gain.
func kaiserLowpass(n int, fc, beta float64) []float32 {
	h := make([]float32, n)
	var sum float64
	v := make([]float64, n)
	for i := range n {
		m := float64(i) - float64(n-1)/2
		x := 2 * fc
		if m != 0 {
			x = math.Sin(2*math.Pi*fc*m) / (math.Pi * m)
		}
		r := 2*float64(i)/float64(n-1) - 1
		v[i] = x * bessel0(beta*math.Sqrt(max(0, 1-r*r))) / bessel0(beta)
		sum += v[i]
	}
	for i := range h {
		h[i] = float32(v[i] / sum)
	}
	return h
}
