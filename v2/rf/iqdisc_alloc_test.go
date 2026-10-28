package rf

import "testing"

func TestAllocsIQDisc(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	for _, on := range []func(*Burst){func(*Burst) {}, nil} {
		o := discOptions()
		o.IQDisc, o.SoftExt, o.ExtAMBE, o.ExtOSD, o.ExtELC, o.ExtLookback = true, true, true, true, true, 8
		r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(on), Format: IQFormatCU8}, o)
		const chunk = 4096
		must(r.writeBB(x[:chunk]))
		i := chunk
		if n := testing.AllocsPerRun(20, func() {
			must(r.writeBB(x[i : i+chunk]))
			i += chunk
		}); n != 0 {
			t.Errorf("OnBurst %v: %v allocations per WriteBaseband", on != nil, n)
		}
	}
}
