//go:build !tinygo

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
	var a, b []int16
	syms := [...]int8{3, 3, -3, -3, 1, -1, 3, -1, -3, 1, 1, 3}
	for i := range 2000 {
		s := syms[i%len(syms)]
		a = flat.AppendSymbol(a, s)
		b = pre.AppendSymbol(b, s)
	}
	x := make([]float64, len(b))
	for i, v := range b {
		x[i] = float64(v)
	}
	y := preEmph(x, hz)
	var worst float64
	for i := len(a) / 10; i < len(a)*9/10; i++ {
		worst = max(worst, math.Abs(y[i]-float64(a[i])))
	}
	if worst > 12 {
		t.Errorf("pre-emphasized output through the analog pre-emphasis differs from flat by %.1f codes, want at most 12", worst)
	}
}
