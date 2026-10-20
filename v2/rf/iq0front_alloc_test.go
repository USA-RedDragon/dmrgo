package rf

import "testing"

func TestAllocsIQ0(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	for _, front := range []int{3, 4, 5} {
		o := iq0Options(front, 2*symbolRate)
		r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(*Burst) {}})
		const chunk = 4096
		r.WriteBaseband(x[:chunk])
		i := chunk
		if n := testing.AllocsPerRun(20, func() {
			r.WriteBaseband(x[i : i+chunk])
			i += chunk
		}); n != 0 {
			t.Errorf("front %d: %v allocations per WriteBaseband", front, n)
		}
	}
}
