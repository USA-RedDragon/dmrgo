package rf

const (
	devGainTiers = 3
	devGainRates = 2
	devGainN     = 33
	devGainStep  = 250
	devGainOne   = 1 << 15
)

func devTabOf(o *options) uint8 {
	if o.IQFront > 0 {
		return 0
	}
	for k, t := range [devGainTiers]Tier{Tier0, Tier1, Tier2} {
		w := tierOptions(t)
		if o.IQHz == w.IQHz && o.IQTaps == w.IQTaps && o.IQKaiser == w.IQKaiser && o.IQCutHz == w.IQCutHz && o.IQHalfband == w.IQHalfband {
			return uint8(k + 1)
		}
	}
	return 0
}

func devGainHz(i int) float64 { return float64((i - devGainN/2) * devGainStep) }

func (r *iqRx) deviation() float64 {
	d := r.d.deviation()
	if r.devTab == 0 {
		return d
	}
	u := float32(r.d.offset())/(iqUnitsPerHz*devGainStep) + devGainN/2
	if !(u > 0) {
		u = 0
	}
	u = min(u, devGainN-1)
	i := min(int(u), devGainN-2)
	w := u - float32(i)
	t := r.devTab - 1
	a, b := float32(devGainIQ[t][i]), float32(devGainIQ[t][i+1])
	return d * devGainOne / float64(a+w*(b-a))
}

func (r *iqLowRx) deviation() float64 {
	g := float32(devGainLow[1])
	if r.d.iqLow {
		g = float32(devGainLow[0])
	}
	return float64(float32(r.d.devHz) / r.d.f.gain * (devGainOne / g))
}
