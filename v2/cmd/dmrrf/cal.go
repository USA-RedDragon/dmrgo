package main

import (
	"math"
	"math/cmplx"
)

const (
	basebandRate = 48000
	burstSeconds = 0.0275
)

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
