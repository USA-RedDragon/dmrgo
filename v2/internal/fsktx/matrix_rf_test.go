//go:build !tinygo

package fsktx

import (
	"math"
	"testing"
)

func (u txUnit) fidelityLimit() float64 {
	tier := u.tier
	if u.mcu {
		tier = Tier0
	}
	if u.audio() {
		return map[Tier]float64{Tier0: 0.7, Tier1: 0.6, Tier2: 0.3}[tier]
	}
	return map[Tier]float64{Tier0: 2.2, Tier1: 0.7, Tier2: 0.45}[tier]
}

func fidelityUnits(full bool) []txUnit {
	units := nominal(full)
	for _, u := range raceUnits(levelUnits(full)) {
		if u.level >= 100 && (u.offset != 0 || u.level != tableUnits) && (full || u.rate == 48000 || u.rate == 24000) {
			units = append(units, u)
		}
	}
	for _, u := range raceUnits(offsetUnits(full)) {
		if u.ramp == 1 && u.offHz != 0 && (full || u.rate <= 192000) {
			u.ramp = max(u.rate/1000, 1)
			units = append(units, u)
		}
	}
	return units
}

func eyeError(x []complex128, syms []int8, f fskResult) float64 {
	z := fskAt(demod(x, rxRate), f.Delay, 0, len(syms))
	gain := 1 + f.DevError/100
	worst := 0.0
	for k := 40; k < len(syms)-40; k++ {
		e := math.Abs(z[k]-gain*float64(syms[k])*DeviationHz-f.OffsetHz) / (2 * gain * DeviationHz)
		worst = max(worst, e)
	}
	return worst
}

// ETSI TS 102 361-1 clause 10.2.2.3; clause 10.2.2
func TestTxMatrixFidelity(t *testing.T) {
	t.Parallel()
	full := txFull()
	ts := newTestStream(t, 2, 40)
	for _, u := range raceUnits(fidelityUnits(full)) {
		if u.rate > 2400000 && !full {
			continue
		}
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			r := radio{ideal: true}
			if dev := etsiDeviation(u.signal48(u.run(t, u.pattern(patETSI, 4000), keyedOn()), r)); math.Abs(dev) > 10 {
				t.Errorf("deviation %+.1f percent of 2749 Hz", dev)
			}
			x := u.signal48(u.run(t, ts.syms, keyedOn()), r)
			f := fsk(x, ts.syms)
			eye := eyeError(x, ts.syms, f)
			t.Logf("FSK error %.3f percent, deviation %+.2f percent, carrier %+.2f Hz off, eye error %.3f", f.FSKError, f.DevError, f.OffsetHz, eye)
			if f.FSKError > u.fidelityLimit() || math.Abs(f.DevError) > 5 || math.Abs(f.OffsetHz) > 5 || eye > 0.15 {
				t.Errorf("FSK error %.2f percent (limit %.2f), deviation %+.2f percent, carrier %+.2f Hz off, eye error %.3f", f.FSKError, u.fidelityLimit(), f.DevError, f.OffsetHz, eye)
			}
		})
	}
}

func TestTxMatrixSpectrum(t *testing.T) {
	t.Parallel()
	full := txFull()
	ts := newTestStream(t, 3, 40)
	for _, u := range raceUnits(fidelityUnits(full)) {
		if u.rate > 2400000 && !full || u.format == fmtS8 && u.rate < 1000000 {
			continue
		}
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			raw := u.run(t, ts.syms, keyedOn())
			var x []complex128
			rate := float64(u.rate)
			if u.audio() {
				x, rate = u.audioFM(raw, radio{ideal: u.rate < 24000}), dacRate
			} else {
				x = shift(iqComplex(raw), rate, -float64(u.offHz))
			}
			sp := newSpectrum(x[len(x)/8:len(x)*7/8], rate)
			t.Logf("adjacent %.1f dBc, alternate %.1f dBc, mask margin %.1f dB", sp.acp(12500), sp.acp(25000), sp.maskMargin())
			checkSpectrum(t, u, sp)
		})
	}
}
