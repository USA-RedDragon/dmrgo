package rf

import "testing"

func TestAllocsMaxBursts(t *testing.T) {
	for _, cfg := range []RxConfig{{Tier: Tier0}, {Tier: Tier1}, {Tier: Tier2}, {Tier: Tier3}, {Tier: Tier1, Rate: discRate}, {OnBursts: func([]Burst) {}}} {
		r := mustNew(cfg)
		if a := testing.AllocsPerRun(10, func() { _ = r.MaxOutput(1 << 20) }); a != 0 {
			t.Errorf("%s: MaxOutput allocates %v times", rxEngine(r), a)
		}
	}
}
