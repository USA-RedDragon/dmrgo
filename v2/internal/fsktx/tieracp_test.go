//go:build !tinygo

package fsktx

import (
	"fmt"
	"math"
	"testing"
)

// ETSI EN 300 113 clause 7.4
func TestTierAdjacentChannel(t *testing.T) {
	t.Parallel()
	points := make([]limPoint, 0, 8)
	for _, tier := range []Tier{Tier1, Tier2} {
		for _, hz := range []int{96000, 2400000} {
			points = append(points, limPoint{u: txUnit{tier: tier, format: fmtS16, rate: hz, ramp: hz / 1000}})
		}
		for _, r := range []limRadio{radioPlain, radioIdeal} {
			points = append(points, limPoint{u: txUnit{tier: tier, format: fmtAudio, rate: 48000, level: tableUnits}, bits: 16, radio: r})
		}
	}
	for _, p := range points {
		t.Run(fmt.Sprint(p, "/T", p.u.tier-Tier0), func(t *testing.T) {
			t.Parallel()
			g := p.measure(t, 1)
			for _, k := range []string{"en_acp_annexc_nominal_db", "en_acp_annexc_wide_db", "en_acp_bins_db"} {
				if g[k] < limNeed(k) {
					t.Errorf("%s: margin %+.2f dB, need %+.2f", k, g[k], limNeed(k))
				}
			}
		})
	}
}

func TestTierDesignFits(t *testing.T) {
	t.Parallel()
	ts := newTestStream(t, 2, 40)
	for _, tier := range []Tier{Tier1, Tier2} {
		for _, hz := range []int{24000, 44100, 48000, 96000} {
			t.Run(fmt.Sprint(tier-Tier0, "/", hz), func(t *testing.T) {
				t.Parallel()
				lv := 16000
				for ; lv > 0; lv -= 10 {
					if _, err := NewAudio(AudioConfig{Hz: hz, Tier: tier, Level: lv}); err == nil {
						break
					}
				}
				if lv < 15000 {
					t.Fatalf("Tier%d at %d Hz: Level %d is the most that fits", tier-Tier0, hz, lv)
				}
				u := txUnit{tier: tier, format: fmtAudio, rate: hz, level: lv}
				for _, syms := range [][]int8{ts.syms, u.pattern(patWorst, 300)} {
					for _, v := range u.run(t, syms, nil) {
						if v == math.MaxInt16 || v == math.MinInt16 {
							t.Fatalf("Tier%d at %d Hz, Level %d: the output reaches %d", tier-Tier0, hz, lv, v)
						}
					}
				}
			})
		}
	}
}
