package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func edgeWindow(syms *[syncSymbols]int8, at, sps, amp, sigma float64, rng *rand.Rand) syncWindow {
	const pad = 8
	var levels [syncSymbols + 2*pad]float64
	for i := range levels {
		levels[i] = float64(2*rng.IntN(4) - 3)
	}
	for i, s := range syms {
		levels[pad+i] = float64(s)
	}
	var win syncWindow
	for k := range win.w {
		t := -float64(k)
		var v float64
		for i, l := range levels {
			x := (t - (at - float64(syncSymbols-1+pad-i)*sps)) / sps
			v += l * raisedCosine(x, 0.2)
		}
		v = amp*v + sigma*rng.NormFloat64()
		win.w[k] = int8(max(min(math.Round(v), math.MaxInt8), math.MinInt8))
	}
	return win
}

func raisedCosine(x, beta float64) float64 {
	if x == 0 {
		return 1
	}
	if d := 1 - 4*beta*beta*x*x; math.Abs(d) < 1e-9 {
		return math.Pi / 4 * math.Sin(math.Pi*x) / (math.Pi * x)
	}
	return math.Sin(math.Pi*x) / (math.Pi * x) * math.Cos(math.Pi*beta*x) / (1 - 4*beta*beta*x*x)
}

func TestSyncPeakBeyondEdge(t *testing.T) {
	t.Parallel()
	var syms [syncSymbols]int8
	syncSyms(enums.SyncBSSourcedData, &syms)
	const sps, end = 2, -4
	for seed := range uint64(20) {
		rng := rand.New(rand.NewPCG(seed, 1))
		for _, late := range []float64{0.75, 0.8, -0.75, -0.8} {
			win := edgeWindow(&syms, end+late*sps, sps, 8, 0.5, rng)
			var c [7]float32
			got := win.peak(&syms, &c, end, sps, false)
			if c[5] < c[4] && late > 0 || c[1] < c[2] && late < 0 {
				t.Fatalf("seed %d late %.2f: the edge is not the best of the searched offsets: %v", seed, late, c)
			}
			want := 6
			if late < 0 {
				want = 0
			}
			if got != want {
				t.Errorf("seed %d, SYNC %.2f symbol off: offset %d, want %d (c %v)", seed, late, got, want, c)
			}
		}
	}
}

func TestSyncPeakEdgeNoise(t *testing.T) {
	t.Parallel()
	var syms [syncSymbols]int8
	syncSyms(enums.SyncBSSourcedData, &syms)
	const sps, end = 2, -4
	moved, edges := 0, 0
	for seed := range uint64(4000) {
		rng := rand.New(rand.NewPCG(seed, 2))
		amp, late := 8.0, 0.5
		switch seed % 4 {
		case 1:
			late = -0.5
		case 2:
			amp, late = 0, 0
		case 3:
			amp, late = 3, 0.45
		}
		win := edgeWindow(&syms, end+late*sps, sps, amp, 8, rng)
		var c [7]float32
		got := win.peak(&syms, &c, end, sps, false)
		if got == 0 || got == 6 {
			moved++
		}
		if c[0] == float32(math.Inf(-1)) && c[6] == float32(math.Inf(-1)) && (got == 1 || got == 5) {
			edges++
		}
	}
	if moved > 0 {
		t.Errorf("the noise moved %d of 4000 SYNC peaks beyond the edge", moved)
	}
	if edges < 1000 {
		t.Errorf("only %d of 4000 peaks at the edge: the test does not test the edge", edges)
	}
}
