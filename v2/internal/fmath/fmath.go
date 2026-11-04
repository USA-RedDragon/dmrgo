// Package fmath holds float64 helpers that compile on every target that
// TinyGo builds dmrgo for.
package fmath

import "math"

// Max returns the larger of x and y as the builtin max does: NaN when
// either is NaN, and +0 for +0 and -0. LLVM cannot select the builtin for
// float64 on Arm cores with a hardware FPU.
func Max(x, y float64) float64 {
	switch {
	case x != x || y != y:
		return math.NaN()
	case x == y:
		if math.Signbit(x) {
			return y
		}
		return x
	case x > y:
		return x
	}
	return y
}

// Min returns the smaller of x and y as the builtin min does: NaN when
// either is NaN, and -0 for +0 and -0.
func Min(x, y float64) float64 {
	switch {
	case x != x || y != y:
		return math.NaN()
	case x == y:
		if math.Signbit(x) {
			return x
		}
		return y
	case x < y:
		return x
	}
	return y
}
