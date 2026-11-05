//go:build !tinygo

package rf

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
)

// TestRxReplayPayloadSync receives a clean MS voice call that starts 8 ms
// before the input, on a payload whose burst E carries an EMB and embedded
// LC that match the MS data SYNC well enough for the sliced search. The
// coherent checks must not take that burst as a SYNC: the past-sync sum
// shows the voice SYNC of burst A on the slot grid, not one in burst E, and
// the slot placement must fit the noise of the burst itself. Tier 3 then
// receives all 12 bursts, and no tier gets a burst wrong.
func TestRxReplayPayloadSync(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom.cs16-48k.inf")
	s := lateEntryOf(t, testscen.Of(15621364793835333848).Apply(lateEntryConfig()), 0, 0.008)
	in := mxInputOf(s, row)
	for _, x := range mxReceivers("cs16-48k", true)[1:] {
		got, dev, err := mxReceive(in, x)
		if err != nil {
			t.Fatal(err)
		}
		r := &mxResult{}
		mxScore(s, in, x.tier, got, dev, false, r)
		if x.tier != Tier3 {
			delete(r.fail, "missing")
		}
		if len(r.fail) > 0 {
			t.Errorf("%s: %s: %s", x.name(), r.symptoms(), strings.Join(r.detail, " | "))
		}
	}
}
