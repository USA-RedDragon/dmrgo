//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestLeanClockRateApplied(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE", Superframes: 30}
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	tx := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	for _, c := range []struct{ ppm, cn float64 }{{50, 25}, {-50, 25}, {-67, 25}, {-67, 0}} {
		ppm := c.ppm
		iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, 0, c.cn, 5)
		for _, tier := range []Tier{Tier0, Tier2} {
			r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: tier, Format: IQFormatCU8})
			if err != nil {
				t.Fatal(err)
			}
			h := r.host()
			locked, applied := 0, 0
			step := 1 << 16
			for k := 0; k < len(iq); k += step {
				r.AppendBursts(nil, iq[k:min(k+step, len(iq))])
				var tm *leanTiming
				var on bool
				switch {
				case h.iqd != nil:
					tm, on = &h.iqd.d.tm, h.iqd.d.locked
				case h.iq0 != nil:
					tm, on = &h.iq0.d.tm, h.iq0.d.locked
				}
				if tm == nil || !on || k < len(iq)/3 {
					continue
				}
				locked++
				if got := float64(tm.drift/(2*tm.step)) * 1e6; math.Abs(got-ppm) < math.Abs(ppm)/2 {
					applied++
				}
			}
			if locked == 0 || float64(applied) < 0.9*float64(locked) {
				t.Errorf("%+.0f ppm at %.0f dB, Tier %d: a clock rate within half of the true one applied in %d of %d locked chunks", ppm, c.cn, tier-Tier0, applied, locked)
			}
		}
	}
}
