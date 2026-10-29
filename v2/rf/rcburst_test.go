package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRxStandaloneRC(t *testing.T) {
	t.Parallel()

	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2}
	built, err := rftx.Sequence("rc", cfg)
	if err != nil {
		t.Fatal(err)
	}
	rc := built.Bursts[len(built.Bursts)-1]
	freq, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	for _, cn := range []float64{20, 40} {
		for seed := range uint64(4) {
			x := noisyFM(freq, env, 1400, cn, 30+seed)
			for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
				var found int
				for _, b := range receiveBaseband(x, tier) {
					if b.Sync != enums.SyncMSSourcedRC {
						continue
					}
					found++
					if data, _ := b.Decoded(); data != rc {
						t.Errorf("C/N %.0f dB seed %d Tier%d: RC burst bits differ", cn, seed, tier-Tier0)
					}
				}
				if found != 1 {
					t.Errorf("C/N %.0f dB seed %d Tier%d: %d RC bursts, want 1", cn, seed, tier-Tier0, found)
				}
			}
		}
	}
}
