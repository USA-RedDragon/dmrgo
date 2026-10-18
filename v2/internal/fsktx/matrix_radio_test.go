//go:build !tinygo

package fsktx

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// radioPath is an analog path from a DAC to the FM modulator of a radio,
// and the filter at the output rate that makes up for it, if any. The
// filters of comp are the ones of TestTxStudy; a modulator that makes up
// for the path itself takes the place of comp.
type radioPath struct {
	name string
	r    radio
	comp func(hz int) biquad
	// limit is the most FSK error of the path, in percent, over the limit
	// of the modulator, or with abs set the most of any modulator. A path
	// with neither is out of the tests of FSK error and detection.
	limit, abs float64
}

// radioPaths returns the analog paths of the matrix: the hold and low
// pass of a DAC, an ideal DAC, AC coupling at 2, 10 and 30 Hz and a
// pre-emphasis from 300 Hz, each without and with a filter that makes up
// for it. The hold of a DAC at 24 kHz leaves about 0.6 percent of FSK
// error, the filter of a pre-emphasis 1.4 percent there, and the filter
// of AC coupling about 2.2 percent, as AC coupling at 2 Hz does without
// it.
func radioPaths() []radioPath {
	paths := []radioPath{
		{name: "hold", r: radio{}, abs: 0.8},
		{name: "ideal", r: radio{ideal: true}, limit: 1e-9},
	}
	for _, ac := range []float64{2, 10, 30} {
		paths = append(paths,
			radioPath{name: "ac" + ftoa(ac), r: radio{acHz: ac}},
			radioPath{name: "ac" + ftoa(ac) + "-comp", r: radio{acHz: ac, gain: 2}, comp: func(hz int) biquad { return half(hfUnity(shelf(ac, ac/10, float64(hz)))) }, abs: 3})
	}
	return append(paths,
		radioPath{name: "pre300", r: radio{preHz: 300, preTopHz: 3000}},
		radioPath{name: "pre300-comp", r: radio{preHz: 300, preTopHz: 3000}, comp: func(hz int) biquad { return shelf(3000, 300, float64(hz)) }, abs: 1.6})
}

// through returns the output raw of the audio unit u through the filter
// of p at its rate, and the number of samples that the filter saturates.
func (p radioPath) through(u txUnit, raw []int16) ([]int16, int) {
	if p.comp == nil {
		return raw, 0
	}
	q := p.comp(u.rate)
	out := make([]int16, len(raw))
	sat := 0
	for i, v := range raw {
		y := q.step(float64(int(v)-u.offset)) + float64(u.offset)
		if y >= math.MaxInt16 || y <= math.MinInt16 {
			sat++
		}
		out[i] = sat16(y)
	}
	return out, sat
}

// TestTxMatrixRadio runs each audio modulator through each analog path,
// and checks that a filter that makes up for the path does not saturate,
// that the FSK error stays within its bound where the path is in the
// test, and that an rf.Receiver of Tier1 detects every burst but the
// first two bit for bit at a high C/N. A path without its filter has no
// bound on its FSK error: the matrix logs it.
func TestTxMatrixRadio(t *testing.T) {
	t.Parallel()
	full := txFull()
	ts := newTestStream(t, 2, 40)
	for _, k := range audioKinds() {
		for _, rate := range audioRates(k.mcu, k.tier, full) {
			if rate < 24000 || dacRate%rate != 0 {
				continue
			}
			k.rate = rate
			for _, p := range radioPaths() {
				u := k
				t.Run(u.String()+"/"+p.name, func(t *testing.T) {
					t.Parallel()
					raw, sat := p.through(u, u.run(t, ts.syms, nil))
					if sat > 0 {
						t.Errorf("the filter saturates %d samples", sat)
					}
					x := resample(u.audioFM(raw, p.r), dacRate, rxRate)
					f := fsk(x, ts.syms)
					t.Logf("FSK error %.3f percent, deviation %+.2f percent", f.FSKError, f.DevError)
					limit := u.fidelityLimit() + p.limit
					if p.abs > 0 {
						limit = p.abs
					}
					if (p.limit > 0 || p.abs > 0) && f.FSKError > limit {
						t.Errorf("FSK error %.2f percent, limit %.2f", f.FSKError, limit)
					}
					sc := ts.rawScore(receive(noisy(x, 0, len(x)+int(2*rxPad*rxRate), 30, 5), rf.Tier1), rxPad)
					t.Logf("%d of %d bursts exact, %d of %d bits wrong", sc.exact, sc.bursts, sc.errs, sc.bits)
					if p.limit > 0 || p.abs > 0 {
						checkExact(t, u.String(), sc, 2)
					}
				})
			}
		}
	}
}
