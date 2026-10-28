//go:build !tinygo

package rf

import "math"

const (
	acDecHz     = 2400
	acBlock     = acDecHz / 4
	acOnRatio   = 3e-5
	acOffRatio  = 1.6e-4
	acRunBlocks = 2
	acPeakDecay = 0.97
	acQuiet     = 0.1
	acLowHz     = 100
	acMidHz     = 300
	acDCHz      = 20
	acSpan      = 8
	acPhases    = 4
	acLag       = 30
	acFFE       = 5
	acDFE       = 16
	acRing      = 64
	acMu        = 6e-4
	acTaps      = 2*acFFE + 1 + acDFE + 1
	acForget    = 0.995
	acRLSInit   = 20
	acRLSMax    = 1e9
	acRho       = 0.75
	acErrOn     = 0.4
	acErrOff    = 0.8
	acErrGain   = 0.02
	acScaleGain = 1.0 / 1024
	acShortGain = 0.1
	acLive      = 0.05
	acLevel     = 1000
	acPulseRes  = 32
	acChunk     = 512
	acOutRing   = 512
	acHold      = 2
	acActive    = 2000
	acAcquire   = 4000
	acTED       = -1e-4
	acMaxSym    = 6
)

type acBiquad struct {
	b0, b1, b2, a1, a2 float32
	z1, z2             float32
}

func newACBiquad(fc, fs float64, high bool) acBiquad {
	w := 2 * math.Pi * fc / fs
	c, al := math.Cos(w), math.Sin(w)/math.Sqrt2
	a0 := 1 + al
	b0, b1 := (1-c)/2, 1-c
	if high {
		b0, b1 = (1+c)/2, -(1 + c)
	}
	return acBiquad{b0: float32(b0 / a0), b1: float32(b1 / a0), b2: float32(b0 / a0), a1: float32(-2 * c / a0), a2: float32((1 - al) / a0)}
}

func (q *acBiquad) push(x float32) float32 {
	y := q.b0*x + q.z1
	q.z1 = q.b1*x - q.a1*y + q.z2
	q.z2 = q.b2*x - q.a2*y
	return y
}

type acDetect struct {
	b, n       int
	norm       float32
	i1, i2, i3 int32
	c1, c2, c3 int32
	dc, dcGain float32
	low        [3]acBiquad
	mid        acBiquad
	elf, emid  float32
	peak       float32
	cnt        int
	run        int
	on         bool
}

func (d *acDetect) init(hz int) {
	d.b = hz / acDecHz
	d.norm = 1 / float32(d.b*d.b*d.b)
	d.dcGain = float32(2 * math.Pi * acDCHz / acDecHz)
	for i := range d.low {
		d.low[i] = newACBiquad(acLowHz, acDecHz, false)
	}
	d.mid = newACBiquad(acMidHz, acDecHz, true)
}

func (d *acDetect) push(v int16) {
	d.i1 += int32(v)
	d.i2 += d.i1
	d.i3 += d.i2
	if d.n++; d.n < d.b {
		return
	}
	d.n = 0
	d.comb()
}

func (d *acDetect) scan(x []int16) int {
	i1, i2, i3, n, b := d.i1, d.i2, d.i3, d.n, d.b
	for i, v := range x {
		i1 += int32(v)
		i2 += i1
		i3 += i2
		if n++; n < b {
			continue
		}
		n = 0
		d.i1, d.i2, d.i3 = i1, i2, i3
		if d.comb(); d.on {
			d.n = 0
			return i + 1
		}
	}
	d.i1, d.i2, d.i3, d.n = i1, i2, i3, n
	return len(x)
}

