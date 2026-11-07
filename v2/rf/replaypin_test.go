//go:build !tinygo

package rf

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
)

// pinnedRun pins the draw that the run seed run gave the test name.
func pinnedRun(t testing.TB, run uint64, name string) pinnedTB {
	return pinnedTB{t, runMix(run, nameHash(name))}
}

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
	if _, _, miss := earlyCount(t, truth, 3, []uint64{6972457321712919085}, clockSignal{clockBaseband(slots, 500), 500}); miss > 0.6 {
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

// TestRxSeekAfterCohTune receives a strong MS talker that Tier 3 finds on
// its EMB after it locks on a weaker one 150 Hz above it. At the lock the
// coherent stage tunes the converter onto the locked talker while outputs
// made at the old tune still wait for the lean core. The core must take
// those outputs at the old tune: taken at the new one, the first seeked
// burst reads about the 263 Hz of the tune off, its levels set the coherent
// frequency of that talker 300 Hz off, and the coherent decisions put 30
// wrong bits in it.
func TestRxSeekAfterCohTune(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.+0.ms2.voice.0ppm.nom.cs16-48k.inf")
	x := mxRx{row.val(mxFormat), Tier3}
	out, err := mxRun(pinnedTB{t, 7213212379218616557}, row, []mxRx{x})
	if err != nil {
		t.Fatal(err)
	}
	if r := out[x]; r.fail["bits"] {
		t.Error(strings.Join(r.detail, " | "))
	}
}

// TestRxEMBNotDataSync pins a payload whose EMB and embedded LC in the SYNC
// field of a voice burst lie 12 bits from the MS data SYNC: every tier
// anchored the burst as data and rewrote its slot type and SYNC bits.
func TestRxEMBNotDataSync(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.+0.ms2.voice.0ppm.nom.cs16-48k.inf")
	rxs := mxReceivers(row.val(mxFormat), true)
	out, err := mxRun(pinnedTB{t, 3117808035017486951}, row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rxs {
		if r := out[x]; r.fail["bits"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxEMBSeekSliceTie pins a payload on which two slices of the first EMB
// of the second talker decoded exactly before the color code was known, one
// at the wrong timing with the wrong color code; taking the first of the
// two lost the talker's burst B.
func TestRxEMBSeekSliceTie(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB")
	x := mxRx{row.val(mxFormat), Tier1}
	out, err := mxRun(pinnedTB{t, 9993286574802603798}, row, []mxRx{x})
	if err != nil {
		t.Fatal(err)
	}
	if r := out[x]; r.fail["missing"] || r.fail["bits"] {
		t.Error(strings.Join(r.detail, " | "))
	}
}

// TestRxEMBNotRCSync pins a payload whose EMB field in a voice burst reads as
// an MS reverse channel SYNC: every tier anchored the burst as RC and
// rewrote its bits.
func TestRxEMBNotRCSync(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom.cs16-48k.inf")
	s := lateEntryOf(t, randomizedN(pinnedTB{t, 3972215592164023459}, 3190001, lateEntryConfig()), 0, 0.008)
	in := mxInputOf(s, row)
	in.iq, in.iqFmt = cf32(s.baseband()), IQFormatCF32
	for _, x := range mxReceivers("cs16-48k", true) {
		got, dev, err := mxReceive(in, x)
		if err != nil {
			t.Fatal(err)
		}
		r := &mxResult{}
		mxScore(s, in, x.tier, got, dev, false, r)
		if r.fail["bits"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxReplayLockOffset pins a payload on which a forced lock left the lock
// strobe 0.1 symbol off its SYNC: the replay kept that origin, fitted a
// false stretch to the eye of the slots before, and placed them up to 20 us
// off.
func TestRxReplayLockOffset(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom.cu8-48k.inf")
	s := lateEntryOf(t, randomizedN(pinnedTB{t, 5419008064897867668}, 3190001, lateEntryConfig()), 0, 0.008)
	in := mxInputOf(s, row)
	x := mxRx{"cu8-48k", Tier2}
	got, dev, err := mxReceive(in, x)
	if err != nil {
		t.Fatal(err)
	}
	r := &mxResult{}
	mxScore(s, in, x.tier, got, dev, false, r)
	if r.fail["seconds"] || r.fail["bits"] {
		t.Error(strings.Join(r.detail, " | "))
	}
}
