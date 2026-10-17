package text

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func TestInt(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	for _, v := range []int64{0, 1, -1, 9, 10, -10, math.MaxInt64, math.MinInt64} {
		if got, want := Int(v), fmt.Sprintf("%d", v); got != want {
			t.Errorf("Int(%d) = %q", v, got)
		}
	}
	for range 10000 {
		v := r.Uint64() >> r.Intn(64)
		if got, want := Int(v), fmt.Sprintf("%d", v); got != want {
			t.Fatalf("Int(%d) = %q", v, got)
		}
		if got, want := Int(int8(v)), fmt.Sprintf("%d", int8(v)); got != want {
			t.Fatalf("Int(int8(%d)) = %q", int8(v), got)
		}
		if got, want := Int(int64(v)), fmt.Sprintf("%d", int64(v)); got != want {
			t.Fatalf("Int(%d) = %q", int64(v), got)
		}
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	for n := range 20 {
		b := make([]byte, n)
		r.Read(b)
		if got, want := Hex(b), fmt.Sprintf("%x", b); got != want {
			t.Errorf("Hex(%v) = %q, want %q", b, got, want)
		}
		if got, want := Bytes(b), fmt.Sprintf("%v", b); got != want {
			t.Errorf("Bytes(%v) = %q, want %q", b, got, want)
		}
	}
	if got := Bool(true) + Bool(false); got != "truefalse" {
		t.Errorf("Bool = %q", got)
	}
}

func TestDigits(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	for range 1000 {
		v := r.Uint64()
		if got, want := Digits(v>>56, 8, 1), fmt.Sprintf("%08b", v>>56); got != want {
			t.Fatalf("Digits(%d, 8, 1) = %q, want %q", v>>56, got, want)
		}
		w := v >> r.Intn(64)
		if got, want := Digits(w, 12, 4), fmt.Sprintf("%012x", w); got != want {
			t.Fatalf("Digits(%d, 12, 4) = %q, want %q", w, got, want)
		}
		if got, want := Digits(w, 4, 1), fmt.Sprintf("%04b", w); got != want {
			t.Fatalf("Digits(%d, 4, 1) = %q, want %q", w, got, want)
		}
	}
}

func TestFixed(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	vals := make([]float64, 0, 400100)
	vals = append(vals,
		0, math.Copysign(0, -1), 1, -1, 0.5, 0.0078125, -0.0078125, 2.5e-7, 5e-7, -1e-10,
		0.9999995, 0.99999949, 1e15, 1e21, 1e300, -1e300, math.MaxFloat64,
		math.SmallestNonzeroFloat64, 1<<52, 1<<53, 180, -180, 90,
		math.NaN(), math.Inf(1), math.Inf(-1),
	)
	for range 100000 {
		vals = append(vals,
			float64(int32(r.Uint32())>>8)*360/(1<<24),
			float64(int32(r.Uint32())>>7)*180/(1<<25),
			r.NormFloat64()*math.Pow(10, float64(r.Intn(40)-20)),
			math.Float64frombits(r.Uint64()),
		)
	}
	for _, v := range vals {
		if got, want := Fixed(v), fmt.Sprintf("%f", v); got != want {
			t.Fatalf("Fixed(%b) = %q, want %q", v, got, want)
		}
	}
	for _, v := range []float64{0, -180, 180, 90, -90, 0.5, -0.25, 1e-4, 999999.125, math.Copysign(0, -1), math.NaN(), math.Inf(-1)} {
		if got, want := Float(v), fmt.Sprintf("%v", v); got != want {
			t.Errorf("Float(%v) = %q, want %q", v, got, want)
		}
	}
}