func (d *acDetect) comb() {
	e1 := d.i3 - d.c1
	d.c1 = d.i3
	e2 := e1 - d.c2
	d.c2 = e1
	e3 := e2 - d.c3
	d.c3 = e2
	w := float32(e3) * d.norm
	d.dc += d.dcGain * (w - d.dc)
	w -= d.dc
	l := d.low[2].push(d.low[1].push(d.low[0].push(w)))
	m := d.mid.push(w)
	d.elf += l * l
	d.emid += m * m
	if d.cnt++; d.cnt < acBlock {
		return
	}
	d.peak = max(d.emid, d.peak*acPeakDecay)
	if was := d.on; d.emid > acQuiet*d.peak {
		r := d.elf / d.emid
		switch {
		case !d.on && r < acOnRatio:
			d.run++
			d.on = d.run >= acRunBlocks
		case !d.on:
			d.run = 0
		case r < acOffRatio:
			d.run = 0
		default:
			d.run++
			d.on = d.run < acRunBlocks
		}
		if d.on != was {
			d.run = 0
		}
	}
	d.elf, d.emid, d.cnt = 0, 0, 0
}

type acPending struct {
	k      int64
	q      int
	rho    float32
	rhos   [acPhases]float32
	pat    int
	sign   float32
	active bool
}

type acStage struct {
	det     acDetect
	sps     float64
	off     [acPhases]float64
	mf      []float32
	mfHist  []float32
	mfHead  int
	mfHalf  float64
	uh      [32]float32
	n       int64
	t       float64
	k       int64
	ring    [acPhases][acRing]float32
	stime   [acRing]float64
	known   [acRing]int8
	dec     [acRing]int8
	live    [acRing]bool
	scale   float32
	short   float32
	scaled  bool
	w       [acTaps]float32
	p       [acTaps * acTaps]float64
	prevY   float32
	err     float32
	trained bool
	last    int64
	lastK   int64
	pend    acPending
	pulse   []float32
	out     [acOutRing]float32
	delay   float64
	lag     float64
	buf     [acChunk]int16
}

func newACStage(hz int) *acStage {
	a := &acStage{sps: float64(hz) / symbolRate}
	a.det.init(hz)
	for q := range a.off {
		a.off[q] = (float64(q) - 2) / acPhases * a.sps
	}
	n := int(2*acSpan*a.sps) + 1
	a.mf = make([]float32, n)
	a.mfHist = make([]float32, 2*n)
	a.mfHalf = float64(n / 2)
	var c float64
	for i := range a.mf {
		v := acRRC((float64(i) - a.mfHalf) / a.sps)
		a.mf[i] = float32(v)
		c += v
	}
	for i := range a.mf {
		a.mf[i] /= float32(c)
	}
	a.pulse = make([]float32, 2*acSpan*acPulseRes+2)
	for i := range a.pulse {
		a.pulse[i] = float32(acRRC(float64(i)/acPulseRes - acSpan))
	}
	a.delay = math.Ceil(float64(acLag+acFFE+acHold+2)*a.sps + a.mfHalf + acSpan*a.sps + 4)
	a.lag = a.delay / float64(hz)
	a.w[acFFE] = 1
	a.resetRLS()
	a.err, a.last = acErrOff+1, -acAcquire
	return a
}

func acRRC(t float64) float64 {
	switch {
	case t == 0:
		return 1 - rrcAlpha + 4*rrcAlpha/math.Pi
	case math.Abs(math.Abs(4*rrcAlpha*t)-1) < 1e-9:
		return rrcAlpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*rrcAlpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*rrcAlpha)))
	}
	return (math.Sin(math.Pi*t*(1-rrcAlpha)) + 4*rrcAlpha*t*math.Cos(math.Pi*t*(1+rrcAlpha))) /
		(math.Pi * t * (1 - (4*rrcAlpha*t)*(4*rrcAlpha*t)))
}

func (a *acStage) write(r *discRx, x []int16) {
	for len(x) > 0 {
		if !a.det.on {
			i := a.det.scan(x)
			a.n += int64(i)
			a.core(r, x[:i])
			x = x[i:]
			if a.det.on {
				a.resume()
			}
			continue
		}
		if !a.trained {
			i := 0
			for i < len(x) && a.det.on && !a.trained {
				a.step(x[i])
				i++
			}
			if !a.det.on {
				a.trained = false
			}
			a.core(r, x[:i])
			x = x[i:]
			continue
		}
		n := min(len(x), acChunk)
		for i := range n {
			a.buf[i] = a.step(x[i])
			if !a.det.on {
				n = i + 1
				break
			}
		}
		if !a.det.on {
			a.trained = false
		}
		a.core(r, a.buf[:n])
		x = x[n:]
	}
}

