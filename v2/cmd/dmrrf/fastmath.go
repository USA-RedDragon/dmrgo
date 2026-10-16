package main

import "math"

// atan2f returns atan2(y, x) in single precision, within 1.2e-5 radians,
// from an odd minimax polynomial of degree 9 for atan on [0, 1]. A single
// precision FPU, such as that of a Cortex-M4F, runs it in hardware, where
// math.Atan2 runs in software.
func atan2f(y, x float32) float32 {
	ax, ay := x, y
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	if ax == 0 && ay == 0 {
		return 0
	}
	swap := ay > ax
	a := ay / ax
	if swap {
		a = ax / ay
	}
	s := a * a
	r := a * (0.99986633 + s*(-0.3303048+s*(0.18015937+s*(-0.08515649+s*0.020845189))))
	if swap {
		r = math.Pi/2 - r
	}
	if x < 0 {
		r = math.Pi - r
	}
	if y < 0 {
		r = -r
	}
	return r
}

// sqrtf returns the square root of x in single precision.
func sqrtf(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}

// abs32 returns the absolute value of x.
func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// pow2 returns 2 to the power e, for e from -126 to 127, and 0 below.
func pow2(e int32) float32 {
	b := e + 127
	if b <= 0 || b > 254 {
		return 0
	}
	return math.Float32frombits(uint32(b) << 23)
}

// sat8 returns x rounded to the nearest integer and saturated to int8.
func sat8(x float32) int8 {
	switch {
	case x >= math.MaxInt8:
		return math.MaxInt8
	case x <= math.MinInt8:
		return math.MinInt8
	case x < 0:
		return int8(x - 0.5)
	}
	return int8(x + 0.5)
}
