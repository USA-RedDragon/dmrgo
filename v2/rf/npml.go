package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	npmlGain  = 0.1
	npmlMax   = 0.9
	npmlMin   = 4
	npmlQuiet = 1.0 / 8
)

type npml struct {
	rho   float32
	n     int
	alpha [burstSymbols + 1][4]float32
	x     [burstSymbols + 1]float32
}

func (q *npml) fitSync(c *leanCore, p enums.SyncPattern, base int64) {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	var sxy, sxx, prev float32
	for k := range syncSymbols {
		r := c.sym(base+int64(syncFirstSym+k)) - c.dcHz - float32(syms[k])*c.dev
		if k > 0 {
			sxy += r * prev
		}
		sxx += r * r
		prev = r
	}
	if sxx <= 0 {
		return
	}
	rho := max(min(sxy/sxx*syncSymbols/(syncSymbols-1), npmlMax), -npmlMax)
	if q.n == 0 {
		q.rho = rho
	} else {
		q.rho += npmlGain * (rho - q.rho)
	}
	q.n++
}

func (q *npml) end() { q.n = 0 }

func (q *npml) fill(c *leanCore, llr *[layer2.BurstBits]int16) {
	rho := q.rho
	if q.n < npmlMin || c.discVar < npmlQuiet*c.dev*c.dev {
		rho = 0
	}
	k := -1 / (2 * max(c.discVar, 1) * (1 - rho*rho))
	floor := float32(-math.MaxFloat32)
	if c.llrFloor > math.MinInt32/2 {
		floor = float32(c.llrFloor) / 8
	}
	dev := c.dev
	x := &q.x
	for i := range x {
		x[i] = c.sym(c.base+int64(cachSymbols-1+i)) - c.dcHz
	}
	var lv [4]float32
	for d := range lv {
		lv[d] = float32(2*d-3) * dev
	}
	for d := range 4 {
		e := x[0] - lv[d]
		q.alpha[0][d] = max(e*e*k*(1-rho*rho), floor)
	}
	for i := 1; i <= burstSymbols; i++ {
		for d1 := range 4 {
			best := float32(-math.MaxFloat32)
			for d0 := range 4 {
				e := (x[i] - lv[d1]) - rho*(x[i-1]-lv[d0])
				best = max(best, q.alpha[i-1][d0]+max(e*e*k, floor))
			}
			q.alpha[i][d1] = best
		}
	}
	var beta [4]float32
	for i := burstSymbols; i >= 1; i-- {
		var app [4]float32
		for d := range 4 {
			app[d] = q.alpha[i][d] + beta[d]
		}
		p := 2 * (i - 1)
		llr[p], llr[p+1] = dibitLLR(app[:], 1)
		var nb [4]float32
		for d0 := range 4 {
			best := float32(-math.MaxFloat32)
			for d1 := range 4 {
				e := (x[i] - lv[d1]) - rho*(x[i-1]-lv[d0])
				best = max(best, max(e*e*k, floor)+beta[d1])
			}
			nb[d0] = best
		}
		beta = nb
	}
}