func (a *acStage) core(r *discRx, x []int16) {
	r.writeCore(x)
}

func (a *acStage) stamp(b *Burst) {
	if a.trained {
		b.Seconds -= a.lag
	}
}

func (a *acStage) resume() {
	if t := float64(a.n); a.t < t {
		a.k += int64((t - a.t) / a.sps)
		a.t = t
	}
	a.out = [acOutRing]float32{}
	a.pend.active = false
}

func (a *acStage) step(v int16) int16 {
	a.det.push(v)
	h := a.mfHead
	nt := len(a.mf)
	a.mfHist[h], a.mfHist[h+nt] = float32(v), float32(v)
	if h++; h == nt {
		h = 0
	}
	a.mfHead = h
	var u float32
	for i, c := range a.mf {
		u += c * a.mfHist[h+i]
	}
	i := a.n
	a.uh[i&31] = u
	a.n++
	for float64(i) >= math.Floor(a.t+a.off[acPhases-1])+2 {
		a.symbol()
		a.t += a.sps
	}
	j := i & (acOutRing - 1)
	o := a.out[j]
	a.out[j] = 0
	if !a.trained {
		return v
	}
	return int16(max(min(o, math.MaxInt16), math.MinInt16))
}

func (a *acStage) at(t float64) float32 {
	b := math.Floor(t)
	mu := float32(t - b)
	i := int64(b)
	p0, p1, p2, p3 := a.uh[(i-1)&31], a.uh[i&31], a.uh[(i+1)&31], a.uh[(i+2)&31]
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0)))
}

func (a *acStage) symbol() {
	k := a.k
	a.k++
	idx := int(k & (acRing - 1))
	var vals [acPhases]float32
	for q := range vals {
		vals[q] = a.at(a.t + a.off[q])
	}
	x0 := vals[2]
	if !a.scaled {
		a.scale, a.short, a.scaled = max(x0*x0, 1e-12), x0*x0, true
	} else {
		a.short += acShortGain * (x0*x0 - a.short)
		if a.short > acLive*a.scale {
			a.scale += acScaleGain * (x0*x0 - a.scale)
		}
	}
	g := sqrtf(5 / max(a.scale, 1e-12))
	for q, v := range vals {
		a.ring[q][idx] = v * g
	}
	a.stime[idx] = a.t
	a.live[idx] = a.short > acLive*a.scale
	a.known[idx] = 0
	a.search(k)
	if j := k - acLag; j >= acDFE+acFFE {
		a.equalize(j)
	}
}

func (a *acStage) search(k int64) {
	if k < acRing {
		return
	}
	if a.k-a.last <= acAcquire {
		if m := (k - a.lastK) % slotSymbols; m > 1 && m < slotSymbols-1 {
			a.flushPending(k)
			return
		}
	}
	var best acPending
	for q := range acPhases {
		rg := &a.ring[q]
		var w [syncSymbols]float32
		var s1, s2 float32
		for i := range w {
			v := rg[(k-syncSymbols+1+int64(i))&(acRing-1)]
			w[i] = v
			s1 += v
			s2 += v * v
		}
		sxx := s2 - s1*s1/syncSymbols
		if sxx <= 0 {
			continue
		}
		den := 1 / sqrtf(syncDevSq[0]*sxx)
		var bq float32
		bp, bs := 0, float32(1)
		for p := range syncSyms8 {
			var c float32
			for i, s := range syncSyms8[p] {
				c += float32(s) * w[i]
			}
			c *= den
			if c < 0 && -c > bq {
				bq, bp, bs = -c, p, -1
			} else if c > bq {
				bq, bp, bs = c, p, 1
			}
		}
		best.rhos[q] = bq
		if bq > best.rho {
			best.rho, best.q, best.pat, best.sign = bq, q, bp, bs
		}
	}
	p := &a.pend
	if best.rho >= acRho && (!p.active || best.rho > p.rho) {
		best.k, best.active = k, true
		*p = best
	}
	a.flushPending(k)
}

