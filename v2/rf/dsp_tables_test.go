package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

type dspTables struct {
	half     [halfTaps]float32
	pre      [preTaps]float32
	rrc      [rrcTaps]float32
	halfQ15  [halfSide + 1]int16
	preQ15   [preTaps/2 + 1]int16
	rrcQ15   [rrcTaps/2 + 1]int16
	audioQ15 [audio12Taps/2 + 1]int16
	audio96  [audio96Taps/2 + 1]int16
	twiddles [fftSize / 2]complex64
	window   [fftSize/2 + 1]float32
	devSq    [numSources]float32
	syms8    [numSources][syncSymbols]int8
	mean     [numSources]float32
	pulse    [2][2*rrcSpan*samplesPerSy + 2]float64
	cos, sin [2][phaseStates]float32
	audioUp  [(audioUpPhases + 1) * audioUpTaps]int16
}

func deriveDSP() *dspTables {
	t := &dspTables{}
	var up [len(t.audioUp)]float32
	resample.Design(up[:], audioUpTaps, audioUpPhases, audioUpCut, audioUpBeta)
	for i, v := range up {
		t.audioUp[i] = int16(math.Round(float64(v) * 32768))
	}
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
	for i, v := range rrcTapsSpan(discSPS/2.0, discSpan)[:len(t.audioQ15)] {
		t.audioQ15[i] = q15(float32(v))
	}
	for i, v := range rrcTapsSpan(2, audio96Span)[:len(t.audio96)] {
		t.audio96[i] = q15(float32(v))
	}

	for i := range t.twiddles {
		p := -2 * math.Pi * float64(i) / fftSize
		im := float32(math.Sin(p))
		if im == 0 {
			im = 0
		}
		t.twiddles[i] = complex(float32(math.Cos(p)), im)
	}
	for i := range t.window {
		t.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize))
	}
	for i := range numSources {
		v, _, _ := sourceAt(i).SyncPatterns()
		var voice [syncSymbols]int8
		syncSyms(v, &voice)
		var mp float32
		for _, a := range voice {
			mp += float32(a)
		}
		mp /= syncSymbols
		t.syms8[i], t.mean[i] = voice, mp
		for _, a := range voice {
			d := float32(a) - mp
			t.devSq[i] += d * d
		}
	}
	for row := range 2 {
		sps := samplesPerSy / (row + 1)
		taps := rrcTapsAt(float64(sps))
		for i, v := range taps {
			t.pulse[row][i+1] = t.pulse[row][i] + v
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

func rrcTapsAt(sps float64) []float64 {
	return rrcTapsSpan(sps, rrcSpan)
}

func rrcTapsSpan(sps float64, span int) []float64 {
	n := int(2*float64(span)*sps) + 1
	h := make([]float64, n)
	var c float64
	for i := range h {
		h[i] = rrcPulse(float64(i-n/2) / sps)
		c += h[i]
	}
	for i := range h {
		h[i] /= c
	}
	return h
}

func rrcPulse(t float64) float64 {
	switch {
	case t == 0:
		return 1 - rrcAlpha + 4*rrcAlpha/math.Pi
	case math.Abs(math.Abs(4*rrcAlpha*t)-1) < 1e-9:
		return rrcAlpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*rrcAlpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*rrcAlpha)))
	}
	return (math.Sin(math.Pi*t*(1-rrcAlpha)) + 4*rrcAlpha*t*math.Cos(math.Pi*t*(1+rrcAlpha))) /
		(math.Pi * t * (1 - (4*rrcAlpha*t)*(4*rrcAlpha*t)))
}

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
		v[i] = x * resample.Bessel0(beta*math.Sqrt(max(0, 1-r*r))) / resample.Bessel0(beta)
		sum += v[i]
	}
	for i := range h {
		h[i] = float32(v[i] / sum)
	}
	return h
}
