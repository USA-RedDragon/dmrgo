package rf

import "testing"

// TestAllocsIQ0 checks that the lean front allocates nothing per
// WriteBaseband.
func TestAllocsIQ0(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	o := iq0Options(3, 2*symbolRate)
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(*Burst) {}})
	const chunk = 4096
	r.WriteBaseband(x[:chunk])
	i := chunk
	if n := testing.AllocsPerRun(20, func() {
		r.WriteBaseband(x[i : i+chunk])
		i += chunk
	}); n != 0 {
		t.Errorf("%v allocations per WriteBaseband", n)
	}
}
