package rf

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

const (
	carrierTailSeconds = 0.6
	carrierTailCN      = 25
)

func carrierTail(fm, env []float64, hz float64, seed uint64) []complex64 {
	n := len(fm) + int(carrierTailSeconds*basebandRate)
	x := make([]complex64, n)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, carrierTailCN/10.0)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		f, e := hz, 1.0
		if i < len(fm) {
			f, e = fm[i], env[i]
		}
		ph += 2 * math.Pi * f / basebandRate
		x[i] = complex(float32(e*math.Cos(ph)+sigma*rng.NormFloat64()), float32(e*math.Sin(ph)+sigma*rng.NormFloat64()))
	}
	return x
}

func TestRxCarrierAfterStream(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 1)
	fm, env := rftx.ModulateBS(slots)
	end := float64(len(fm)) / basebandRate
	seeds := map[Tier]uint64{Tier0: 64, Tier1: 64, Tier2: 64, Tier3: 8}
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		t.Run(strconv.Itoa(int(tier-Tier0)), func(t *testing.T) {
			t.Parallel()
			for seed := range seeds[tier] {
				hz := []float64{2000, 0, -2000}[seed%3]
				for _, b := range receiveBaseband(carrierTail(fm, env, hz, 300+seed), tier) {
					if b.Seconds > end {
						t.Errorf("seed %d, carrier at %+.0f Hz: burst at %.4f s after the stream ends at %.4f s", seed, hz, b.Seconds, end)
						break
					}
				}
			}
		})
	}
}
