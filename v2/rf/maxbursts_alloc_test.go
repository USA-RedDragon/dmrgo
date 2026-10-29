package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
)

func TestAllocsMaxBursts(t *testing.T) {
	for _, cfg := range []RxConfig{{Tier: Tier0}, {Tier: Tier1}, {Tier: Tier2}, {Tier: Tier3}, {Tier: Tier1, Rate: discRate}, {OnBursts: func([]Burst) {}}} {
		r := mustNew(cfg)
		if a := allocstest.PerRun(t, 10, func() { _ = r.MaxOutput(1 << 20) }); a != 0 {
			t.Errorf("%s: MaxOutput allocates %v times", rxEngine(r), a)
		}
	}
}
