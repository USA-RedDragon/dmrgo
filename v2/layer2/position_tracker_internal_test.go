package layer2

import (
	"math"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func nearestLostLoop(gap, est int) int {
	if gap > est+2 {
		return est
	}
	n := gap
	for n+VoiceBurstsPerSuperframe/2 < est {
		n += VoiceBurstsPerSuperframe
	}
	return n
}

func TestNearestLostClosedForm(t *testing.T) {
	t.Parallel()

	for gap := 0; gap < VoiceBurstsPerSuperframe; gap++ {
		for est := -2; est < 5000; est++ {
			if got, want := nearestLost(gap, est), nearestLostLoop(gap, est); got != want {
				t.Fatalf("nearestLost(%d, %d) = %d, want %d", gap, est, got, want)
			}
		}
	}
	if got := nearestLost(1, math.MaxInt-3); got < 0 {
		t.Fatalf("nearestLost overflowed: %d", got)
	}
}

func TestPlaceDecodedHugeSteps(t *testing.T) {
	t.Parallel()

	var tr PositionTracker
	b := NewBurst()
	b.Kind, b.VoiceBurst = KindVoice, enums.VoiceBurstA
	h := Hint{}
	tr.PlaceDecoded(b, &h, 1)
	b.VoiceBurst = enums.VoiceBurstUnknown
	b.EmbeddedSignalling.FEC.Uncorrectable = true
	start := time.Now()
	for _, steps := range []int{maxPlaceSteps + 1, 1 << 30, math.MaxInt} {
		h = Hint{}
		lost, p := tr.PlaceDecoded(b, &h, steps)
		if lost != steps-1 || p != PlacementInOrder || h.VoiceBurst != enums.VoiceBurstUnknown {
			t.Fatalf("steps %d: lost %d placement %v burst %v", steps, lost, p, h.VoiceBurst)
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("huge steps took %v", d)
	}
}
