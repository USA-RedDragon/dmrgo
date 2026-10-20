//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
)

const (
	rampStep = iota
	rampLinear
	rampCosine
)

func rampEnvelope(length, on, off, n, kind int) []float64 {
	e := make([]float64, length)
	for i := range e {
		var a float64
		switch {
		case i >= on && i < off:
			a = 1
		case i >= on-n && i < on:
			a = rampShape(float64(i-(on-n))/float64(n), kind)
		case i >= off && i < off+n:
			a = rampShape(1-float64(i-off)/float64(n), kind)
		}
		e[i] = a
	}
	return e
}

func rampShape(x float64, kind int) float64 {
	switch kind {
	case rampLinear:
		return x
	case rampCosine:
		return 0.5 - 0.5*math.Cos(math.Pi*x)
	default:
		return 1
	}
}

func adjacentPower(x []complex128, rate, offset float64) []float64 {
	trans := 1000.0 / rate
	n := int(math.Ceil((90-8)/(2.285*2*math.Pi*trans))) | 1
	half := n / 2
	fc := 4250.0 / rate
	beta := 0.1102 * (90 - 8.7)
	h := make([]float64, n)
	var sum float64
	for j := range h {
		t := float64(j - half)
		v := 2 * fc
		if t != 0 {
			v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
		}
		h[j] = v * taper(winKaiser, beta, t/float64(half+1))
		sum += h[j]
	}
	y := make([]complex128, len(x))
	for i, v := range x {
		y[i] = v * cmplx.Rect(1, -2*math.Pi*offset*float64(i)/rate)
	}
	p := make([]float64, len(x))
	for i := range p {
		var acc complex128
		for j, c := range h {
			if k := i + j - half; k >= 0 && k < len(y) {
				acc += y[k] * complex(c/sum, 0)
			}
		}
		p[i] = real(acc)*real(acc) + imag(acc)*imag(acc)
	}
	return p
}

// ETSI EN 300 113 clause 7.9
func transient(x []complex128, rate, carrier float64) float64 {
	w := int(rate * 1e-4)
	worst := 0.0
	for _, off := range []float64{12500, -12500} {
		p := adjacentPower(x, rate, off)
		var run float64
		for i := range p {
			run += p[i]
			if i >= w {
				run -= p[i-w]
			}
			worst = max(worst, run/float64(w))
		}
	}
	return 10 * math.Log10(worst/carrier)
}
