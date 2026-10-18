package rf

import "math"

// Layout of the 16-bit front end.
const (
	// frontRMS is the RMS of the input, in 8-bit units, that the gain
	// control of leanFront keeps, and frontBlock the number of input
	// samples of each of its measurements.
	frontRMS   = 24
	frontBlock = 256
	// rrcStepHz is the resolution of the discriminator samples that the
	// matched filter holds.
	rrcStepHz = 128
)

// leanFront is discFront for the streaming receiver, with its filter
// history held as 8-bit samples in rings and its taps in Q15: discHalfQ15,
// center tap first, discPreQ15 and discRRCQ15, each up to its center tap. A
// gain control scales the input to frontRMS. It returns the powers and lag
// products in units of the input.
type leanFront struct {
	half bool
	odd  bool
	// skip is set after an output of the discriminator that the matched
	// filter does not follow.
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

// init sets up the front end for input at discRate when half, at
// basebandRate otherwise.
func (f *leanFront) init(half bool) {
	*f = leanFront{half: half, ncoRot: 1, ncoStep: 1, gain: frontRMS}
	f.inv2 = 1 / (f.gain * f.gain)
}

// center returns the input sample at the center of discriminator sample
// 0, as discFront.center.
func (f *leanFront) center() int64 {
	c := -int64(preTaps-1) - 1 - discSpan*samplesPerSy
	if !f.half {
		c += 1 - halfTaps/2
	}
	return c
}

// tune sets the oscillator to remove a carrier at hz, from the next sample.
func (f *leanFront) tune(hz float64) {
	p := -2 * math.Pi * hz / discRate
	f.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

// shift adds d Hz to the discriminator samples that the matched filter
// holds.
func (f *leanFront) shift(d float32) {
	for i, v := range &f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) + d/rrcStepHz)
	}
}

// at2 returns sample k of the ring h whose oldest sample is at head.
func at2(h [][2]int8, head, k int) [2]int8 {
	if k += head; k >= len(h) {
		k -= len(h)
	}
	return h[k]
}

// push takes one input sample and reports whether it completed a
// discriminator sample, which it returns, with the power of the prefilter
// output.
func (f *leanFront) push(v complex64) (s, power float32, ok bool) {
	f.gAcc += real(v)*real(v) + imag(v)*imag(v)
	if f.gN++; f.gN == frontBlock {
		if rms := sqrtf(f.gAcc / frontBlock); rms > 0 {
			if g := frontRMS / rms; g > 2*f.gain || g < f.gain/2 {
				f.gain = g
				f.inv2 = 1 / (g * g)
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
	// The matched filter keeps the discriminator samples at discRate and
	// gives every other output, at half of it: as at the 12 kHz of
	// iqRx, the core samples the SYNC again from few enough of them.
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
