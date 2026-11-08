package rf

import (
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

const tier1VoiceDraws = 8

// TestTier1Voice receives a BS stream at 8.2 dB Eb/N0, below the 90% point
// of whole voice frames of Tier0, as discriminator audio at 9.6 kHz and as
// I/Q, and checks that Tier1, which detects each burst with the
// correlation of the noise of its symbols, recovers more whole voice
// frames than Tier0: their C2 and C3 bits have no FEC. It pools 8 draws of
// payload and noise. Over 200 single draws of 180 frames the gain on I/Q
// had a mean of 0.144 and a standard deviation of 0.032, and 23 of them fell
// under the 0.1 that Tier1 must gain; the mean of 8 draws has a deviation
// of about 0.011.
func TestTier1Voice(t *testing.T) {
	t.Parallel()
	logRepro(t)
	draw := runMix(testseed.RunSeed(), nameHash(t.Name()))
	var mu sync.Mutex
	var exact, total [4]int
	forSeeds(tier1VoiceDraws, func(k int) {
		p := pinnedTB{t, runMix(draw, uint64(k))}
		slots, truth := benchStream(p, 10)
		x := noisyBaseband(p, slots, 1400, 7, 3)
		a := discAudio(x, 9600)
		got := [4][]Burst{receiveDisc(t, a, 9600, Tier0), receiveDisc(t, a, 9600, Tier1), receiveBaseband(x, Tier0), receiveBaseband(x, Tier1)}
		mu.Lock()
		defer mu.Unlock()
		for i, g := range got {
			s := scoreBench(truth, g)["ambe"]
			exact[i] += s.Exact
			total[i] += s.Truth
		}
	})
	var rate [4]float64
	for i := range rate {
		rate[i] = float64(exact[i]) / float64(max(total[i], 1))
	}
	a0, a1, q0, q1 := rate[0], rate[1], rate[2], rate[3]
	t.Logf("whole voice frames: audio %.3f Tier0, %.3f Tier1; I/Q %.3f Tier0, %.3f Tier1", a0, a1, q0, q1)
	if a1 < a0+0.1 || q1 < q0+0.1 {
		t.Errorf("whole voice frames: audio %.3f Tier0, %.3f Tier1; I/Q %.3f Tier0, %.3f Tier1", a0, a1, q0, q1)
	}
}
