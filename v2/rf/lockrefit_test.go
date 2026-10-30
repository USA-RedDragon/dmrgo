//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestLockRefitScale(t *testing.T) {
	t.Parallel()
	var syms [syncSymbols]int8
	syncSyms(enums.SyncBSSourcedData, &syms)
	const g, dc = 0.42, 37.0
	c := &leanCore{ns: 500}
	end := c.ns - 1
	j := ringPos(end - syncSymbols + 1)
	for k := range syncSymbols {
		c.ring[j] = int8(math.Round((dc + g*deviationHz*float64(syms[k])) / discStepHz))
		if j++; j == leanRing {
			j = 0
		}
	}
	c.dcHz, c.devScale = dc, 0.3
	x := &softExt{vP: enums.SyncBSSourcedData, vEnd: end}
	x.refitScale(c)
	if math.Abs(float64(c.devScale)-g) > 0.01 || c.tm.dev != c.devScale*deviationHz {
		t.Errorf("refit scale %.4f (timing %.1f), want %.2f", c.devScale, c.tm.dev, g)
	}
	x.vEnd = c.ns - leanRing
	c.devScale = 0.3
	x.refitScale(c)
	if c.devScale != 0.3 {
		t.Errorf("refit from an overwritten ring: scale %.4f, want it unchanged", c.devScale)
	}
}
