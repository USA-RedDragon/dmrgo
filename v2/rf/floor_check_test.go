package rf

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestFloorDelta(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 1)
	slots = slots[:12]
	f := newFloorSignal(slots, 3, 5)
	x := noisyBaseband(slots, 1400, 3, 5)
	for i := range x {
		if cmplx.Abs(complex128(x[i])-f.y[i]*cmplx.Exp(complex(0, carrierPhase(i, len(x), 1400)))) > 1e-5 {
			t.Fatalf("sample %d differs", i)
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 20 {
		i := 2 + rng.IntN(len(slots)-4)
		mod := append([]rftx.BSSlot(nil), slots...)
		c := mod[i].Burst
		var ch []floorChange
		for range 1 + rng.IntN(6) {
			p := rng.IntN(264)
			c[p/8] ^= 0x80 >> (p % 8)
		}
		ch = f.burstChanges(ch, i, &c)
		mod[i].Burst = c
		got := f.delta(ch, true)
		gotB := f.delta(ch, false)
		want := directDelta(f, x, slots, mod)
		t.Logf("trial %d: %d changes, delta %.4f, direct %.4f, no tail %.4f", trial, len(ch), got, want, gotB)
		if math.Abs(got-want) > 1e-6*math.Max(1, math.Abs(want)) {
			t.Errorf("trial %d: delta %v, direct %v", trial, got, want)
		}
	}
}

func carrierPhase(i, n int, offsetHz float64) float64 {
	pad := int(benchPad * basebandRate)
	k := i - pad
	if k < 0 {
		return 0
	}
	k = min(k, n-2*pad-1)
	return 2 * math.Pi * offsetHz * float64(k+1) / basebandRate
}

func directDelta(f *floorSignal, x []complex64, slots, mod []rftx.BSSlot) float64 {
	sig := func(s []rftx.BSSlot) []complex128 {
		fm, env := rftx.ModulateBS(s)
		pad := int(benchPad * basebandRate)
		out := make([]complex128, len(x))
		var ph float64
		for k := range fm {
			ph += 2 * math.Pi * (fm[k] + 1400) / basebandRate
			out[k+pad] = complex(env[k]*math.Cos(ph), env[k]*math.Sin(ph))
		}
		return out
	}
	a, b := sig(slots), sig(mod)
	var d float64
	for i, v := range x {
		y := complex128(v)
		d += real(cmplx.Conj(y)*b[i]) - real(cmplx.Conj(y)*a[i])
	}
	return d * f.scale
}

func TestFloorNoiseless(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	e := newFloorEval(1)
	e.f = newFloorSignal(slots, 14, 1)
	e.slots, e.truth = slots, truth
	e.stream()
	for k, s := range e.sc {
		t.Logf("%s: %d of %d", k, s.Exact, s.Truth)
		if s.Exact != s.Truth || s.Truth == 0 {
			t.Errorf("%s: %d of %d", k, s.Exact, s.Truth)
		}
	}
}
