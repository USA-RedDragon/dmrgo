package rf

import (
	"slices"
	"testing"
)

func TestLeanAFCQuietTail(t *testing.T) {
	t.Parallel()
	conds := rxConds()
	c := conds[slices.IndexFunc(conds, func(c rxCond) bool { return c.name == "u8 dc cw 0.5 s" })]
	in := rxBuild(rxMS(t, 1), c, 40, false, rxSeed("ms"+c.name))
	lead := in.iq[:2*int(0.7*rxByteRate)]
	for _, tier := range []Tier{Tier0, Tier1} {
		r := mustNewOpt(RxConfig{Tier: tier, Rate: rxByteRate, Format: IQFormatCU8}, *halfOpt(ptr(tierOptions(tier))))
		for k := 0; k < len(lead); k += 1 << 14 {
			must(r.writeIQ(lead[k:min(k+1<<14, len(lead))]))
		}
		if hz := r.host().lean.tuneHz; hz < -leanSearchHz || hz > leanSearchHz {
			t.Errorf("Tier%d: the AFC moved %.0f Hz on the DC and CW lead", tier-Tier0, hz)
		}
	}
}
