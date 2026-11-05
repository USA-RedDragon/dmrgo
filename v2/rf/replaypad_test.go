//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestRxReplayNoisePad checks that the late-entry replay of Tiers 1 to 3
// gives no burst from the noise before an MS voice call at 30 dB, on the
// starts and seeds of TestSecondsLockBurstExact where it once did.
func TestRxReplayNoisePad(t *testing.T) {
	t.Parallel()
	cfg := testConfig(t)
	cfg.Superframes = 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	ramp := (slotSeconds - burstSeconds) / 2
	for _, k := range []int{87, 192, 250, 292, 353} {
		start := benchPad + float64(k%16)*0.00037
		x := signalBaseband([]signalPart{{start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, 30, uint64(k+1))
		first := math.Floor(start*basebandRate)/basebandRate + ramp
		for tier := Tier1; tier <= Tier3; tier++ {
			for _, b := range receiveIQAt(t, x, basebandRate, tier) {
				if b.Seconds < first-0.001 {
					t.Errorf("start %d Tier%d: a burst at %.4f s, %.1f ms before the call", k, tier-Tier0, b.Seconds, (first-b.Seconds)*1e3)
				}
			}
		}
	}
}
