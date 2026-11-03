//go:build !tinygo

package rf

import (
	"strconv"
	"strings"
	"testing"
)

// TestRxZeroFilledGaps receives an MS call whose gaps between bursts are
// exact zeros, as gated audio taps and SDR software that zero-fills give
// them, at every I/Q tier and input format, with no noise.
func TestRxZeroFilledGaps(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"cs16-48k", "cu8-48k", "cs16-2048k", "cu8-2048k", "cf32-48k"} {
		for _, off := range []float64{0, 1500} {
			t.Run(format+"/"+strconv.FormatFloat(off, 'f', 0, 64), func(t *testing.T) {
				t.Parallel()
				mf := format
				if mf == "cf32-48k" {
					mf = "cs16-48k"
				}
				row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom."+mf+".inf")
				s := lateEntrySignal(t, 0, 0.008)
				s.parts[0].hz = off
				in := mxInputOf(s, row)
				if format == "cf32-48k" {
					x := s.baseband()
					in.iq, in.iqFmt = cf32(x), IQFormatCF32
				}
				for _, x := range mxReceivers(mf, true) {
					got, dev, err := mxReceive(in, x)
					if err != nil {
						t.Fatal(err)
					}
					r := &mxResult{}
					mxScore(s, in, x.tier, got, dev, false, r)
					delete(r.fail, "missing")
					if len(r.fail) > 0 {
						t.Errorf("%s: %s: %s", x.name(), r.symptoms(), strings.Join(r.detail, " | "))
					}
				}
			})
		}
	}
}
