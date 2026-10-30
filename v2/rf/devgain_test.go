package rf

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

type devGainSignal struct {
	f, e  []float64
	syncs []float64
}

func devGainBursts() (*devGainSignal, error) {
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, Message: "DMRGO DEVIATION", Superframes: 1}
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		return nil, err
	}
	s := &devGainSignal{}
	s.f, s.e = rftx.Modulate(built.Bursts, rftx.Schedule{})
	ramp := (slotSeconds - burstSeconds) / 2
	for i := range built.Bursts {
		s.syncs = append(s.syncs, float64(2*i)*slotSeconds+ramp+float64(burstSymbols/2-syncSymbols/2)/symbolRate)
	}
	return s, nil
}

func (s *devGainSignal) iq(offHz float64) []complex128 {
	x := make([]complex128, len(s.f))
	var ph, prev float64
	for i, f := range s.f {
		if i == 0 {
			prev = f
		}
		ph += 2 * math.Pi * ((f+prev)/2 + offHz) / basebandRate
		prev = f
		x[i] = cmplx.Rect(s.e[i], ph)
	}
	return x
}

func devGainDisc(x []complex128, fs float64) []float64 {
	y := make([]float64, len(x))
	for i := 1; i < len(x); i++ {
		y[i] = cmplx.Phase(x[i]*cmplx.Conj(x[i-1])) * fs / (2 * math.Pi)
	}
	return y
}

func devGainRRC(fs float64) []float64 { return rrcTapsSpan(fs/symbolRate, discSpan) }

func devGainIdealRRC(fs float64) []float64 { return rrcTapsSpan(fs/symbolRate, 16) }

func devGainFilter(x, h []float64, step int) []float64 {
	n := len(h) / 2
	y := make([]float64, 0, len(x)/step+1)
	for i := 0; i < len(x); i += step {
		var s float64
		for k, t := range h {
			if j := i + k - n; j >= 0 && j < len(x) {
				s += t * x[j]
			}
		}
		y = append(y, s)
	}
	return y
}

func devGainAt(y []float64, t float64) float64 {
	const half = 24
	i := int(math.Floor(t))
	var s float64
	for k := i - half + 1; k <= i+half; k++ {
		if k < 0 || k >= len(y) {
			continue
		}
		d := t - float64(k)
		w := 1.0
		if d != 0 {
			w = math.Sin(math.Pi*d) / (math.Pi * d)
		}
		s += w * (0.5 + 0.5*math.Cos(math.Pi*d/half)) * y[k]
	}
	return s
}

func devGainFit(y []float64, fs float64, at float64) float64 {
	var syms [syncSymbols]int8
	syncSyms(enums.SyncMSSourcedData, &syms)
	var mp, spp float64
	for _, v := range syms {
		mp += float64(v)
	}
	mp /= syncSymbols
	for _, v := range syms {
		spp += (float64(v) - mp) * (float64(v) - mp)
	}
	g := func(t0 float64) float64 {
		var sxy float64
		for k := range syncSymbols {
			sxy += (float64(syms[k]) - mp) * devGainAt(y, (t0+(float64(k)+0.5)/symbolRate)*fs)
		}
		return sxy / spp / deviationHz
	}
	best, bt := math.Inf(-1), at
	for d := -60; d <= 60; d++ {
		t := at + float64(d)*0.1/symbolRate
		if v := g(t); v > best {
			best, bt = v, t
		}
	}
	for step := 0.05 / symbolRate; step > 1e-4/symbolRate; step /= 2 {
		for _, t := range []float64{bt - step, bt + step} {
			if v := g(t); v > best {
				best, bt = v, t
			}
		}
	}
	return best
}

func devGainMean(s *devGainSignal, y []float64, fs float64) float64 {
	var sum float64
	for _, at := range s.syncs {
		sum += devGainFit(y, fs, at)
	}
	return sum / float64(len(s.syncs))
}

func devGainIdeal(s *devGainSignal, fs float64) []float64 {
	step := int(basebandRate / fs)
	y := make([]float64, 0, len(s.f)/step+1)
	for i := 0; i < len(s.f); i += step {
		y = append(y, s.f[i])
	}
	return y
}

func devGainTier(s *devGainSignal, t Tier, offHz float64) float64 {
	o := tierOptions(t)
	var d iqDisc
	d.init(basebandRate/o.IQHz, iqFilter{taps: o.IQTaps, beta: o.IQKaiser, cutHz: o.IQCutHz, half: o.IQHalfband})
	var f []float64
	for _, v := range s.iq(offHz) {
		if y, _, ok := d.push(complex64(v)); ok {
			f = append(f, float64(y)-offHz)
		}
	}
	fs := float64(o.IQHz)
	if o.IQHz == discRate {
		h := make([]float64, halfTaps)
		c := halfTaps / 2
		h[c] = float64(discHalfQ15[0]) / 32768
		for k, v := range discHalfQ15[1:] {
			h[c-2*k-1], h[c+2*k+1] = float64(v)/32768, float64(v)/32768
		}
		f, fs = devGainFilter(f, h, 2), fs/2
	}
	return devGainMean(s, devGainFilter(f, devGainRRC(fs), 1), fs)
}

func devGainLowRate(s *devGainSignal, hz int, offHz float64) float64 {
	x := s.iq(offHz)
	step := basebandRate / hz
	y := make([]complex128, 0, len(x)/step+1)
	for i := 0; i < len(x); i += step {
		y = append(y, x[i])
	}
	f := devGainDisc(y, float64(hz))
	for i := range f {
		f[i] -= offHz
	}
	return devGainMean(s, devGainFilter(f, devGainRRC(float64(hz)), 1), float64(hz))
}

func devGainQ(g, ideal float64) uint16 {
	return uint16(math.Round(g / ideal * devGainOne))
}

func devGainClamp(v *[devGainN]uint16) {
	c := devGainN / 2
	ref := float64(v[c])
	for _, dir := range []int{-1, 1} {
		last := c
		for i := c + dir; i >= 0 && i < devGainN; i += dir {
			x := float64(v[i])
			if x < 0.85*ref || x > 1.02*ref || math.Abs(x-float64(v[last])) > 0.03*ref || last != i-dir {
				v[i] = v[last]
				continue
			}
			last = i
		}
	}
}

func deriveDevGain() (iq [devGainTiers][devGainN]uint16, low [devGainRates]uint16, err error) {
	s, err := devGainBursts()
	if err != nil {
		return iq, low, err
	}
	for k, t := range []Tier{Tier0, Tier1, Tier2} {
		fs := float64(tierOptions(t).IQHz)
		if fs == discRate {
			fs /= 2
		}
		ideal := devGainMean(s, devGainFilter(devGainIdeal(s, fs), devGainIdealRRC(fs), 1), fs)
		for i := range devGainN {
			iq[k][i] = devGainQ(devGainTier(s, t, devGainHz(i)), ideal)
		}
		devGainClamp(&iq[k])
	}
	for k, hz := range []int{iq0LowHz, discRate / 2} {
		ideal := devGainMean(s, devGainFilter(devGainIdeal(s, float64(hz)), devGainIdealRRC(float64(hz)), 1), float64(hz))
		low[k] = devGainQ(devGainLowRate(s, hz, 0), ideal)
	}
	return iq, low, nil
}

func devGainHz(i int) float64 { return float64((i - devGainN/2) * devGainStep) }
