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
