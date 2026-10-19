package rf

import (
	"encoding/hex"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestLeanGapNoise checks that the noise in the gaps between the
// transmissions of an MS does not anchor the stream. Without a carrier the
// discriminator noise matches a SYNC now and then with few clicks; such an
// anchor moved the slots and the levels of the stream, and the first burst
// of the next transmission was lost at 17 dB C/N.
func TestLeanGapNoise(t *testing.T) {
	cfg := testConfig()
	cfg.ColorCode, cfg.Wakeups, cfg.Slot, cfg.Preambles = 1, 0, 1, 2
	built, err := rftx.Sequence("preamble", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	want := map[string]int{}
	var freq, amp []float64
	jit := rand.New(rand.NewPCG(7, 8))
	const calls = 120
	for range calls {
		g := int(0.3*basebandRate) + jit.IntN(10)
		freq = append(freq, make([]float64, g)...)
		amp = append(amp, make([]float64, g)...)
		freq = append(freq, fm...)
		amp = append(amp, env...)
		for _, b := range built.Bursts {
			if x, err := layer2.NewBurstFromBytes(b); err == nil && x.Kind == layer2.KindData {
				p, _ := b.BPTCPayload()
				want[hex.EncodeToString(p[:])]++
			}
		}
	}
	for _, tier := range []Tier{Tier0, Tier1} {
		x := noisyFM(freq, amp, 0, 17, 24)
		got := map[string]int{}
		for _, b := range receiveBaseband(x, tier) {
			data, _ := b.Decoded()
			if y, err := layer2.NewBurstFromBytes(data); err == nil && y.Kind == layer2.KindData {
				p, _ := data.BPTCPayload()
				got[hex.EncodeToString(p[:])]++
			}
		}
		for k, n := range want {
			if got[k] < n {
				t.Errorf("Tier%d: %d of %d bursts of payload %s", tier-Tier0, got[k], n, k)
			}
		}
	}
}
