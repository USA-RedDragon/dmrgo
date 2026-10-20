//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestTxMatrixRange(t *testing.T) {
	t.Parallel()
	full := txFull()
	n := 300
	if full {
		n = 3000
	}
	for _, u := range levelUnits(full) {
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			for p := range patCount {
				checkAudioRange(t, u, p, u.run(t, u.pattern(p, n), nil))
			}
		})
	}
	for _, u := range offsetUnits(full) {
		if u.rate > 2400000 && !full {
			continue
		}
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			for p := range patCount {
				syms := u.pattern(p, n)
				checkIQRange(t, u, p, syms, u.run(t, syms, keyedOn()))
			}
		})
	}
}

func checkAudioRange(t *testing.T, u txUnit, p int, raw []int16) {
	t.Helper()
	lo, hi := u.audioBounds()
	tol := 0.5 + 1e-6*math.Abs(hi)
	if u.mcu || u.tier == Tier0 {
		tol = 0
	}
	most, least := math.Inf(-1), math.Inf(1)
	for _, v := range raw {
		most, least = max(most, float64(v)), min(least, float64(v))
	}
	if most > hi+tol || least < lo-tol {
		t.Errorf("%s: output %v to %v, bounds %v to %v", patternName(p), least, most, lo, hi)
	}
	if dac12 := u.offset == 2048 && (u.level == 1040 || u.level == 1080 && (u.mcu || u.tier == Tier0)); dac12 && (most > 4095 || least < 0) {
		t.Errorf("%s: output %v to %v at level %d, beyond a 12-bit DAC", patternName(p), least, most, u.level)
	}
	if p == patWorst {
		if most < hi-tol || least > lo+tol {
			t.Errorf("worst: output %v to %v, short of the bounds %v to %v", least, most, lo, hi)
		}
	}
	rest := raw[len(raw)-u.rate/SymbolRate:]
	for _, v := range rest {
		if math.Abs(float64(v)-float64(u.offset)) > tol {
			t.Errorf("%s: output %d at rest, offset %d", patternName(p), v, u.offset)
			break
		}
	}
}

func checkIQRange(t *testing.T, u txUnit, p int, syms []int8, raw []int16) {
	t.Helper()
	a := u.amplitude()
	x := iqComplex(raw)
	if want := len(syms) * u.rate / SymbolRate; len(x) < want-1 || len(x) > want+1 {
		t.Errorf("%s: %d samples, want %d", patternName(p), len(x), want)
	}
	for i, v := range raw {
		if math.Abs(float64(v)) > a {
			t.Errorf("%s: value %d at %d beyond the amplitude %v", patternName(p), v, i, a)
			break
		}
	}
	start := u.ramp + 1
	q := 1.5 / a
	if u.format == fmtS16 && (u.mcu || u.tier == Tier0) {
		q = 3.0 / a
	}
	for i := start; i < len(x); i++ {
		if r := cmplx.Abs(x[i]) / a; math.Abs(r-1) > q {
			t.Errorf("%s: power %.5f of full at sample %d", patternName(p), r*r, i)
			break
		}
	}
	w := max(u.rate/rxRate, 1)
	peak := u.peakHz()
	up := func(i int) float64 { return min(cmplx.Abs(x[i]), cmplx.Abs(x[i+w])) / a }
	base := shift(x, float64(u.rate), -float64(u.offHz))
	for i := 0; i+w < len(x); i += w {
		r := up(i)
		if r < 0.25 {
			continue
		}
		noise := (2*q/r + u.phaseStep()) * float64(u.rate) / (2 * math.Pi * float64(w))
		f := cmplx.Phase(base[i+w]*cmplx.Conj(base[i]))/(2*math.Pi*float64(w))*float64(u.rate) + float64(u.offHz)
		if math.Abs(f-float64(u.offHz)) > peak*1.001+noise+1 {
			t.Errorf("%s: frequency %.1f Hz at sample %d, carrier %d Hz and peak deviation %.1f Hz", patternName(p), f, i, u.offHz, peak)
			break
		}
	}
	w = 1
	for i := 0; i+1 < len(x); i++ {
		r := up(i)
		if r < 0.25 {
			continue
		}
		step := 2*math.Pi*(math.Abs(float64(u.offHz))+peak*1.001+1)/float64(u.rate) + 2*q/r + u.phaseStep()
		if d := math.Abs(cmplx.Phase(x[i+1] * cmplx.Conj(x[i]))); d > step {
			t.Errorf("%s: phase step %.4f rad at sample %d, most %.4f", patternName(p), d, i, step)
			break
		}
	}
}
