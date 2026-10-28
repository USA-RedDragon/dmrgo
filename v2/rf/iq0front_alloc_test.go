package rf

import "testing"

func TestAllocsIQ0(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	for _, front := range []int{3, 4, 5} {
		o := iq0Options(front, 2*symbolRate)
		r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(func(*Burst) {}), Format: IQFormatCU8}, o)
		const chunk = 4096
		must(r.writeBB(x[:chunk]))
		i := chunk
		if n := testing.AllocsPerRun(20, func() {
			must(r.writeBB(x[i : i+chunk]))
			i += chunk
		}); n != 0 {
			t.Errorf("front %d: %v allocations per WriteBaseband", front, n)
		}
	}
}
