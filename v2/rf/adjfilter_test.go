//go:build !tinygo

package rf

import (
	"strings"
	"testing"
)

// TestIQAdjacentOffCenter runs every I/Q tier on a carrier 1500 Hz off the
// channel toward a DMR channel 40 dB stronger at -12.5 kHz. The oscillator
// follows the carrier, but the channel filter stays at the channel center:
// on the carrier it keeps about 13 dB more of the skirt of the other channel, and
// the RC burst after the lock came out with 20 or more bits wrong.
func TestIQAdjacentOffCenter(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "s0.cw500ms.-1500.ms.rc.+500ppm.high.cs16-48k.inf.dmr40")
	res, err := mxRun(row, mxReceivers(row.val(mxFormat), true))
	if err != nil {
		t.Fatal(err)
	}
	for x, r := range res {
		if len(r.fail) > 0 {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}
