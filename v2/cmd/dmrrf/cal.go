package main

import (
	"math"
	"math/cmplx"
)

const (
	basebandRate = 48000
	burstSeconds = 0.0275
)

func decimate(x []complex128, rate float64) []complex128 {
	return boxcar(x, rate, basebandRate)
}

func boxcar(x []complex128, rate, to float64) []complex128 {
	ratio := rate / to
	n := int(float64(len(x)) / ratio)
	out := make([]complex128, n)
	for i := range n {
		lo := int(float64(i) * ratio)
		hi := min(int(float64(i+1)*ratio), len(x))
		var s complex128
		for _, v := range x[lo:hi] {
			s += v
		}
		out[i] = s / complex(float64(max(hi-lo, 1)), 0)
	}
	return out
}

func power(x []complex128) []float64 {
	p := make([]float64, len(x))
	for i, v := range x {
		p[i] = real(v)*real(v) + imag(v)*imag(v)
	}
	return p
}

func burstPower(pw []float64) (sum []float64, ref float64) {
	n := min(int(burstSeconds*basebandRate), len(pw))
	sum = make([]float64, len(pw)+1)
	for i, v := range pw {
		sum[i+1] = sum[i] + v
	}
	for i := n; i < len(sum); i++ {
		ref = math.Max(ref, sum[i]-sum[i-n])
	}
	return sum, ref / float64(max(n, 1))
}

func carrierOffset(x []complex128) float64 {
	sum, ref := burstPower(power(x))
	n := int(burstSeconds * basebandRate)
	var acc complex128
	for i := 1; i < len(x); i++ {
		lo := max(i-n/2, 0)
		hi := min(lo+n, len(x))
		if (sum[hi]-sum[lo])/float64(hi-lo) > ref/4 {
			acc += x[i] * cmplx.Conj(x[i-1])
		}
	}
	return cmplx.Phase(acc) * basebandRate / (2 * math.Pi)
}
