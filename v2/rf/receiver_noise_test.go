package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
)

func TestReceiverNoiseOnly(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCU8})
		n := 0
		for seed := uint64(1); seed <= uint64(testmode.N(100, 100, 10)); seed++ {
			n += len(must(r.writeBB(noisyBaseband(nil, 0, 0, seed))))
		}
		n += len(r.flushAll())
		if n != 0 {
			t.Errorf("tier %d: %d bursts from noise", tier, n)
		}
	}
}

func TestLookbackNoise(t *testing.T) {
	t.Parallel()
	fm, env, _ := msStream(t, 3, 0.37)
	for _, seed := range []uint64{129, 134} {
		x := impairedFM(fm, env, 1400, 0, 50, 18, seed)
		n, first := 0, 0.0
		for _, b := range receiveBaseband(x, Tier2) {
			switch {
			case b.Seconds < benchPad:
				n++
			case first == 0 || b.Seconds < first:
				first = b.Seconds
			}
		}
		if n != 0 {
			t.Errorf("seed %d: %d bursts from the noise before the stream", seed, n)
		}
		if first == 0 || first > benchPad+slotSeconds+0.01 {
			t.Errorf("seed %d: first burst at %.3f s", seed, first)
		}
	}
}
