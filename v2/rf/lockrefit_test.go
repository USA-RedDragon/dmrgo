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

func TestLevelsSkipClickedSymbols(t *testing.T) {
	t.Parallel()
	var syms [syncSymbols]int8
	syncSyms(enums.SyncBSSourcedVoice, &syms)
	const g, dc = 0.6, 120.0
	for _, c := range []struct {
		clicks int
		spread float64
		refit  bool
	}{
		{0, 0, true}, {1, 0, true}, {leanSyncClipMax, 0, true}, {leanSyncClipMax + 1, 0, false},
		{1, 300, true},
	} {
		r := &leanCore{ns: 500}
		end := r.ns - 1
		j := ringPos(end - syncSymbols + 1)
		var n, sp, spp, sx, spx float64
		for k := range syncSymbols {
			v := dc + g*deviationHz*float64(syms[k])
			switch {
			case k < c.clicks:
				v -= 4 * deviationHz * float64(syms[k])
			default:
				v += c.spread * float64(1-2*(k&1))
			}
			r.ring[j] = int8(math.Round(v / discStepHz))
			if k >= c.clicks {
				q, p := float64(r.ring[j])*discStepHz, float64(syms[k])
				n, sp, spp, sx, spx = n+1, sp+p, spp+p*p, sx+q, spx+p*q
			}
			if j++; j == leanRing {
				j = 0
			}
		}
		gi := (n*spx - sp*sx) / (n*spp - sp*sp)
		dci := (sx - gi*sp) / n
		g0, dc0, res0 := r.fitSync(end, enums.SyncBSSourcedVoice)
		r.devScale, r.dcHz, r.discVar = g, dc, 1
		if !r.fitLevels(end, enums.SyncBSSourcedVoice, false, true) {
			t.Fatalf("%+v: the fit is refused", c)
		}
		wantG, wantDC := float32(levelForget*g+(1-levelForget)*gi/deviationHz), float32(levelForget*dc+(1-levelForget)*dci)
		if !c.refit {
			wantG, wantDC = levelForget*g+(1-levelForget)*g0, levelForget*dc+(1-levelForget)*dc0
		}
		if math.Abs(float64(r.devScale-wantG)) > 1e-4 || math.Abs(float64(r.dcHz-wantDC)) > 0.1 {
			t.Errorf("%+v: levels %.4f %.1f Hz, want %.4f %.1f Hz", c, r.devScale, r.dcHz, wantG, wantDC)
		}
		if want := levelForget + (1-levelForget)*res0; math.Abs(float64(r.discVar-want)) > 1e-3*float64(want) {
			t.Errorf("%+v: variance %.0f, want %.0f from every symbol", c, r.discVar, want)
		}
	}
}

func TestLevelStepNeedsResidual(t *testing.T) {
	t.Parallel()
	var syms [syncSymbols]int8
	syncSyms(enums.SyncBSSourcedVoice, &syms)
	fired := 0
	for i := range 64 {
		g, dc := 0.5+0.005*float64(i), 100+3*float64(i)
		r := &leanCore{ns: 500}
		end := r.ns - 1
		j := ringPos(end - syncSymbols + 1)
		for k := range syncSymbols {
			r.ring[j] = int8(math.Round((dc + g*deviationHz*float64(syms[k])) / discStepHz))
			if j++; j == leanRing {
				j = 0
			}
		}
		r.devScale, r.dcHz, r.discVar = float32(g), float32(dc), 1
		r.fitLevels(end, enums.SyncBSSourcedVoice, false, true)
		if r.preDev != 0 {
			fired++
		}
	}
	if fired != 0 {
		t.Errorf("a noise-free SYNC at the held levels fires a level step in %d of 64 cases", fired)
	}
}
