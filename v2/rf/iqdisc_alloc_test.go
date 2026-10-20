package rf

import "testing"

func TestAllocsIQDisc(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	for _, on := range []func(*Burst){func(*Burst) {}, nil} {
		o := discOptions()
		o.IQDisc, o.SoftExt, o.ExtAMBE, o.ExtOSD, o.ExtELC, o.ExtLookback = true, true, true, true, true, 8
		r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: on})
		const chunk = 4096
		r.WriteBaseband(x[:chunk])
		i := chunk
		if n := testing.AllocsPerRun(20, func() {
			r.WriteBaseband(x[i : i+chunk])
			i += chunk
		}); n != 0 {
			t.Errorf("OnBurst %v: %v allocations per WriteBaseband", on != nil, n)
		}
	}
}
