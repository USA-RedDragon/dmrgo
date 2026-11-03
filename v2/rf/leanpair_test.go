package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestLeanPairLock checks that Tier 0 locks a weak BS carrier that starts
// mid-stream on two syncs a whole number of slots apart. At 2 dB C/N few
// syncs reach the single-sync lock gate, so without the pair a whole stream
// can pass undecoded. It fails when more than 2 of the 16 seeds decode no
// burst.
func TestLeanPairLock(t *testing.T) {
	t.Parallel()
	const seeds = 16
	slots, _ := benchStream(t, 4)
	fm, env := rftx.ModulateBS(slots)
	cut := int(0.3 * basebandRate)
	tx := rftx.ToIQ(fm[cut:], env[cut:], 2e6, -320, 0.9)
	got := make([]int, seeds)
	forSeeds(seeds, func(i int) {
		x := rftx.Channel(tx, 2e6, 2e6, 0, 2-22.1, uint64(i+1))
		r := mustNew(RxConfig{Rate: 2e6, Tier: Tier0, Format: IQFormatCU8})
		got[i] = len(r.AppendFlush(r.AppendBursts(nil, x)))
	})
	none := 0
	for _, n := range got {
		if n == 0 {
			none++
		}
	}
	if none > 2 {
		t.Errorf("%d of %d seeds decode no burst: %v", none, seeds, got)
	}
}
