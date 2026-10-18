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
