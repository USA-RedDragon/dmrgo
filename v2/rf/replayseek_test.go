//go:build !tinygo

package rf

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
)

// TestRxReplaySeek checks that Tiers 1 to 3 receive, with every bit right,
// a second MS talker that has no SYNC in the input before the lock: both
// talkers start before the input, the lock comes on the SYNC of the next
// superframe of the first, and the late-entry replay before it finds the
// second talker on its EMB, 0.33 symbol and 200 Hz off the first. The
// bursts of the second talker after the lock then come on the timing that
// the replay found. Only the first bursts of the replay can stay missing:
// an EMB seek of burst C or D takes the EMB of the slot two before it too.
func TestRxReplaySeek(t *testing.T) {
	t.Parallel()
	replaySeek(t, func(src uint32, cfg rftx.SeqConfig) rftx.SeqConfig { return randomizedN(t, uint64(src), cfg) })
}

// TestRxReplaySeekEMBTiming pins payloads on which the EMB seek put the
// second talker 7 to 9 us off when it timed the slot on the peak of the
// correlation over the 24 EMB symbols alone.
func TestRxReplaySeekEMBTiming(t *testing.T) {
	t.Parallel()
	for _, run := range []uint64{12108563718879219994, 14370708237565064112} {
		s := testscen.Of(runMix(run, nameHash("TestRxReplaySeek")))
		replaySeek(t, func(src uint32, cfg rftx.SeqConfig) rftx.SeqConfig {
			return testscen.Of(runMix(s.VoiceSeed, uint64(src))).Apply(cfg)
		})
	}
}

func replaySeek(t *testing.T, payload func(uint32, rftx.SeqConfig) rftx.SeqConfig) {
	row := mxRowNamed(t, "mid.none.+0.ms2.voice.0ppm.nom.cs16-48k.inf")
	s := &mxSignal{r: 1, seed: row.seed(), step: math.Inf(1), gain: [2]float64{1, 1}}
	ramp := (slotSeconds - burstSeconds) / 2
	for who, c := range []struct {
		src      uint32
		slot     int
		at, frac float64
		hz       float64
	}{{3009101, 0, 0, 0, 0}, {3009102, 1, -6 * slotSeconds, 0.33, -200}} {
		cfg := payload(c.src, rftx.SeqConfig{ColorCode: 1, Source: c.src, Dest: 9990, Message: "DMRGO REPLAY", Superframes: 2, Slot: c.slot, LateEntry: true})
		built, err := rftx.Sequence("voice", cfg)
		if err != nil {
			t.Fatal(err)
		}
		f, e := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: c.slot})
		t0 := c.at - mxCutSeconds + c.frac/symbolRate
		s.parts = append(s.parts, mxPart{f: f, e: e, t0: t0, amp: 1, hz: c.hz})
		for i, b := range built.Bursts {
			s.truth = append(s.truth, mxTruth{at: t0 + float64(2*i+c.slot)*slotSeconds + ramp, data: b, who: who, g: 2*i + c.slot})
		}
		s.end = max(s.end, t0+float64(len(f))/basebandRate)
	}
	s.end += mxTail
	s.truth = slices.DeleteFunc(s.truth, func(tr mxTruth) bool { return tr.at < 0 || tr.at+burstSeconds > s.end })
	slices.SortFunc(s.truth, func(a, b mxTruth) int { return cmp.Compare(a.at, b.at) })
	in := mxInputOf(s, row)
	for _, x := range mxReceivers(row.val(mxFormat), true) {
		if x.tier == Tier0 {
			continue
		}
		got, dev, err := mxReceive(in, x)
		if err != nil {
			t.Fatal(err)
		}
		res := &mxResult{}
		mxScore(s, in, x.tier, got, dev, false, res)
		late := false
		for j, tr := range s.truth {
			if tr.at < 3*slotSeconds+0.02 {
				continue
			}
			if !slices.ContainsFunc(got, func(b Burst) bool { return mxNearest(s, b.Seconds+in.delay) == j }) {
				late = true
			}
		}
		if late || res.fail["bits"] || res.fail["extra"] || res.fail["seconds"] || res.fail["slot"] {
			t.Errorf("%s: %s", x.name(), strings.Join(res.detail, " | "))
		}
	}
}
