package rf

import "math"

const (
	frontRMS     = 24
	frontBlock   = 256
	frontRelease = 4
	rrcStepHz    = 128
)

type leanFront struct {
	half    bool
	odd     bool
	quiet   uint8
	skip    bool
	gain    float32
	inv2    float32
	gAcc    float32
	gN      int
	hbHist  [halfTaps][2]int8
	hbHead  int
	ncoStep complex64
	ncoRot  complex64
	ncoN    int
	preHist [preTaps][2]int8
	preHead int
	prev    complex64
	lag     complex64
	rrcHist [rrcTaps]int8
	rrcHead int
}

func (f *leanFront) init(half bool) {
	*f = leanFront{half: half, ncoRot: 1, ncoStep: 1, gain: frontRMS}
	f.inv2 = 1 / (f.gain * f.gain)
}

func (f *leanFront) center() int64 {
	c := -int64(preTaps-1) - 1 - discSpan*samplesPerSy
	if !f.half {
		c += 1 - halfTaps/2
	}
	return c
}

func (f *leanFront) lagTicks() int32 {
	c := -int64(2*(preTaps/2)+2*(rrcTaps/2-1))<<8 - 256
	if !f.half {
		c += int64(1-halfTaps/2) << 8
	}
	return int32(f.center()<<8 - c)
}

func (f *leanFront) tune(hz float64) {
	p := -2 * math.Pi * hz / discRate
	f.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

func (f *leanFront) shift(d float32) {
	for i, v := range &f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) + d/rrcStepHz)
	}
}

func at2(h [][2]int8, head, k int) [2]int8 {
	if k += head; k >= len(h) {
		k -= len(h)
	}
	return h[k]
}

func (f *leanFront) push(v complex64) (s, power float32, ok bool) {
	f.gAcc += real(v)*real(v) + imag(v)*imag(v)
	if f.gN++; f.gN == frontBlock {
		if rms := sqrtf(f.gAcc / frontBlock); rms > 0 {
			switch g := frontRMS / rms; {
			case g < f.gain/2, g > 2*f.gain && f.quiet+1 >= frontRelease:
				f.gain, f.quiet = g, 0
				f.inv2 = 1 / (g * g)
			case g > 2*f.gain:
				f.quiet++
			default:
				f.quiet = 0
			}
		}
		f.gAcc, f.gN = 0, 0
	}
	x := [2]int8{sat8(real(v) * f.gain), sat8(imag(v) * f.gain)}
	if !f.half {
		f.hbHist[f.hbHead] = x
		if f.hbHead++; f.hbHead == halfTaps {
			f.hbHead = 0
		}
		if f.odd = !f.odd; f.odd {
			return 0, 0, false
		}
		c := halfTaps / 2
		m := at2(f.hbHist[:], f.hbHead, c)
		re, im := int32(discHalfQ15[0])*int32(m[0]), int32(discHalfQ15[0])*int32(m[1])
		for k, t := range discHalfQ15[1:] {
			a, b := at2(f.hbHist[:], f.hbHead, c-2*k-1), at2(f.hbHist[:], f.hbHead, c+2*k+1)
			re += int32(t) * (int32(a[0]) + int32(b[0]))
			im += int32(t) * (int32(a[1]) + int32(b[1]))
		}
		x = [2]int8{int8(max(min((re+1<<14)>>15, 127), -128)), int8(max(min((im+1<<14)>>15, 127), -128))}
	}
	if f.ncoN++; f.ncoN == 512 {
		f.ncoN = 0
		g := (3 - real(f.ncoRot)*real(f.ncoRot) - imag(f.ncoRot)*imag(f.ncoRot)) / 2
		f.ncoRot *= complex(g, 0)
	}
	w := complex(float32(x[0]), float32(x[1])) * f.ncoRot
	f.ncoRot *= f.ncoStep
	f.preHist[f.preHead] = [2]int8{sat8(real(w)), sat8(imag(w))}
	if f.preHead++; f.preHead == preTaps {
		f.preHead = 0
	}
	c := preTaps / 2
	m := at2(f.preHist[:], f.preHead, c)
	re, im := int32(discPreQ15[c])*int32(m[0]), int32(discPreQ15[c])*int32(m[1])
	for k, t := range discPreQ15[:c] {
		a, b := at2(f.preHist[:], f.preHead, k), at2(f.preHist[:], f.preHead, preTaps-1-k)
		re += int32(t) * (int32(a[0]) + int32(b[0]))
		im += int32(t) * (int32(a[1]) + int32(b[1]))
	}
	y := complex(float32(re), float32(im)) * (1.0 / 32768)
	d := y * complex(real(f.prev), -imag(f.prev))
	f.prev, f.lag = y, d*complex(f.inv2, 0)
	f.rrcHist[f.rrcHead] = sat8(atan2f(imag(d), real(d)) * (discRate / (2 * math.Pi) / rrcStepHz))
	if f.rrcHead++; f.rrcHead == rrcTaps {
		f.rrcHead = 0
	}
	if f.skip = !f.skip; f.skip {
		return 0, 0, false
	}
	const mid = rrcTaps / 2
	k0 := f.rrcHead + mid
	if k0 >= rrcTaps {
		k0 -= rrcTaps
	}
	acc := int32(discRRCQ15[mid]) * int32(f.rrcHist[k0])
	i, j := f.rrcHead, f.rrcHead-1
	if j < 0 {
		j += rrcTaps
	}
	for _, t := range discRRCQ15[:mid] {
		acc += int32(t) * (int32(f.rrcHist[i]) + int32(f.rrcHist[j]))
		if i++; i == rrcTaps {
			i = 0
		}
		if j--; j < 0 {
			j = rrcTaps - 1
		}
	}
	return float32(acc) * (rrcStepHz / 32768.0), (real(y)*real(y) + imag(y)*imag(y)) * f.inv2, true
}

