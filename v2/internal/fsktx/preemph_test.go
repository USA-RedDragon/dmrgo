package fsktx

import (
	"math"
	"testing"
)

func TestPreEmphasisInverts(t *testing.T) {
	t.Parallel()
	const hz = 48000
	flat, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier2})
	if err != nil {
		t.Fatal(err)
	}
	pre, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier2, PreEmphasisHz: [2]float64{300, 3000}})
	if err != nil {
		t.Fatal(err)
	}
	z := math.Tan(math.Pi * 300 / hz)
	p := math.Tan(math.Pi * 3000 / hz)
	a0 := 1 + 1/p
	b0, b1, a1 := (1+1/z)/a0, (1-1/z)/a0, (1-1/p)/a0
	var a, b []int16
	var x1, y1, worst float64
	syms := [...]int8{3, 3, -3, -3, 1, -1, 3, -1, -3, 1, 1, 3}
	for i := range 2000 {
		s := syms[i%len(syms)]
		a = flat.AppendSymbol(a[:0], s)
		b = pre.AppendSymbol(b[:0], s)
		for j, v := range b {
			x := float64(v)
			y := b0*x + b1*x1 - a1*y1
			x1, y1 = x, y
			if i > 200 {
				worst = max(worst, math.Abs(y-float64(a[j])))
			}
		}
	}
	if worst > 12 {
		t.Errorf("pre-emphasized output differs from flat by %.1f codes, want at most 12", worst)
	}
}

func TestPreEmphasisRange(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]float64{{-1, 3000}, {3000, 300}, {300, 0}, {300, 30000}} {
		if _, err := NewAudio(AudioConfig{Tier: Tier1, PreEmphasisHz: c}); err == nil {
			t.Errorf("PreEmphasisHz %v: no error", c)
		}
	}
}
