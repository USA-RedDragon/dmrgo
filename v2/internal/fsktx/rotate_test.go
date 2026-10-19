package fsktx

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

func rotatePair(t testing.TB, tier Tier, rate int, off float64) (*IQ, *IQ) {
	c := IQConfig{Rate: rate, OffsetHz: off, Tier: tier, Exact: true, Delay: 8, ShapePred: true,
		Pulse: PulseConfig{Window: 3, Beta: 17, Flat: true}}
	if tier == Tier1 {
		c.Pulse.Span, c.Delay = 16, 16
	}
	a, err := NewIQ(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Rotate = true
	b, err := NewIQ(c)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

// TestRotateMatches checks that Rotate gives the I/Q of Tier2 within a
// rounding step, keyed up and down.
func TestRotateMatches(t *testing.T) {
	for _, tier := range []Tier{Tier1, Tier2} {
		for _, rate := range []int{48000, 96000, 192000, 1000000, 2400000} {
			for _, off := range []float64{0, 1200, -2500, float64(rate)/4 - 100} {
				a, b := rotatePair(t, tier, rate, off)
				r := rand.New(rand.NewPCG(1, 2))
				var x, y []int16
				worst := 0
				for i := range 4000 {
					if i%300 == 10 {
						a.Key(true)
						b.Key(true)
					}
					if i%300 == 250 {
						a.Key(false)
						b.Key(false)
					}
					s := int8(2*r.IntN(4) - 3)
					x = a.AppendS16(x[:0], s)
					y = b.AppendS16(y[:0], s)
					for k := range x {
						worst = max(worst, absInt(int(x[k])-int(y[k])))
					}
				}
				if worst > 1 {
					t.Errorf("tier %d rate %d offset %v: worst difference %d", tier, rate, off, worst)
				}
			}
		}
	}
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func BenchmarkRotate(b *testing.B) {
	for _, rate := range []int{48000, 96000, 2400000} {
		for _, rot := range []bool{false, true} {
			for _, tier := range []Tier{Tier1, Tier2} {
				q, err := NewIQ(IQConfig{Rate: rate, Tier: tier, Exact: true, Rotate: rot})
				if err != nil {
					b.Fatal(err)
				}
				q.Key(true)
				var dst []int16
				name := fmt.Sprint(rate, map[bool]string{false: "old", true: "rot"}[rot], map[Tier]string{Tier1: "1", Tier2: "2"}[tier])
				b.Run(name, func(b *testing.B) {
					for i := 0; b.Loop(); i++ {
						dst = q.AppendS16(dst[:0], int8(2*(i&3)-3))
					}
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)*4800/1e6, "ms/s")
				})
			}
		}
	}
}