type leanRx struct {
	leanCore
	fe       *frontEnd
	wide     []complex64
	inChunk  int
	dc       dcBlock
	signed   bool
	df       leanFront
	bPow     float32
	bLag     complex64
	bN       int32
	floor    float32
	noiseCoh complex64
}

func (f *leanFront) retuneFront(hz, d float32) {
	f.turn(2 * math.Pi * d / discRate)
	f.tune(float64(hz))
	f.shift(-d)
}

func (f *leanFront) turn(w float32) {
	for r := range preTaps {
		j := (f.preHead - 1 - r + preTaps) % preTaps
		v := complex(float32(f.preHist[j][0]), float32(f.preHist[j][1])) * turnBy(w, r+1)
		f.preHist[j] = [2]int8{sat8(real(v)), sat8(imag(v))}
	}
	f.prev *= turnBy(w, preTaps-preTaps/2)
}

func newLean(cfg rxSetup, opt options) *leanRx {
	r := &leanRx{signed: cfg.Signed}
	r.dc.init(float64(cfg.BasebandHz))
	r.init(cfg.OnBurst, opt, float32(discSPS)/2)
	r.sink = cfg.sink
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Format, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		r.inChunk = max(r.fe.size*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), r.fe.size)
	}
	r.df.init(cfg.BasebandHz == discRate)
	r.iq, r.center = &r.df, r.df.center()
	r.lagTicks = r.df.lagTicks()
	if cfg.OffsetHz != 0 {
		r.retune(float32(cfg.OffsetHz))
	}
	if opt.SoftExt {
		r.ext = softExtOf(&opt, float32(discSPS)/2)
	}
	return r
}

func (r *leanRx) write(iq []byte) {
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
		return
	}
	for len(iq) > 0 {
		n := min(len(iq), r.inChunk)
		n -= n % r.fe.size
		if n == 0 {
			break
		}
		w := r.fe.process(iq[:n], r.wide)
		iq = iq[n:]
		for _, v := range r.wide[:w] {
			r.sample(v)
		}
	}
}

func (r *leanRx) byteSample(i, q byte) complex64 {
	var v complex64
	if r.signed {
		v = complex(float32(int8(i))/127.5, float32(int8(q))/127.5)
	} else {
		v = complex((float32(i)-127.5)/127.5, (float32(q)-127.5)/127.5)
	}
	return r.dc.remove(v)
}

func (r *leanRx) writeBaseband(x []complex64) {
	for _, v := range x {
		r.sample(v)
	}
}

func (r *leanRx) flush() {
	for range slotSamplesN {
		r.sample(0)
	}
	if r.locked {
		r.unlock()
	}
}

func (r *leanRx) sample(v complex64) {
	s, p, ok := r.df.push(v)
	if !ok {
		return
	}
	r.power(p)
	if !r.locked {
		r.block(p)
	}
	if !r.locked || r.hunt {
		r.keep(s)
	}
	r.push(s)
}

func (r *leanRx) block(p float32) {
	r.bPow += p
	r.bLag += r.df.lag
	if r.bN++; r.bN < leanBlock {
		return
	}
	pw, lag := r.bPow/leanBlock, r.bLag/leanBlock
	r.bPow, r.bLag, r.bN = 0, 0, 0
	if r.floor == 0 || pw < r.floor {
		r.floor = pw
	} else {
		r.floor *= leanFloorRise
	}
	if pw <= leanPresent*r.floor {
		if pw > 0 {
			r.noiseCoh = 0.9*r.noiseCoh + complex(0.1/pw, 0)*lag
		}
		return
	}
	lag -= r.noiseCoh * complex(r.floor, 0)
	if hz := atan2f(imag(lag), real(lag)) * (discRate / (2 * math.Pi)); abs32(hz) > leanSearchHz {
		r.retune(hz)
	}
}
