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
)

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
		rho, src, d := r.search.discAt(&r.chz.sym, t, float64(samplesPerSy)/float64(r.chz.discDec), r.nsrc)
		if src == r.src && rho > best {
			best, bestTau, data = rho, tau, d
		}
	}
	s.present = r.noise == 0 || r.slotEnergy(s) > 1.5*r.noise
	s.anchor = best >= discAnchor
	if !s.anchor {
		return
	}
	voice, dp, _ := sourceAt(r.src).SyncPatterns()
	s.a.pattern = voice
	if data {
		s.a.pattern = dp
	}
	r.timing(s, bestTau)
	var syms [syncSymbols]int8
	syncSyms(s.a.pattern, &syms)
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
// discriminator output, as Gaussian around the four levels.
func (r *Receiver) discDetect(s *slotState) {
	g := -1 / float32(2*math.Max(r.discNoise, 1))
	dc, dev := float32(r.dcHz), float32(r.devScale*deviationHz)
	t0 := r.symLag(r.symTime(s.index, 0))
	base := int64(math.Floor(t0))
	u, du := float32(t0-float64(base)), float32(r.symLag(r.symTime(s.index, 1))-r.symLag(r.symTime(s.index, 0)))
	app := r.appOf(s)
	for k := range slotSymbols {
		x := r.chz.sym.lerpFrom(base, u+float32(k)*du) - dc
		for d := range 4 {
			e := x - dev*float32(2*d-3)
			app[4*k+d] = e * e * g
		}
	}
}
