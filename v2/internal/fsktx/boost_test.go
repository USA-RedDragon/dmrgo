package fsktx

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestBoostPattern(t *testing.T) {
	t.Parallel()
	a := &Audio{bpairs: 4, boost: 0.01 / 4}
	pat := []int8{3, 3, -3, -3}
	var v float64
	for i := range 64 {
		v = a.boosted(pat[i%4])
	}
	if want := float64(pat[63%4]) * 1.01; math.Abs(v-want) > 1e-12 {
		t.Fatalf("pattern: got %v, want %v", v, want)
	}
	b := &Audio{bpairs: 4, boost: 0.01 / 4}
	for range 64 {
		v = b.boosted(1)
	}
	if v != 1 {
		t.Fatalf("run: got %v, want 1", v)
	}
}

func TestBoostSum(t *testing.T) {
	t.Parallel()
	const m = 42
	a := &Audio{bpairs: m, boost: 1}
	r := rand.New(rand.NewPCG(1, 2))
	h := make([]int8, 0, 5000)
	for k := range 5000 {
		s := int8(2*r.IntN(4) - 3)
		var u int
		for j := 1; j <= m; j++ {
			if i := k - 2*j; i >= 0 {
				if j%2 == 1 {
					u -= int(h[i])
				} else {
					u += int(h[i])
				}
			}
		}
		h = append(h, s)
		if got := a.boosted(s); got != float64(s)+float64(u) {
			t.Fatalf("symbol %d: got %v, want %v", k, got, float64(s)+float64(u))
		}
	}
}
