//go:build !tinygo

package rf

import (
	"math"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func lateEntrySignal(t *testing.T, ppm, cut float64) *mxSignal {
	t.Helper()
	return lateEntryOf(t, randomizedN(t, 3190001, lateEntryConfig()), ppm, cut)
}

func lateEntryConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 1, Source: 3190001, Dest: 9990, Message: "DMRGO REPLAY", Superframes: 2, LateEntry: true}
}

func lateEntryOf(t *testing.T, cfg rftx.SeqConfig, ppm, cut float64) *mxSignal {
	t.Helper()
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	f, e := rftx.Modulate(built.Bursts, rftx.Schedule{})
	s := &mxSignal{r: 1 + ppm*1e-6, step: math.Inf(1), gain: [2]float64{1, 1}}
	t0 := -cut / s.r
	s.parts = []mxPart{{f: f, e: e, t0: t0, amp: 1}}
	ramp := (slotSeconds - burstSeconds) / 2
	for i, b := range built.Bursts {
		s.truth = append(s.truth, mxTruth{at: t0 + (float64(2*i)*slotSeconds+ramp)/s.r, data: b, g: 2 * i})
	}
	s.end = t0 + float64(len(f))/basebandRate/s.r + mxTail
	s.truth = slices.DeleteFunc(s.truth, func(tr mxTruth) bool { return tr.at < 0 })
	return s
}

var replayBurstRef = regexp.MustCompile(`#[0-9]+`)

func TestRxReplayOneSync(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"cs16-48k", "a48000"} {
		row := mxRowOf(t, "s0.none.+0.ms.voice.0ppm.nom."+format+".inf")
		var want [Tier3 + 1]string
		for _, ppm := range []float64{0, 500, -500} {
			s := lateEntrySignal(t, ppm, 0.008)
			in := mxInputOf(s, row)
			for _, x := range mxReceivers(format, true)[1:] {
				got, dev, err := mxReceive(in, x)
				if err != nil {
					t.Fatal(err)
				}
				r := &mxResult{}
				mxScore(s, in, x.tier, got, dev, false, r)
				miss := ""
				for _, d := range r.detail {
					if m, ok := strings.CutPrefix(d, "missing: "); ok {
						miss = strings.Join(replayBurstRef.FindAllString(m, -1), " ")
					}
				}
				delete(r.fail, "missing")
				if ppm == 0 {
					want[x.tier] = miss
				}
				if len(r.fail) > 0 || miss != want[x.tier] {
					t.Errorf("%s %+.0f ppm %s: %s; missing %q, at 0 ppm %q: %s", format, ppm, x.name(), r.symptoms(), miss, want[x.tier], strings.Join(r.detail, " | "))
				}
			}
		}
	}
}
