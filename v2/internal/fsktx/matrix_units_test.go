//go:build !tinygo

package fsktx

import "math"

func (u txUnit) offsetRange() (lo, hi int) {
	v := u
	v.offset = 0
	a, b := v.audioBounds()
	if u.mcu || u.tier == Tier0 {
		return int(math.MinInt16 - a), int(math.MaxInt16 - b)
	}
	return int(math.Ceil(math.MinInt16 - a)), int(math.Floor(math.MaxInt16 - b))
}

func audioRates(mcu bool, tier Tier, full bool) []int {
	if mcu || tier == Tier0 {
		return []int{24000, 48000}
	}
	r := []int{8000, 9600, 11025, 24000, 44100, 48000, 65536}
	if full {
		r = append(r, 12000, 16000, 22050, 32000, 96000, 192000)
	}
	return r
}

func iqRates(mcu bool, tier Tier, full bool) []int {
	if mcu || tier == Tier0 {
		return []int{48000, 96000, 192000}
	}
	r := []int{48000, 48001, 96000, 1000000, 2400000}
	if full {
		r = append(r, 250000, 2000000, 8000000, 10000000, 20000000)
	}
	return r
}

func audioKinds() []txUnit {
	return []txUnit{
		{mcu: true, tier: Tier0, format: fmtAudio, level: tableUnits},
		{tier: Tier0, format: fmtAudio, level: tableUnits},
		{tier: Tier1, format: fmtAudio, level: tableUnits},
		{tier: Tier2, format: fmtAudio, level: tableUnits},
	}
}

func iqKinds() []txUnit {
	return []txUnit{
		{mcu: true, tier: Tier0, format: fmtS16},
		{tier: Tier0, format: fmtS16}, {tier: Tier0, format: fmtS8},
		{tier: Tier1, format: fmtS16}, {tier: Tier1, format: fmtS8},
		{tier: Tier2, format: fmtS16}, {tier: Tier2, format: fmtS8},
	}
}

func nominal(full bool) []txUnit {
	var out []txUnit
	for _, k := range audioKinds() {
		for _, r := range audioRates(k.mcu, k.tier, full) {
			k.rate = r
			out = append(out, k)
		}
	}
	for _, k := range iqKinds() {
		for _, r := range iqRates(k.mcu, k.tier, full) {
			k.rate = r
			k.ramp = int(math.Round(1e-3 * float64(r)))
			out = append(out, k)
		}
	}
	return out
}

func levelUnits(full bool) []txUnit {
	var out []txUnit
	for _, k := range audioKinds() {
		for _, r := range audioRates(k.mcu, k.tier, full) {
			if !full && !k.mcu && k.tier != Tier0 && r != 8000 && r != 44100 && r != 48000 && r != 65536 {
				continue
			}
			k.rate = r
			for _, l := range []int{1, 100, tableUnits, 16000} {
				u := k
				u.level = l
				lo, hi := u.offsetRange()
				for _, o := range []int{0, lo, hi} {
					u.offset = o
					out = append(out, u)
				}
			}
			u := k
			u.level, u.offset = 1040, 2048
			out = append(out, u)
			if k.mcu || k.tier == Tier0 {
				u.level = 1080
				out = append(out, u)
			}
		}
	}
	return out
}

func offsetUnits(full bool) []txUnit {
	var out []txUnit
	for _, k := range iqKinds() {
		for _, r := range iqRates(k.mcu, k.tier, full) {
			k.rate = r
			maxRamp := int(math.Round(0.01 * float64(r)))
			if k.mcu {
				maxRamp = 1 << 15
			}
			offs := []int{0, 1200, -2500, (r - 1) / 4, -(r - 1) / 4}
			ramps := []int{1, int(math.Round(1e-3 * float64(r))), maxRamp}
			if !full {
				offs, ramps = []int{0, -2500, (r - 1) / 4, -(r - 1) / 4}, []int{1, maxRamp}
			}
			for _, f := range offs {
				for _, ramp := range ramps {
					u := k
					u.offHz, u.ramp = f, ramp
					out = append(out, u)
				}
			}
		}
	}
	return out
}
