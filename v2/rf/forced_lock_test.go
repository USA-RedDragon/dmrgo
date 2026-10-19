package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestForcedLockHeader checks that Tier2 decodes the voice LC header of a
// call after noise. The coherent scan forces a lock at the carrier start,
// in the header before its SYNC, and the lock ends unanchored at the next
// slot. The core kept no samples while it was locked, so that the search
// could not sample the header again and found the call one burst late.
func TestForcedLockHeader(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Wakeups, cfg.Slot = 1, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	g := int(0.3 * basebandRate)
	freq := append(make([]float64, g), fm...)
	amp := append(make([]float64, g), env...)
	lost := 0
	const seeds = 16
	for seed := range uint64(seeds) {
		x := noisyFM(freq, amp, 0, 17, 100+seed)
		ok := false
		for _, b := range receiveBaseband(x, Tier2) {
			data, _ := b.Decoded()
			if y, err := layer2.NewBurstFromBytes(data); err == nil && y.Kind == layer2.KindData && y.SlotType.DataType == enums.DataTypeVoiceLCHeader {
				ok = true
			}
		}
		if !ok {
			lost++
		}
	}
	if lost > 0 {
		t.Errorf("voice LC header lost in %d of %d runs", lost, seeds)
	}
}
