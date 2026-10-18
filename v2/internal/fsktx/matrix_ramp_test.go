//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

// rampUnits returns the I/Q units of the ramp test: each modulator and
// format at each rate, with ramps from one sample to the most each takes.
func rampUnits(full bool) []txUnit {
	var out []txUnit
	for _, k := range iqKinds() {
		for _, r := range iqRates(k.mcu, k.tier, full) {
			if r > 2400000 && !full {
				continue
			}
			ramps := []int{1}
			for _, ms := range []float64{0.1, 0.25, 0.5, 1, 1.25, 10} {
				ramps = append(ramps, int(math.Round(ms*1e-3*float64(r))))
			}
			if k.mcu {
				ramps = append(ramps, 1<<15)
			}
			for _, ramp := range ramps {
				u := k
				u.rate, u.ramp, u.offHz = r, ramp, 0
				out = append(out, u)
			}
		}
	}
	return out
}

// TestTxMatrixRamp keys an unmodulated carrier of each I/Q modulator up
// and down with each ramp, and measures the power ramp as ETSI EN 300 113
// clause 7.9.3.1 does: the adjacent channel transient power at 12.5 kHz
// spacing, at most 50 dB below the carrier, and the attack time from 30
// dB to 6 dB under the carrier, at least 0.1 ms; and checks that the
// carrier reaches full power within the ramp, and within the 1.25 ms of
// the guard time of a burst (ETSI TS 102 361-1 clause 10.2.3.1.1) for a
// ramp that fits it. A ramp shorter than 0.25 ms is out of the test of
// the transient, and one shorter than 0.5 ms out of the test of the
// attack time: a raised cosine ramp of length T rises from 30 dB to 6 dB
// under the carrier in about 0.39 T.
func TestTxMatrixRamp(t *testing.T) {
	t.Parallel()
	full := txFull()
	for _, u := range rampUnits(full) {
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			lead := 30
			hold := max(200, 2*u.ramp/(u.rate/SymbolRate)+40)
			syms := make([]int8, lead+hold+2*u.ramp/(u.rate/SymbolRate)+60)
			raw := u.run(t, syms, []keying{{at: lead, on: true}, {at: lead + hold, on: false}})
			x := iqComplex(raw)
			on := lead * u.rate / SymbolRate
			a := u.amplitude()
			p := a * a
			at := func(db float64) int {
				for i := on; i < len(x); i++ {
					if 10*math.Log10(real(x[i]*cmplx.Conj(x[i]))/p) >= db {
						return i
					}
				}
				return len(x)
			}
			full := at(-0.05) - on + 1
			if full > u.ramp+1 || float64(u.ramp) <= 1.25e-3*float64(u.rate) && float64(full)/float64(u.rate) > 1.25e-3 {
				t.Errorf("full power %d samples after keying, ramp %d", full, u.ramp)
			}
			if float64(u.ramp)/float64(u.rate) < 0.25e-3 {
				return
			}
			attack := float64(at(-6)-at(-30)) / float64(u.rate)
			rate := u.rate
			if rate > 192000 {
				x, rate = resample(x, rate, dacRate), dacRate
			}
			tr := transient(x, float64(rate), p)
			t.Logf("transient %.1f dBc, attack %.3f ms, full power %.3f ms", tr, attack*1e3, float64(full)/float64(u.rate)*1e3)
			if tr > -50 || attack < 1e-4 && float64(u.ramp)/float64(u.rate) >= 0.5e-3 {
				t.Errorf("transient %.1f dBc, attack %.3f ms", tr, attack*1e3)
			}
		})
	}
}
