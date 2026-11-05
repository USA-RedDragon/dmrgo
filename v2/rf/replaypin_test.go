//go:build !tinygo

package rf

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
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

// TestRxLateFillLevels receives a clean MS voice call with zero-filled gaps
// from cu8 I/Q at 2048 kHz, on a payload where Tier 2 queues the oldest
// replayed burst until a later burst gives its position, and the levels of
// the live stream then differ from those the replay sliced it with. The
// burst must decode from the levels its symbols were taken at.
func TestRxLateFillLevels(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom.cu8-2048k.inf")
	s := lateEntryOf(t, testscen.Of(15368350369733910535).Apply(lateEntryConfig()), 0, 0.008)
	in := mxInputOf(s, row)
	for _, x := range mxReceivers("cu8-2048k", true) {
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
}

// TestCoreLockSlotSync locks core Tier 3 on a BS stream at 3 dB and +500
// ppm, on a payload and noise where a voice burst without a SYNC passes the
// sliced search and the past-sync sum of the slots before it. The lock must
// wait for a slot with its own coherent SYNC: the replay fits its stretch
// from the lock SYNC, and a lock on a slot without one leaves the replay
// unstretched, 1.15 symbols off at the oldest slot.
func TestCoreLockSlotSync(t *testing.T) {
	t.Parallel()
	ids := testscen.Of(16528811544069809204).Apply(rftx.SeqConfig{Source: 3009101, BS: 990913, Dest: 3009201, Message: "DMRGO RXBENCH"})
	slots, truth := benchStreamOf(t, ids, 6)
	if _, _, miss := earlyCount(t, truth, 3, []uint64{6972457321712919085}, clockSignal{clockBaseband(slots, 5000, 500), 500}); miss > 0.6 {
		t.Errorf("the oldest replayed slot sits %.2f symbols off", miss)
	}
}

// TestRxReplayStretchSyncs receives a clean MS voice call at -500 ppm and
// -4000 Hz on a payload where the correlation peaks of the replayed SYNCs
// sit off by the data around them, so with a stretch fitted to the peaks
// the oldest replayed bursts come 15 us early. The stretch must come from
// the timing the MM detector gives each SYNC.
func TestRxReplayStretchSyncs(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.-4000.ms.voice.-500ppm.nom.cs16-48k.inf")
	s := lateEntryOf(t, testscen.Of(43).Apply(lateEntryConfig()), -500, 0)
	s.parts[0].hz = -4000
	in := mxInputOf(s, row)
	got, dev, err := mxReceive(in, mxRx{"cs16-48k", Tier3})
	if err != nil {
		t.Fatal(err)
	}
	r := &mxResult{}
	mxScore(s, in, Tier3, got, dev, false, r)
	if r.fail["seconds"] {
		t.Error(strings.Join(r.detail, " | "))
	}
}
