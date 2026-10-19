package fsktx

import (
	"math/rand/v2"
	"testing"
)

func shapeSyncRun(t *testing.T, sync int, ageSyms int) []int16 {
	t.Helper()
	a, err := NewAudio(AudioConfig{Hz: 48000, Tier: Tier1, Level: 64, Offset: 128, Shape: 2,
		NoiseZerosHz: [4]float64{7000, 21000}, Corr: 5, CorrHiHz: 12000, ShapeSync: sync})
	if err != nil {
		t.Fatal(err)
	}
	var out []int16
	age := rand.New(rand.NewPCG(3, 4))
	for range ageSyms {
		out = a.AppendSymbol(out[:0], int8(age.IntN(4)*2-3))
	}
	out = out[:0]
	rng := rand.New(rand.NewPCG(5, 6))
	for range 4000 {
		out = a.AppendSymbol(out, int8(rng.IntN(4)*2-3))
	}
	return out
}

func TestAudioShapeSync(t *testing.T) {
	const age = 120 * SymbolRate
	from := 200 * 48000 / SymbolRate
	diff := func(a, b []int16) int {
		n := 0
		for i := from; i < len(a); i++ {
			if a[i] != b[i] {
				n++
			}
		}
		return n
	}
	if n := diff(shapeSyncRun(t, 0, 0), shapeSyncRun(t, 0, age)); n == 0 {
		t.Fatal("without ShapeSync an aged Audio gives the same codes; the test has no power")
	}
	for _, s := range []int{2, 3} {
		if n := diff(shapeSyncRun(t, s, 0), shapeSyncRun(t, s, age)); n != 0 {
			t.Fatalf("ShapeSync %d: %d codes differ between a fresh and an aged Audio", s, n)
		}
	}
}
