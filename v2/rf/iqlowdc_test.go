//go:build !tinygo

package rf

import "testing"

// TestIQLowDCOffset checks the DC removal of the low-rate I/Q receiver: a
// constant I/Q offset as strong as a low-level carrier, with no lead, a
// noise lead or a carrier lead on the tuned frequency, and a carrier lead
// with no offset, must not cost a burst or a bit at Tier 0.
func TestIQLowDCOffset(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"s0.none.+0.ms.voice.0ppm.low.cs16-9600.40dB.dc",
		"s0.noise.-1500.ms.voice.0ppm.low.cs16-9600.40dB.dc",
		"s0.cw500ms.+0.ms.voice.0ppm.nom.cs16-9600.40dB.dc",
		"s0.cw2s.+0.ms.voice.0ppm.high.cs16-9600.40dB",
		"agc.cw2s.+1500.ms.data34.0ppm.low.cs16-9600.inf.dc",
	} {
		row := mxRowOf(t, name)
		res, err := mxRun(row, []mxRx{{row.val(mxFormat), Tier0}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if s := r.symptoms(); s != "" {
				t.Errorf("%s: %s", name, s)
			}
		}
	}
}
