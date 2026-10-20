package rf

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestRingRotateCompact(t *testing.T) {
	t.Parallel()
	r := newRing(64, 1, true)
	val := func(i int64) complex64 {
		return complex64(cmplx.Rect(1+float64(i%7)/7, float64(i)*0.3))
	}
	const n = 203
	for i := range int64(n) {
		r.push(val(i))
	}
	const w = 0.1
	r.rotate(1, complex64(cmplx.Rect(1, w)))
	first := r.first()
	for i := first; i < n; i++ {
		want := val(i) * complex64(cmplx.Rect(1, w*float64(i-first)))
		if d := cmplx.Abs(complex128(r.at(i) - want)); d > 0.03 {
			t.Errorf("sample %d: %v, want %v", i, r.at(i), want)
		}
	}
	if r.at(first-1) != 0 {
		t.Errorf("sample %d before the ring: %v", first-1, r.at(first-1))
	}
	if math.Abs(float64(first)-float64(n&^(qBlock-1)-64)) != 0 {
		t.Errorf("first %d", first)
	}
}

func TestQringAdd(t *testing.T) {
	t.Parallel()
	r := newQring(50, discStepHz)
	val := func(i int64) float32 { return float32((i*37)%900 - 450) }
	for i := range int64(80) {
		r.push(val(i))
	}
	var sum float32
	for _, d := range []float32{-1400.4, 21.7, 37, 300.25} {
		r.add(d)
		sum += d
	}
	for i := int64(30); i < 80; i++ {
		if e := math.Abs(float64(r.at(i) - val(i) - sum)); e > discStepHz/2+0.01 {
			t.Errorf("sample %d: off by %.1f Hz", i, e)
		}
	}
}
