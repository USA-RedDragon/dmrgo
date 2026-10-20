package rf

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func (f *floorSignal) useAudio(slots []rftx.BSSlot, hz int) {
	fm, env := rftx.ModulateBS(slots)
	pad := int(benchPad * basebandRate)
	n := len(f.y)
	s := make([]complex128, n)
	iq := make([]complex128, n)
	var sph float64
	for i := range n {
		if k := i - pad; k >= 0 && k < len(fm) {
			sph += 2 * math.Pi * (fm[k] + floorOffsetHz) / basebandRate
			s[i] = complex(env[k]*math.Cos(sph), env[k]*math.Sin(sph))
			if env[k] > 1e-3 {
				iq[i] = cmplx.Conj(f.zv[i] / s[i])
			}
		}
	}
	h := audioIFTaps()
	th := make([]float64, n)
	prev := 0.0
	for i := range n {
		var hs, hy complex128
		for k, t := range h {
			if j := i - len(h)/2 + k; j >= 0 && j < n {
				hs += s[j] * complex(t, 0)
				hy += iq[j] * complex(t, 0)
			}
		}
		v := prev
		if hs != 0 && hy != 0 {
			d := cmplx.Phase(hy/hs) - prev
			v = prev + d - 2*math.Pi*math.Round(d/(2*math.Pi))
		}
		th[i], prev = v, v
	}
	aa := lowpassTaps(63, min(6250, 0.42*float64(hz))/basebandRate)
	x := make([]complex128, n)
	var cph float64
	for i := range n {
		if k := i - pad; k >= 0 && k < len(fm) {
			cph += 2 * math.Pi * floorOffsetHz / basebandRate
		}
		var v float64
		for k, t := range aa {
			if j := i - len(aa)/2 + k; j >= 0 && j < n {
				v += th[j] * t
			}
		}
		x[i] = s[i] * cmplx.Exp(complex(0, v))
		f.y[i] = x[i] * complex(math.Cos(cph), -math.Sin(cph))
		f.zv[i] = cmplx.Conj(x[i]) * s[i]
		f.zp[i+1] = f.zp[i] + f.zv[i]
	}
	f.scale = 1 / matchedNoise(x, s, f.base, pad+len(fm))
}

func matchedNoise(x, s []complex128, base, end int) float64 {
	taps := rftx.RRCTaps()
	step := make([]float64, len(taps)+samplesPerSy)
	var c float64
	for i := range step {
		if i < len(taps) {
			c += taps[i]
		}
		step[i] = c
	}
	q := make([]float64, len(step))
	var q2 float64
	for i := range q {
		q[i] = step[i]
		if i >= samplesPerSy {
			q[i] -= step[i-samplesPerSy]
		}
		q2 += q[i] * q[i]
	}
	var sum, sum2 float64
	cnt := 0
	for c := base + 2*samplesPerSy - len(taps)/2; c+len(q) < end; c += samplesPerSy {
		var t float64
		for k, v := range q {
			d := x[c+k] - s[c+k]
			t += real(cmplx.Conj(d)*complex(0, 1)*s[c+k]) * v
		}
		sum += t
		sum2 += t * t
		cnt++
	}
	m := sum / float64(cnt)
	return (sum2/float64(cnt) - m*m) / q2
}
