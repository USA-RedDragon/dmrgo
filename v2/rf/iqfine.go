package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

const (
	iqFineLen   = 72
	iqFineHz    = 150
	iqFineReach = 4
)

type iqFine struct {
	y   [iqFineLen]float32
	hb  [halfTaps]float32
	rrc [audio12Taps]float32
}

func (r *iqRx) attachFine(x *softExt) {
	if r.refill == nil || !r.d.f.half || r.d.f.slow || r.conv.halfOn {
		return
	}
	r.fine = new(iqFine)
	x.fineZero = r.fineZero
}

func (r *iqRx) fineZero(c *leanCore, p enums.SyncPattern, end, d float32) (float32, bool) {
	sps := 2 * c.tm.step
	if back := -(end + d) + float32(syncSymbols-1)*sps; back+sps/4+iqFineReach > iqFineLen-1 || end+d > 0 {
		return 0, false
	}
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	g := r.fine
	m := c.n
	if !r.fineRun(m, 0) {
		return 0, false
	}
	a, dc := g.fit(&syms, end+d, sps)
	if a <= 0 {
		return 0, false
	}
	if abs32(dc) >= iqFineHz {
		if !r.fineRun(m, dc) {
			return 0, false
		}
		if a, _ = g.fit(&syms, end+d, sps); a <= 0 {
			return 0, false
		}
	}
	return g.zero(&syms, end, d, sps)
}

func (r *iqRx) fineRun(m int64, shift float32) bool {
	f, g := r.refill, r.fine
	per := 2 * int64(r.conv.decim)
	j0 := m - iqFineLen
	warm := int64(len(r.conv.taps)) + int64(2*audio12Taps+halfTaps)*per
	s := j0*per - warm
	if s < 0 || s < max(f.n-int64(len(f.raw)), f.from) || m*per > f.n || len(f.conv.hist) < 2*len(r.conv.taps) {
		return false
	}
	s = (s + per - 1) / per * per
	cv := &f.conv
	hist := cv.hist
	clear(hist)
	*cv = iqDisc{decim: r.conv.decim, step: r.conv.step, taps: r.conv.taps, hist: hist, ncoRot: 1, ncoStep: r.conv.ncoStep, hz: r.conv.hz, fc: r.conv.fc, post: 1, postW: r.conv.postW, postRot: r.conv.postRot, floor: r.conv.floor, span: r.conv.span}
	if shift != 0 {
		cv.tuneAt(r.conv.hz+shift, r.filterAt(r.conv.hz+shift))
	}
	clear(g.hb[:])
	clear(g.rrc[:])
	sign := float32(1)
	if r.d.f.gain < 0 {
		sign = -1
	}
	var hbN, rrcN int
	odd := false
	j := s / per
	p := f.head - int(f.n-s)
	if p < 0 {
		p += len(f.raw)
	}
	for i := s; i < m*per; i++ {
		w := f.raw[p]
		if p++; p == len(f.raw) {
			p = 0
		}
		v, _, ok := cv.push(w)
		if !ok {
			continue
		}
		g.hb[hbN] = sign * v
		if hbN++; hbN == halfTaps {
			hbN = 0
		}
		if odd = !odd; odd {
			continue
		}
		const ch = halfTaps / 2
		hv := float32(discHalfQ15[0]) * g.hb[ringAt(hbN, ch)]
		for k, t := range discHalfQ15[1:] {
			hv += float32(t) * (g.hb[ringAt(hbN, ch-2*k-1)] + g.hb[ringAt(hbN, ch+2*k+1)])
		}
		g.rrc[rrcN] = hv * (1.0 / 32768)
		if rrcN++; rrcN == audio12Taps {
			rrcN = 0
		}
		if j >= j0 && j < m {
			var y float32
			for k := range audio12Taps {
				t := min(k, audio12Taps-1-k)
				q := rrcN + k
				if q >= audio12Taps {
					q -= audio12Taps
				}
				y += float32(audioRRCQ15[t]) * g.rrc[q]
			}
			g.y[j-j0] = y * (1.0 / 32768)
		}
		j++
	}
	return j == m
}

func (g *iqFine) at(t float32) float32 {
	x := float32(iqFineLen-1) + t
	i0 := min(max(int(math.Floor(float64(x)))-2, 0), iqFineLen-6)
	u := x - float32(i0)
	var v float32
	for i := range 6 {
		l := float32(1)
		for k := range 6 {
			if k != i {
				l *= (u - float32(k)) / float32(i-k)
			}
		}
		v += l * g.y[i0+i]
	}
	return v
}

func (g *iqFine) fit(syms *[syncSymbols]int8, t, sps float32) (a, dc float32) {
	var sv, sx, sxx, sxv float32
	for i, s := range syms {
		v := g.at(t - float32(syncSymbols-1-i)*sps)
		q := float32(s)
		sv += v
		sx += q
		sxx += q * q
		sxv += q * v
	}
	const n = syncSymbols
	den := sxx - sx*sx/n
	if den <= 0 {
		return 0, 0
	}
	a = (sxv - sx*sv/n) / den
	return a, (sv - a*sx) / n
}

func (g *iqFine) mmSum(syms *[syncSymbols]int8, t, sps float32) float32 {
	var e float32
	prev := g.at(t - float32(syncSymbols-1)*sps)
	m := prev
	for k := 1; k < syncSymbols; k++ {
		y := g.at(t - float32(syncSymbols-1-k)*sps)
		e += float32(syms[k-1])*y - float32(syms[k])*prev
		m += y
		prev = y
	}
	return e - m/syncSymbols*float32(syms[0]-syms[syncSymbols-1])
}

func (g *iqFine) zero(syms *[syncSymbols]int8, end, d, sps float32) (float32, bool) {
	const h = leanLockMMStep
	c := d
	for range 2 * leanMMSteps {
		lo, hi := c-h, c+h
		a, b := g.mmSum(syms, end+lo, sps), g.mmSum(syms, end+hi, sps)
		if a == b {
			return 0, false
		}
		z := lo - a*(hi-lo)/(b-a)
		if z > c-2*h && z < c+2*h {
			return z, true
		}
		if z > c {
			c += 2 * h
		} else {
			c -= 2 * h
		}
		if abs32(c-d) > sps/4 {
			return 0, false
		}
	}
	return 0, false
}