func (a *acStage) flushPending(k int64) {
	if p := &a.pend; p.active && k-p.k >= acHold {
		a.commit(p)
		p.active = false
	}
}

func (a *acStage) commit(p *acPending) {
	for i, s := range syncSyms8[p.pat] {
		a.known[(p.k-syncSymbols+1+int64(i))&(acRing-1)] = int8(float32(s) * p.sign)
	}
	if a.k-a.last > acAcquire {
		d := a.off[p.q] - a.off[2]
		if p.q > 0 && p.q < acPhases-1 {
			lo, mid, hi := p.rhos[p.q-1], p.rhos[p.q], p.rhos[p.q+1]
			if den := lo - 2*mid + hi; den < 0 {
				d += float64(max32(min32(0.5*(lo-hi)/den, 0.5), -0.5)) * a.sps / acPhases
			}
		}
		a.t += d
	}
	a.lastK, a.last = p.k, a.k
}

func acSlice(y float32) int8 {
	switch {
	case y >= 2:
		return 3
	case y >= 0:
		return 1
	case y >= -2:
		return -1
	}
	return -3
}

func (a *acStage) equalize(j int64) {
	r0 := &a.ring[2]
	var x [acTaps]float32
	for i := range 2*acFFE + 1 {
		x[i] = r0[(j-acFFE+int64(i))&(acRing-1)]
	}
	for m := range acDFE {
		x[2*acFFE+1+m] = float32(a.dec[(j-1-int64(m))&(acRing-1)])
	}
	x[acTaps-1] = 1
	var y float32
	for i, v := range x {
		y += a.w[i] * v
	}
	idx := int(j & (acRing - 1))
	live := a.live[idx]
	t := a.known[idx]
	switch {
	case t != 0:
		e := y - float32(t)
		a.err += acErrGain * (e*e - a.err)
		switch {
		case a.err < acErrOn:
			a.trained = true
		case a.err > acErrOff:
			a.trained = false
		}
		if live {
			a.rls(&x, float32(t)-y)
		}
	default:
		t = acSlice(y)
		if a.trained && live && a.k-a.last < acActive {
			e := acMu * (y - float32(t))
			for i, v := range x {
				a.w[i] -= e * v
			}
		}
	}
	if a.trained && live {
		a.t += acTED * float64(a.prevY*float32(t)-y*float32(a.dec[(j-1)&(acRing-1)])) * a.sps
	}
	a.prevY = y
	a.dec[idx] = t
	a.place(a.stime[idx]-a.mfHalf+a.delay, max32(min32(y, acMaxSym), -acMaxSym)*acLevel)
}

func (a *acStage) rls(x *[acTaps]float32, e float32) {
	var px [acTaps]float64
	den := acForget
	for i := range px {
		row := a.p[i*acTaps : (i+1)*acTaps]
		var s float64
		for k, v := range x {
			s += row[k] * float64(v)
		}
		px[i] = s
		den += s * float64(x[i])
	}
	if !(den > 0) || den > acRLSMax {
		a.resetRLS()
		return
	}
	for i, v := range px {
		a.w[i] += float32(v / den * float64(e))
	}
	for i, vi := range px {
		row := a.p[i*acTaps : (i+1)*acTaps]
		c := vi / den
		for k, vk := range px {
			row[k] = (row[k] - c*vk) / acForget
		}
	}
}

func (a *acStage) resetRLS() {
	for i := range a.p {
		a.p[i] = 0
	}
	for i := range acTaps {
		a.p[i*acTaps+i] = acRLSInit
	}
}

func (a *acStage) place(c float64, z float32) {
	span := acSpan * a.sps
	m0, m1 := int64(math.Ceil(c-span)), int64(math.Floor(c+span))
	for m := m0; m <= m1; m++ {
		f := (float64(m)-c)/a.sps*acPulseRes + acSpan*acPulseRes
		i := int(f)
		if i < 0 || i+1 >= len(a.pulse) {
			continue
		}
		w := float32(f - float64(i))
		a.out[m&(acOutRing-1)] += z * (a.pulse[i] + w*(a.pulse[i+1]-a.pulse[i]))
	}
}
