//go:build !tinygo

package fsktx

import (
	"math"
	"testing"
)

// fidelityLimit returns the most FSK error of u, in percent: about 1.5
// times what the matrix measures for its modulator, which is the most at
// the 24 kHz shaper of four symbols on each side of the Tier0 I/Q.
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

// fidelityUnits returns the units of the fidelity tests: every modulator
// and rate at the nominal setup, the audio ones at the levels from 100 and
// at the extreme offsets, and the I/Q ones at the extreme carrier
// offsets.
func fidelityUnits(full bool) []txUnit {
	units := nominal(full)
	for _, u := range levelUnits(full) {
		if u.level >= 100 && (u.offset != 0 || u.level != tableUnits) && (full || u.rate == 48000 || u.rate == 24000) {
			units = append(units, u)
		}
	}
	for _, u := range offsetUnits(full) {
		if u.ramp == 1 && u.offHz != 0 && (full || u.rate <= 192000) {
			u.ramp = max(u.rate/1000, 1)
			units = append(units, u)
		}
	}
	return units
}

// eyeError returns the largest error of the receive-filtered deviation of
// x at the centre of any symbol, after the fit f, as a share of the 1296
// Hz between two levels: an error of 0.5 would decide a symbol wrong
// without noise.
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

// TestTxMatrixFidelity checks for every modulator the deviation test of
// ETSI TS 102 361-1 clause 10.2.2.3, 2749 Hz within 10 percent on +3 +3
// -3 -3 repeated, and the FSK error, the deviation and the carrier
// frequency on a BS outbound stream (clause 10.2.2), with an ideal DAC for
// the audio modulators, and the eye: the error of the worst symbol stays
// within 0.15 of the gap between two levels, so that an ideal
// receiver decides every symbol right with a margin.
func TestTxMatrixFidelity(t *testing.T) {
	t.Parallel()
	full := txFull()
	ts := newTestStream(t, 2, 40)
	for _, u := range fidelityUnits(full) {
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

// TestTxMatrixSpectrum checks the adjacent and alternate channel power and
// the margin to Emission Mask D of every modulator, as TestSpectrum does,
// about the carrier: the audio ones through the model of a DAC and a
// radio, an ideal DAC below 24 kHz, and the I/Q ones at their rate. The
// 8-bit I/Q below 1 MHz is out of the test: its noise spreads over too
// narrow a rate to meet the limit, as AppendS8 says.
func TestTxMatrixSpectrum(t *testing.T) {
	t.Parallel()
	full := txFull()
	ts := newTestStream(t, 3, 40)
	for _, u := range fidelityUnits(full) {
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
