package rf

import "testing"

func TestAllocsMaxBursts(t *testing.T) {
	for _, cfg := range []Config{{}, {Tier: Tier1}, {Tier: Tier2}, {Tier: Tier3}, {Tier: Tier1, BasebandHz: BasebandRate / 2}, {OnBurst: func(*Burst) {}}} {
		r := mustNew(cfg)
		if a := testing.AllocsPerRun(10, func() { _ = r.MaxBursts(1 << 20) }); a != 0 {
			t.Errorf("%s: MaxBursts allocates %v times", rxEngine(r), a)
		}
	}
}
