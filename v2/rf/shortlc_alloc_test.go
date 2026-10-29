//go:build !tinygo

package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
)

func TestAllocsAudioShortLC(t *testing.T) {
	slots, _ := benchStream(t, 4)
	a := discAudio(noisyBaseband(slots, 1400, 20, 1), benchAudioHz)
	const chunk = 2048
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		lcs := 0
		r := mustAudio(AudioRxConfig{Rate: benchAudioHz, Tier: tier, OnBursts: func([]Burst) {}, OnShortLC: func(l []ShortLC) { lcs += len(l) }})
		pos := 0
		n := allocstest.PerRun(t, len(a)/chunk-2, func() {
			_, _ = r.Write(a[pos : pos+chunk])
			pos += chunk
		})
		_ = r.Flush()
		if n != 0 || lcs == 0 {
			t.Errorf("tier %d: %v allocations per Write, %d Short LCs", tier, n, lcs)
		}
	}
}
