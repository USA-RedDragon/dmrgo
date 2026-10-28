package resample

import (
	"math"
	"testing"
)

func table(taps, phases int, lerp bool, fc float64) []float32 {
	rows := phases
	if lerp {
		rows++
	}
	h := make([]float32, rows*taps)
	Design(h, taps, phases, fc, 8)
	return h
}

func TestDesignGain(t *testing.T) {
	t.Parallel()
	for _, fc := range []float64{0.45, 0.25, 0.1} {
		h := table(72, 256, false, fc)
		for p := range 256 {
			var s float64
			for _, c := range h[p*72 : (p+1)*72] {
				s += float64(c)
			}
			if math.Abs(s-1) > 1e-3 {
				t.Fatalf("fc %v, phase %d: gain %v", fc, p, s)
			}
		}
	}
}

func sineError(t *testing.T, in, out float64, taps, phases int, lerp bool) float64 {
	t.Helper()
	fc := 0.45 * min(1, out/in)
	h := table(taps, phases, lerp, fc)
	var c Complex
	var r Real
	if !c.Init(h, taps, phases, lerp, in/out, make([]complex64, 2*taps)) || !r.Init(h, taps, phases, lerp, in/out, make([]int16, taps)) {
		t.Fatal("Init failed")
	}
	const f, a = 1000.0, 20000.0
	bc := make([]complex64, c.MaxOut())
	br := make([]int16, r.MaxOut())
	var worst float64
	k := 0
	for i := range int(in) {
		w := 2 * math.Pi * f * float64(i) / in
		nc := c.Push(complex64(complex(a*math.Cos(w), a*math.Sin(w))), bc)
		nr := r.Push(int16(math.Round(a*math.Cos(w))), br)
		if nc != nr {
			t.Fatalf("%d complex outputs, %d real", nc, nr)
		}
		for j := range nc {
			u := 2 * math.Pi * f * float64(k) * (in / out) / in
			want := complex(a*math.Cos(u), a*math.Sin(u))
			if k > 2*taps {
				worst = max(worst, math.Hypot(real(complex128(bc[j]))-real(want), imag(complex128(bc[j]))-imag(want)), math.Abs(float64(br[j])-real(want))-0.5)
			}
			k++
		}
	}
	if math.Abs(float64(k)-out) > float64(taps)*out/in+2 {
		t.Fatalf("%v Hz to %v Hz: %d outputs in a second", in, out, k)
	}
	return 20 * math.Log10(max(worst, 1e-9)/a)
}

func TestResampleSine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in, out      float64
		taps, phases int
		lerp         bool
		most         float64
	}{
		{44100, 48000, 72, 256, false, -60},
		{44100, 48000, 32, 64, true, -60},
		{44100, 48000, 8, 32, true, -40},
		{22050, 24000, 8, 32, true, -40},
		{96000, 48000, 72, 256, false, -60},
		{250000, 48000, 72, 256, false, -60},
		{8000, 9600, 16, 32, true, -40},
	} {
		e := sineError(t, c.in, c.out, c.taps, c.phases, c.lerp)
		t.Logf("%v Hz to %v Hz, %d taps, %d phases, lerp %v: %.1f dB", c.in, c.out, c.taps, c.phases, c.lerp, e)
		if e > c.most {
			t.Errorf("%+v: error %.1f dB", c, e)
		}
	}
}

func TestInitRejects(t *testing.T) {
	t.Parallel()
	h := table(8, 4, false, 0.4)
	var c Complex
	var r Real
	for _, step := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if c.Init(h, 8, 4, false, step, make([]complex64, 16)) || r.Init(h, 8, 4, false, step, make([]int16, 8)) {
			t.Errorf("step %v taken", step)
		}
	}
	if c.Init(h, 8, 4, true, 1.5, make([]complex64, 16)) || r.Init(h, 8, 5, false, 1.5, make([]int16, 8)) {
		t.Error("a short table taken")
	}
	if c.Init(h, 8, 4, false, 1.5, make([]complex64, 15)) || r.Init(h, 8, 4, false, 1.5, make([]int16, 7)) {
		t.Error("a short history taken")
	}
	if c.Push(1, make([]complex64, 4)) != 0 || r.Push(1, make([]int16, 4)) != 0 || c.MaxOut() != 0 {
		t.Error("a zero resampler gave outputs")
	}
}
