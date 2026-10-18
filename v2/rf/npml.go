package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Settings of npml: the weight of each SYNC field in the noise correlation,
// its bound, and the SYNC fields before it counts; and the noise variance,
// over the square of the deviation of level 1, under which it detects
// symbol by symbol. There a level has 2.8 times the noise as its margin,
// so that the symbol errors are rare, and the residual of the SYNC fits
// holds much of the distortion of the transmitter and the radio, which the
// noise prediction does not model.
const (
	npmlGain  = 0.1
	npmlMax   = 0.9
	npmlMin   = 4
	npmlQuiet = 1.0 / 8
)

// npml detects the symbols of a burst from the discriminator samples of
// the core with the correlation of their noise from symbol to symbol. The
// noise of a discriminator rises with the square of the frequency, so that
// after the matched filter the noise of a symbol is about rho times that of
// the symbol before, with rho near -0.5 above the FM threshold and near 0
// below it, where the clicks set the noise. A BCJR over the values of the
// symbol before predicts the noise of each symbol from that of the one
// before, and weighs the rest, whose variance is 1-rho^2 of the noise.
type npml struct {
	// rho is the correlation of the noise of a symbol with that of the
	// symbol before, from n SYNC fields of the stream.
	rho float32
	n   int
	// alpha holds the log state metrics of the forward pass, by symbol and
	// digit of the symbol.
	alpha [burstSymbols + 1][4]float32
}

// fitSync adds the noise of the SYNC field with pattern p of the slot of
// core c from symbol base to the correlation.
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

// end forgets the correlation of a stream.
func (q *npml) end() { q.n = 0 }

// fill sets llr to the log-likelihood ratios of the burst of core c at
// c.base, as softExt.fill does symbol by symbol, by the BCJR over the
// prediction of the noise. The floor of the likelihoods of the core keeps
// a click from making a confident error.
func (q *npml) fill(c *leanCore, llr *[layer2.BurstBits]int16) {
	rho := q.rho
	if q.n < npmlMin || c.discVar < npmlQuiet*c.dev*c.dev {
		rho = 0
	}
	// The log-likelihood factor of the noise that the prediction leaves,
	// and the floor, in nats.
	k := -1 / (2 * max(c.discVar, 1) * (1 - rho*rho))
	floor := float32(-math.MaxFloat32)
	if c.llrFloor > math.MinInt32/2 {
		floor = float32(c.llrFloor) / 8
	}
	dev := c.dev
	// Symbol i of the run is slot symbol cachSymbols-1+i: the last of the
	// CACH starts it with its own likelihood.
	var x [burstSymbols + 1]float32
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
