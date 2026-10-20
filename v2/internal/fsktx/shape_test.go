//go:build !tinygo

package fsktx

import (
	"math/rand/v2"
	"testing"
)

func TestShapersAgree(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	syms := make([]int8, 4000)
	for i := range syms {
		syms[i] = int8(2*rng.IntN(4) - 3)
		if i%500 < 7 || i%777 == 3 {
			syms[i] = 0
		}
	}
	pu := pulse{span: 4, win: winKaiser, beta: 4}
	for _, hz := range []int{9600, 24000, 48000} {
		ref := newPolyF64(pu, hz, sweepGain)
		cases := map[string]shaper{
			"poly32":  newPolyF32(pu, hz, sweepGain),
			"polyq15": newPolyQ15(pu, hz, sweepGain, 14),
			"lut g1":  newLUTQ15(pu, hz, 1, false),
			"lut g2h": newLUTQ15(pu, hz, 2, true),
			"lut g4":  newLUTQ15(pu, hz, 4, false),
			"lut8":    newLUT8(pu, hz/SymbolRate, sweepGain),
		}
		var want []int16
		for _, s := range syms {
			want = ref.appendSymbol(want, s)
		}
		for name, k := range cases {
			var got []int16
			for _, s := range syms {
				got = k.appendSymbol(got, s)
			}
			if len(got) != len(want) {
				t.Fatalf("%s at %d Hz: %d samples, want %d", name, hz, len(got), len(want))
			}
			for i := range got {
				if d := int(got[i]) - int(want[i]); d < -3 || d > 3 {
					t.Fatalf("%s at %d Hz: sample %d is %d, want %d", name, hz, i, got[i], want[i])
				}
			}
		}
	}
}
