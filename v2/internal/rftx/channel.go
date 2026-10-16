package rftx

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
)

// Channel resamples signed 8-bit I/Q from txRate to rxRate, adds a carrier
// offset and white noise at snrDB, and returns rtl_sdr style unsigned bytes.
func Channel(iq []byte, txRate, rxRate, offsetHz, snrDB float64, seed uint64) []byte {
	x := FromHackRF(iq)
	n := int(float64(len(x)) * rxRate / txRate)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(0.81/math.Pow(10, snrDB/10)) / math.Sqrt2
	out := make([]byte, 2*n)
	for i := range n {
		pos := float64(i) * txRate / rxRate
		k := int(pos)
		v := x[k]
		if k+1 < len(x) {
			v += (x[k+1] - x[k]) * complex(pos-float64(k), 0)
		}
		v *= cmplx.Exp(complex(0, 2*math.Pi*offsetHz*float64(i)/rxRate))
		v += complex(rng.NormFloat64()*sigma, rng.NormFloat64()*sigma)
		clip := func(f float64) byte { return byte(math.Max(0, math.Min(255, math.Round(f*127.5+127.5)))) }
		out[2*i], out[2*i+1] = clip(real(v)*0.8), clip(imag(v)*0.8)
	}
	return out
}
