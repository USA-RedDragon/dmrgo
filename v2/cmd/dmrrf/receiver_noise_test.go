package main

import "testing"

// TestReceiverNoiseOnly feeds a minute of white noise to TierDisc and
// TierCoherent and checks that neither reports a burst.
func TestReceiverNoiseOnly(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{TierDisc, TierCoherent} {
		r := NewReceiver(ReceiverConfig{Rate: basebandRate, Tier: tier})
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
