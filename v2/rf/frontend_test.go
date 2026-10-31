//go:build !tinygo

package rf

import (
	"encoding/binary"
	"math"
	"math/cmplx"
	"testing"
)

func frontToneDelay(rate, hz float64) float64 {
	n := int(rate * 0.2)
	in := make([]byte, 4*n)
	for i := range n {
		p := 2 * math.Pi * hz * float64(i) / rate
		binary.LittleEndian.PutUint16(in[4*i:], uint16(int16(20000*math.Cos(p))))
		binary.LittleEndian.PutUint16(in[4*i+2:], uint16(int16(20000*math.Sin(p))))
	}
	out := make([]complex64, n)
	w := newFrontEnd(rate, IQFormatCS16, basebandRate).process(in, out)
	var acc complex128
	for k := w / 2; k < w; k++ {
		acc += complex128(out[k]) * cmplx.Exp(complex(0, -2*math.Pi*hz*float64(k)/basebandRate))
	}
	return -cmplx.Phase(acc) / (2 * math.Pi * hz)
}

func iqLagSeconds(t *testing.T, rate float64, tier Tier) float64 {
	t.Helper()
	r, err := newReceiver(RxConfig{Rate: int(rate), Tier: tier, Format: IQFormatCS16}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := r.full.(*rxHost)
	if !ok || h.iqd == nil {
		t.Fatalf("%.0f Hz %v: no host I/Q receiver", rate, tier)
	}
	return float64(h.iqd.d.lagTicks) / 256 / basebandRate
}

func TestFrontDelay(t *testing.T) {
	t.Parallel()
	const hz = 2000
	base := frontToneDelay(basebandRate, hz)
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		lag0 := iqLagSeconds(t, basebandRate, tier)
		for _, rate := range []float64{1.024e6, 2.048e6, 2.4e6} {
			delay := frontToneDelay(rate, hz) - base
			lag := iqLagSeconds(t, rate, tier) - lag0
			if math.Abs(delay-lag) > 0.1e-6 {
				t.Errorf("%v %.3f MHz: front delay %+.3f us, receiver lag %+.3f us", tier, rate/1e6, delay*1e6, lag*1e6)
			}
		}
	}
}
