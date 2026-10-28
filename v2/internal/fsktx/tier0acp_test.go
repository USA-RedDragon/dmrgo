//go:build !tinygo

package fsktx

import (
	"fmt"
	"testing"
)

// ETSI EN 300 113 clause 7.4
func TestTier0AdjacentChannel(t *testing.T) {
	t.Parallel()
	points := make([]limPoint, 0, 5)
	points = append(points, limPoint{u: txUnit{mcu: true, tier: Tier0, format: fmtS16, rate: 96000, ramp: 96}})
	for _, hz := range []int{24000, 48000} {
		for _, r := range []limRadio{radioPlain, radioIdeal} {
			points = append(points, limPoint{u: txUnit{mcu: true, tier: Tier0, format: fmtAudio, rate: hz, level: tableUnits}, bits: 16, radio: r})
		}
	}
	for _, p := range points {
		t.Run(fmt.Sprint(p), func(t *testing.T) {
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
