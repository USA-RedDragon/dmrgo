package rf

import (
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func (e *floorEval) detectSoft(i int, pri, out *[softBurstBits]float32) {
	d := e.g.d
	n0, n1, mask := e.span(i)
	steps := n1 - n0
	prior := make([]float32, 4*steps)
	base := i*slotSymbols + cachSymbols
	for k := range burstSymbols {
		if !e.free[2*k] && !e.free[2*k+1] {
			continue
		}
		j := base + k - n0 - d
		for v := range 4 {
			b0, b1 := dibitOfDigit(v)
			var lp float32
			if e.free[2*k] {
				lp += halfPrior(pri[2*k], b0 == 0)
			}
			if e.free[2*k+1] {
				lp += halfPrior(pri[2*k+1], b1 == 0)
			}
			prior[4*j+v] = lp
		}
	}
	e.g.run(e.f, n0, n1, mask, false, e.tail, prior, e.app)
	*out = [softBurstBits]float32{}
	for k := range burstSymbols {
		if !e.free[2*k] && !e.free[2*k+1] {
			continue
		}
		j := base + k - n0 - d
		a := e.app[4*j : 4*j+4]
		if e.free[2*k] {
			out[2*k] = floorLogAdd(a[2], a[3]) - floorLogAdd(a[0], a[1])
		}
		if e.free[2*k+1] {
			out[2*k+1] = floorLogAdd(a[1], a[2]) - floorLogAdd(a[0], a[3])
		}
	}
}

func halfPrior(l float32, zero bool) float32 {
	if zero {
		return l / 2
	}
	return -l / 2
}

func toLLR(l float32) int16 {
	return satLLR(float32(math.Max(-4000, math.Min(4000, float64(l)))))
}

func (e *floorEval) bptcTurbo(i int, dt enums.DataType, iters int, alpha float32) (sep, turbo bool) {
	check := softBPTCCheck(dt)
	d := e.dec
	tb := e.truth[i].burst
	want, _ := tb.BPTCPayload()
	var free, rpos []int16
	for j := range softBPTCCode.code().n {
		r := false
		for x := softBPTCData; x < softBPTCInfo; x++ {
			r = r || int(softBPTCCode.code().sys[x]) == j
		}
		if r {
			rpos = append(rpos, softBPTCCode.code().pos[j])
		} else {
			free = append(free, softBPTCCode.code().pos[j])
		}
	}
	e.setFree(free)
	var pri, lapp, le, ext [softBurstBits]float32
	var lei [softBurstBits]int16
	gridExt := func() {
		for p := range le {
			lei[p] = toLLR(le[p])
		}
		d.grid.decode(&lei, e.gridIters, e.gridWeight)
		d.grid.extrinsic(&ext)
	}
	for it := range iters {
		e.detectSoft(i, &pri, &lapp)
		for _, p := range free {
			le[p] = lapp[p] - pri[p]
		}
		for _, p := range rpos {
			le[p] = 1000
			if tb[p/8]&(0x80>>(p%8)) != 0 {
				le[p] = -1000
			}
		}
		if it > 0 {
			gridExt()
		}
		e.llr = [softBurstBits]int16{}
		var img [softBurstBits]unpacked.Bit
		for _, p := range free {
			e.llr[p] = toLLR(le[p] + ext[p])
			if e.llr[p] < 0 {
				img[p] = 1
			}
		}
		for _, p := range rpos {
			e.llr[p] = toLLR(le[p])
			if e.llr[p] < 0 {
				img[p] = 1
			}
		}
		res, fr := d.decodeBPTCOSD(&e.llr, check, nil)
		ok := res.Payload == want
		if it == 0 {
			sep = ok
		}
		turbo = ok
		if check != nil && !fr.Uncorrectable || check == nil && gridIsCodeword(&softGridPos, &softGridRowH, &softGridColH, &img) {
			break
		}
		if it == 0 {
			gridExt()
		}
		for _, p := range free {
			pri[p] = max(-floorPriorMax, min(floorPriorMax, alpha*ext[p]))
		}
	}
	return sep, turbo
}

const floorPriorMax = 30

func TestFloorTurbo(t *testing.T) {
	t.Parallel()
	spec := os.Getenv("DMRRF_FLOOR_TURBO")
	if spec == "" {
		t.Skip("set DMRRF_FLOOR_TURBO=alpha,gridpasses,gridweight,passes,cn+cn")
		return
	}
	f := strings.Split(spec, ",")
	alpha, _ := strconv.ParseFloat(f[0], 64)
	gi, _ := strconv.Atoi(f[1])
	gw, _ := strconv.ParseFloat(f[2], 64)
	iters, _ := strconv.Atoi(f[3])
	slots, truth := benchStream(t, 30)
	for _, c := range strings.Split(f[4], "+") {
		cn, _ := strconv.ParseFloat(c, 64)
		var mu sync.Mutex
		var wg sync.WaitGroup
		n, ns, nt := 0, 0, 0
		for seed := range uint64(16) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e := newFloorEval(1)
				e.gridIters, e.gridWeight = gi, float32(gw)
				e.truth = truth
				e.f = newFloorSignal(slots, cn, 100+seed)
				a, b, m := 0, 0, 0
				for i, ts := range truth {
					if ts.kind != "data" || (ts.dt != enums.DataTypeRate12 && ts.dt != enums.DataTypeCSBK) {
						continue
					}
					x, y := e.bptcTurbo(i, ts.dt, iters, float32(alpha))
					m++
					if x {
						a++
					}
					if y {
						b++
					}
				}
				mu.Lock()
				n, ns, nt = n+m, ns+a, nt+b
				mu.Unlock()
			}()
		}
		wg.Wait()
		t.Logf("%s cn %.1f eb %.2f: %d blocks, one pass %.4f, iterative %.4f", spec, cn, cn+1.146, n, 1-float64(ns)/float64(n), 1-float64(nt)/float64(n))
	}
}
