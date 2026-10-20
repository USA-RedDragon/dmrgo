package rf

import (
	"math/rand/v2"
	"testing"
)

func TestTickSeconds(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	check := func(v int64) {
		want := float64(v) * (1.0 / (basebandRate * 256))
		if got := tickSeconds(v); got != want {
			t.Fatalf("tickSeconds(%d) = %v, want %v", v, got, want)
		}
	}
	for v := int64(-1 << 12); v <= 1<<12; v++ {
		check(v)
	}
	for i := 0; i < 1<<20; i++ {
		v := rng.Int64() >> (10 + rng.IntN(53))
		check(v)
		check(-v)
	}
	for s := 0; s < 53; s++ {
		check(1 << s)
		check(1<<s - 1)
		check(1<<s + 1)
	}
}
