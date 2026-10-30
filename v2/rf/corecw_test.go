//go:build !tinygo

package rf

import (
	"fmt"
	"slices"
	"testing"
)

func TestCoreTier3CarrierLead(t *testing.T) {
	t.Parallel()
	streams := rxStreams(t)
	conds := rxConds()
	for _, cn := range []float64{12, 40} {
		for _, name := range []string{"cw 0.5 s", "cw 2.0 s", "u8 dc cw 0.5 s"} {
			c := conds[slices.IndexFunc(conds, func(c rxCond) bool { return c.name == name })]
			if c.highCN && cn < 40 {
				continue
			}
			t.Run(fmt.Sprintf("%g dB/%s", cn, name), func(t *testing.T) {
				t.Parallel()
				in := rxBuild(streams["bs"], c, cn, false, rxSeed("bs"+c.name))
				engine := rxMatch(rxEligible(in), rxReceive(t, in, rxRecv{benchEntry: benchEntry{benchIQ, Tier3}}))
				r, err := newReceiver(RxConfig{Tier: Tier3, Rate: basebandRate, Format: IQFormatCU8}, ptr(coreIQ3Options()))
				if err != nil {
					t.Fatal(err)
				}
				var got []Burst
				if in.iq != nil {
					r, err = newReceiver(RxConfig{Tier: Tier3, Rate: rxByteRate, Format: IQFormatCU8}, ptr(coreIQ3Options()))
					if err != nil {
						t.Fatal(err)
					}
					for k := 0; k < len(in.iq); k += 1 << 14 {
						got = append(got, must(r.writeIQ(in.iq[k:min(k+1<<14, len(in.iq))]))...)
					}
				} else {
					for k := 0; k < len(in.x); k += 4096 {
						got = append(got, must(r.writeBB(in.x[k:min(k+4096, len(in.x))]))...)
					}
				}
				got = append(got, r.flushAll()...)
				core := rxMatch(rxEligible(in), got)
				if core.exact < engine.exact {
					t.Errorf("core Tier 3: %d of %d payloads, engine %d", core.exact, core.total, engine.exact)
				}
			})
		}
	}
}
