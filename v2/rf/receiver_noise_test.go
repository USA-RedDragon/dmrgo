package rf

import "testing"

// TestReceiverNoiseOnly feeds a minute of white noise to Tier0, Tier1
// and Tier2 and checks that none reports a burst.
func TestReceiverNoiseOnly(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		r := mustNew(Config{Rate: basebandRate, Tier: tier})
		n := 0
		for seed := uint64(1); seed <= 100; seed++ {
			n += len(r.WriteBaseband(noisyBaseband(nil, 0, 0, seed)))
		}
		n += len(r.Flush())
		if n != 0 {
			t.Errorf("tier %d: %d bursts from noise", tier, n)
		}
	}
}

// TestLookbackNoise checks that the slots before a lock that Tier2
// decodes again give no burst from the noise before the first
// transmission, while the first burst of the transmission stays. At 18 dB
// in a 50 Hz fade, the coherent stage forced a lock on the noise before
// the stream of seeds 129 and 134, whose levels fit nothing; their slots
// before it held a burst A by its SYNC distance, and the bursts that place
// by it.
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
		// The first burst of msStream ends its first slot.
		if first == 0 || first > benchPad+slotSeconds+0.01 {
			t.Errorf("seed %d: first burst at %.3f s", seed, first)
		}
	}
}
