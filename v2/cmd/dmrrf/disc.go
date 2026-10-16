package main

import "math"

// Limits of the discriminator tier.
const (
	// sincTaps is the length of the fractional delay interpolator.
	sincTaps = 16
	// discAnchor is the discriminator SYNC correlation of an anchor.
	discAnchor = 0.6
	// levelForget weighs the old level estimates per anchor.
	levelForget = 0.8
	// discAnchorBits is the most bits of the sliced SYNC field of an
	// anchor that differ from its pattern, so that a field such as the
	// embedded signalling of a voice burst E that correlates with the
	// pattern through noise does not count as an anchor.
	discAnchorBits = 12
)

// discSyncErrors returns the number of bits where the sliced symbols of
// the SYNC field of slot s, tau samples off, differ from syms.
func (r *Receiver) discSyncErrors(s *slotState, tau float64, syms *[syncSymbols]int8) int {
	step := r.devScale * deviationHz
	n := 0
	for k, want := range syms {
		x := (float64(r.chz.sym.lerp(r.symLag(r.symTime(s.index, syncFirstSym+k)+tau))) - r.dcHz) / step
		var got int8
		switch {
		case x >= 2:
			got = 3
		case x >= 0:
			got = 1
		case x >= -2:
			got = -1
		default:
			got = -3
		}
		if got != want {
			n += symbolBitErrors(got, want)
		}
	}
	return n
}

// symbolBitErrors returns the number of bits where the dibits of symbols a
// and b differ.
func symbolBitErrors(a, b int8) int {
	a0, a1 := dibitOfDigit(int(a+3) / 2)
	b0, b1 := dibitOfDigit(int(b+3) / 2)
	return int(a0^b0) + int(a1^b1)
}

// symLag returns the discriminator sample that matches channel sample t.
func (r *Receiver) symLag(t float64) float64 {
	return r.chz.symIndex(t)
}

// measureDisc classifies the SYNC field of slot s on the discriminator
// output and, for an anchor, updates the timing, the carrier offset and
// the deviation scale from it.
func (r *Receiver) measureDisc(s *slotState) {
	var best float32
	bestTau := 0.0
	data := false
	for tau := -3.0; tau <= 3; tau += 0.5 {
		t := r.symLag(r.symTime(s.index, syncFirstSym) + tau)
		base := int64(math.Floor(t))
		rho2, src, d := r.search.discAt(&r.chz.sym, base, float32(t-float64(base)), r.chz.symStep(), r.nsrc)
		if src == r.src && rho2 > best {
			best, bestTau, data = rho2, tau, d
		}
	}
	s.present = r.noise == 0 || r.slotEnergy(s) > 1.5*r.noise
	s.anchor = best >= discAnchor*discAnchor
	if !s.anchor {
		return
	}
	voice, dp, _ := sourceAt(r.src).SyncPatterns()
	s.a.pattern = voice
	if data {
		s.a.pattern = dp
	}
	var syms [syncSymbols]int8
	syncSyms(s.a.pattern, &syms)
	if r.amp != 0 && r.discSyncErrors(s, bestTau, &syms) > discAnchorBits {
		s.anchor = false
		return
	}
	r.timing(s, bestTau)
	var mp, mx float64
	var x [syncSymbols]float64
	for k := range syncSymbols {
		x[k] = float64(r.chz.sym.lerp(r.symLag(r.symTime(s.index, syncFirstSym+k))))
		mx += x[k]
		mp += float64(syms[k])
	}
	mp /= syncSymbols
	mx /= syncSymbols
	var sxy, spp float64
	for k := range syncSymbols {
		sxy += (float64(syms[k]) - mp) * (x[k] - mx)
		spp += (float64(syms[k]) - mp) * (float64(syms[k]) - mp)
	}
	g := sxy / spp / deviationHz
	dc := mx - g*deviationHz*mp
	var res float64
	for k := range syncSymbols {
		e := x[k] - dc - g*deviationHz*float64(syms[k])
		res += e * e
	}
	res /= syncSymbols
	if r.amp == 0 {
		r.devScale, r.dcHz, r.discNoise = g, dc, res
		r.amp, r.noise = math.Sqrt(r.slotEnergy(s)), r.slotEnergy(s)/4
	} else {
		r.devScale = levelForget*r.devScale + (1-levelForget)*g
		r.dcHz = levelForget*r.dcHz + (1-levelForget)*dc
		r.discNoise = levelForget*r.discNoise + (1-levelForget)*res
	}
}

// discDetect sets the symbol log-likelihoods of slot s from the
// discriminator output, as Gaussian around the four levels, with a click
// outlier of Options.DiscOutlier spread over the basebandRate Hz of the
// discriminator range as their floor.
func (r *Receiver) discDetect(s *slotState) {
	v := math.Max(r.discNoise, 1)
	g := -1 / float32(2*v)
	po := r.opt.DiscOutlier
	floor := float32(math.Inf(-1))
	if po > 0 {
		floor = float32(math.Log(po/basebandRate) - math.Log((1-po)/math.Sqrt(2*math.Pi*v)))
	}
	dc, dev := float32(r.dcHz), float32(r.devScale*deviationHz)
	t0 := r.symLag(r.symTime(s.index, 0))
	base := int64(math.Floor(t0))
	u, du := float32(t0-float64(base)), float32(r.period/slotSymbols/float64(r.chz.discDec))
	st, p := r.symsOf(s)
	var a [4]float32
	for k := range slotSymbols {
		x := r.chz.sym.lerpFrom(base, u+float32(k)*du) - dc
		for d := range a {
			e := x - dev*float32(2*d-3)
			a[d] = max(e*e*g, floor)
		}
		st.set(p+k, a[:])
	}
}
