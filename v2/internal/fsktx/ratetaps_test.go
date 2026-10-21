package fsktx

import (
	"math"
	"slices"
	"testing"
)

func TestRateTapsAddToPulse(t *testing.T) {
	syms := newTestStream(t, 5, 60).syms
	run := func(r RateDesign) []int16 {
		a, err := NewAudio(AudioConfig{Hz: 24000, Tier: Tier1, Exact: true, Rates: []RateDesign{r}})
		if err != nil {
			t.Fatal(err)
		}
		var out []int16
		for _, s := range syms {
			out = a.AppendSymbol(out, s)
		}
		return out
	}
	base := run(RateDesign{Hz: 24000})
	if !slices.Equal(base, run(RateDesign{Hz: 24000, Taps: make([]float64, 79)})) {
		t.Fatal("taps of the wrong length changed the output")
	}
	taps := make([]float64, 80)
	taps[7] = 100
	got := run(RateDesign{Hz: 24000, Taps: taps})
	scale := float64(tableUnits) / (3 * DeviationHz)
	for n := range got {
		var want float64
		if k := (n - 7) / 5; n >= 7 && (n-7)%5 == 0 && k < len(syms) {
			switch s := syms[k]; s {
			case -3, -1, 1, 3:
				want = float64(s) * 100 * scale
			}
		}
		if d := float64(got[n]) - float64(base[n]); math.Abs(d-want) > 1 {
			t.Fatalf("sample %d: the tap added %v, want %v", n, d, want)
		}
	}
}
