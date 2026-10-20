package rf

import "testing"

func TestTier1Voice(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 7, 3)
	a := discAudio(x, 9600)
	rate := func(got []Burst) float64 {
		s := scoreBench(truth, got)["ambe"]
		return float64(s.Exact) / float64(max(s.Truth, 1))
	}
	a0, a1 := rate(receiveDisc(t, a, 9600, Tier0)), rate(receiveDisc(t, a, 9600, Tier1))
	q0, q1 := rate(receiveBaseband(x, Tier0)), rate(receiveBaseband(x, Tier1))
	t.Logf("whole voice frames: audio %.2f Tier0, %.2f Tier1; I/Q %.2f Tier0, %.2f Tier1", a0, a1, q0, q1)
	if a1 < a0+0.1 || q1 < q0+0.1 {
		t.Errorf("whole voice frames: audio %.2f Tier0, %.2f Tier1; I/Q %.2f Tier0, %.2f Tier1", a0, a1, q0, q1)
	}
}
