//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"
)

func acCoupled(a []int16, hz int) []int16 {
	type biquad struct{ b0, b1, b2, a1, a2, z1, z2 float64 }
	qs := make([]biquad, 0, 4)
	for k := range 4 {
		q := 1 / (2 * math.Cos(math.Pi*float64(2*k+1)/16))
		w := 2 * math.Pi * 300 / float64(hz)
		c, al := math.Cos(w), math.Sin(w)/(2*q)
		a0 := 1 + al
		qs = append(qs, biquad{b0: (1 + c) / 2 / a0, b1: -(1 + c) / a0, b2: (1 + c) / 2 / a0, a1: -2 * c / a0, a2: (1 - al) / a0})
	}
	out := make([]int16, len(a))
	for i, v := range a {
		x := float64(v)
		for j := range qs {
			q := &qs[j]
			y := q.b0*x + q.z1
			q.z1 = q.b1*x - q.a1*y + q.z2
			q.z2 = q.b2*x - q.a2*y
			x = y
		}
		out[i] = int16(max(min(math.Round(x), math.MaxInt16), math.MinInt16))
	}
	return out
}

func TestDiscACCoupled(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 16)
	x := noisyBaseband(slots, 1400, 30, 1)
	for _, hz := range []int{24000, 12000, 9600} {
		a := acCoupled(discAudio(x, hz), hz)
		for _, tier := range []Tier{Tier1, Tier2} {
			base := scoreBench(truth, receivePlain(t, a, hz, tier))
			got, _ := receiveFixed(t, a, hz, tier)
			sc := scoreBench(truth, got)
			if b := base["csbk"]; b.Exact > b.Truth/10 {
				t.Errorf("%d Hz %v: %d of %d CSBKs without the restorer, so the audio is not hard enough", hz, tier, b.Exact, b.Truth)
			}
			for _, k := range []string{"csbk", "rate12", "ambe"} {
				if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth/2 {
					t.Errorf("%d Hz %v, %s: %d of %d recovered from AC-coupled audio", hz, tier, k, s.Exact, s.Truth)
				}
			}
		}
	}
}

func TestDiscACStaysOff(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	rng := rand.New(rand.NewPCG(7, 9))
	noise := make([]complex64, 48000*2)
	for i := range noise {
		noise[i] = complex(float32(rng.NormFloat64()), float32(rng.NormFloat64()))
	}
	for _, hz := range []int{24000, 12000, 9600} {
		for _, in := range [][]complex64{noisyBaseband(slots, 1400, 30, 1), noisyBaseband(slots, -900, 3, 2), noise} {
			a := discAudio(in, hz)
			want := receivePlain(t, a, hz, Tier1)
			got, r := receiveFixed(t, a, hz, Tier1)
			if ac := r.acOf(); ac == nil || ac.det.on || ac.k != 0 || r.inverted() {
				t.Errorf("%d Hz: the restorer or the polarity check engaged on DC-coupled audio", hz)
			}
			if len(got) != len(want) {
				t.Fatalf("%d Hz: %d bursts with the restorer, %d without", hz, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("%d Hz: burst %d differs with the restorer", hz, i)
				}
			}
		}
	}
}
