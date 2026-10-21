package fsktx

import (
	"math"
	"slices"
	"testing"
)

func corrMaskMargin(t *testing.T, corr int, r radio) float64 {
	t.Helper()
	a, err := NewAudio(AudioConfig{Hz: 48000, Tier: Tier1, Exact: true, Pulse: corrPulse(), Corr: corr, CorrHiHz: 12000, CorrTaps: 81, Corr48k: true})
	if err != nil {
		t.Fatal(err)
	}
	var out []int16
	for _, s := range newTestStream(t, 6, 40).syms {
		out = a.AppendSymbol(out, s)
	}
	x := dacFM(out, 48000, r)
	x = x[len(x)/10:]
	const n = 2048
	w := make([]float64, n)
	var wsum float64
	for i := range w {
		c := 2 * math.Pi * float64(i) / n
		w[i] = 0.35875 - 0.48829*math.Cos(c) + 0.14128*math.Cos(2*c) - 0.01168*math.Cos(3*c)
		wsum += w[i] * w[i]
	}
	peak := make([]float64, n)
	buf := make([]complex128, n)
	for st := 0; st+n <= len(x); st += n / 16 {
		for i := range buf {
			buf[i] = x[st+i] * complex(w[i], 0)
		}
		fftInPlace(buf)
		for i, v := range buf {
			peak[i] = max(peak[i], (real(v)*real(v)+imag(v)*imag(v))/(n*wsum))
		}
	}
	bin := float64(dacRate) / n
	m := math.Inf(1)
	for i := range n {
		f := float64(i) * bin
		if i >= n/2 {
			f -= dacRate
		}
		if math.Abs(f) < 5625 || math.Abs(f) > 12500 {
			continue
		}
		p := peak[i] + peak[(i+n-1)%n]
		m = min(m, -10*math.Log10(p)-optMaskAttn(f))
	}
	return m
}

func TestCorrFlatRadio(t *testing.T) {
	t.Parallel()
	for _, r := range []radio{{}, {ideal: true}} {
		off, on := corrMaskMargin(t, 0, r), corrMaskMargin(t, 5, r)
		t.Logf("ideal %v: Mask D margin %.2f dB without Corr, %.2f dB with it", r.ideal, off, on)
		if on < off+0.5 {
			t.Errorf("ideal %v: Corr leaves %.2f dB of Mask D margin, %.2f dB without it", r.ideal, on, off)
		}
	}
}

func TestShapeMinLevel(t *testing.T) {
	t.Parallel()
	syms := newTestStream(t, 1, 40).syms
	run := func(level, shape, minLevel int) []int16 {
		a, err := NewAudio(AudioConfig{Hz: 48000, Tier: Tier1, Level: level, Offset: 512, Shape: shape, NoiseZerosHz: [4]float64{3000, 21000}, ShapeMinLevel: minLevel})
		if err != nil {
			t.Fatal(err)
		}
		var out []int16
		for _, s := range syms {
			out = a.AppendSymbol(out, s)
		}
		return out
	}
	if !slices.Equal(run(64, 2, 100), run(64, 0, 0)) {
		t.Error("Level 64 under ShapeMinLevel 100 is still shaped")
	}
	if slices.Equal(run(260, 2, 100), run(260, 0, 0)) || !slices.Equal(run(260, 2, 100), run(260, 2, 0)) {
		t.Error("Level 260 over ShapeMinLevel 100 is not shaped as Shape 2")
	}
}
