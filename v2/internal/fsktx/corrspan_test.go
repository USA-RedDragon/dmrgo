//go:build !tinygo

package fsktx

import "testing"

func TestCorrLongerSpanAligns(t *testing.T) {
	t.Parallel()
	const pad = 4
	run := func(p PulseConfig, corr int) []int16 {
		a, err := NewAudio(AudioConfig{Hz: 48000, Tier: Tier1, Pulse: p, Corr: corr, CorrHiHz: 12000, CorrTaps: 81, Corr48k: true})
		if err != nil {
			t.Fatal(err)
		}
		var out []int16
		for _, s := range newTestStream(t, 6, 40).syms {
			out = a.AppendSymbol(out, s)
		}
		return out
	}
	p := corrPulse()
	z := make([]float64, pad*p.PerSymbol)
	long := p
	long.Shape = append(append(append([]float64{}, z...), p.Shape...), z...)
	long.Span += pad
	lag := pad * 48000 / SymbolRate
	worst := func(corr int) int {
		want, got := run(p, corr), run(long, corr)
		var m int
		for i := range len(want) - lag {
			m = max(m, abs(int(got[i+lag])-int(want[i])))
		}
		return m
	}
	if base, with := worst(0), worst(5); with > base+1 {
		t.Fatalf("a longer pulse moves the output by %d codes with Corr, %d without", with, base)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
