//go:build !tinygo

package rf

import (
	"fmt"
	"math"
	"testing"
)

// TestRxLevelStep steps the level of an audio BS stream in burst 4. The CACH
// of that burst must come out exact, except at Tiers 0 and 1 for a step of
// 10 dB or more within the RRC half-span after it: there the undershoot of
// the matched filter at the step puts about 0.09·(G-1)·x(ts) on the CACH
// symbol before it, with G the gain ratio and x(ts) the signal at the step,
// and an ideal linear matched filter gets that CACH wrong on 64 of 64
// payloads at +20 dB. At 10 dB the outer level before the step and the
// inner level after it differ by 5%, and a slicer that finds the step from
// the symbol values alone gets the CACH wrong on 11 to 14 of 64 payloads.
func TestRxLevelStep(t *testing.T) {
	t.Parallel()
	for _, db := range []float64{-10, 10, 20} {
		for _, off := range []string{"+0", "-2500"} {
			row := mxRowOf(t, "s0.none."+off+".bs.data12.0ppm.nom.a24000.inf")
			for _, ms := range []float64{0, 2, 3, 8, 12} {
				for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
					t.Run(fmt.Sprintf("%+.0fdB/%s/%gms/t%d", db, off, ms, tier-Tier0), func(t *testing.T) {
						t.Parallel()
						s, err := mxBuild(t, row)
						if err != nil {
							t.Fatal(err)
						}
						const j = 4
						s.step = s.truth[j].at + ms/1000
						s.gain = [2]float64{1, math.Pow(10, db/20)}
						if db > 0 {
							s.gain = [2]float64{math.Pow(10, -db/20), 1}
						}
						from := j + 2
						if db > 0 && tier >= Tier2 {
							from = j + 4
						}
						if db > 10 {
							from++
						}
						gibbs := tier <= Tier1 && math.Abs(db) >= 10 && ms/1000*symbolRate < discSpan
						in := mxInputOf(s, row)
						got, dev, err := mxReceive(in, mxRx{"a24000", tier})
						if err != nil {
							t.Fatal(err)
						}
						ok := make([]bool, len(s.truth))
						for i := range got {
							b := &got[i]
							if k := mxNearest(s, b.Seconds+in.delay); k == j && !gibbs && b.CACH != s.truth[k].cach {
								t.Errorf("CACH of the burst with the %+.0f dB step: %d bits wrong", db, hamming(b.CACH[:], s.truth[k].cach[:]))
							}
						}
						for i := range got {
							b := &got[i]
							if k := mxNearest(s, b.Seconds+in.delay); k >= 0 && b.Data == s.truth[k].data {
								ok[k] = true
							}
						}
						for k := from; k < len(s.truth); k++ {
							if !ok[k] {
								t.Errorf("burst %d of %d after a %+.0f dB step at burst %d: not decoded exactly", k, len(s.truth), db, j)
							}
						}
						res := &mxResult{}
						last := -1
						for k, v := range ok {
							if v {
								last = k
							}
						}
						mxDeviation(s, in, tier, dev, ok, last, res)
						if len(res.detail) > 0 {
							t.Errorf("level after a %+.0f dB step: %v", db, res.detail)
						}
					})
				}
			}
		}
	}
}

// TestRxLevelStepTiming steps an audio input with a 2500 Hz offset up by
// 10 dB at 8 and 12 ms into a burst, which moves the level and the DC
// between two SYNCs. The timing loop must not follow the strobes far outside
// the eye of the held levels, or the bursts after the next SYNC come out late.
func TestRxLevelStepTiming(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.-2500.bs.data12.0ppm.nom.a24000.inf")
	for _, tier := range []Tier{Tier0, Tier1} {
		for c := range 64 {
			run, ms := uint64(c/2+1), float64(8+4*(c%2))
			s, err := mxBuild(pinnedTB{t, run}, row)
			if err != nil {
				t.Fatal(err)
			}
			const j = 4
			s.step = s.truth[j].at + ms/1000
			s.gain = [2]float64{math.Pow(10, -0.5), 1}
			in := mxInputOf(s, row)
			got, _, err := mxReceive(in, mxRx{"a24000", tier})
			if err != nil {
				t.Fatal(err)
			}
			ok := make([]bool, len(s.truth))
			for i := range got {
				if k := mxNearest(s, got[i].Seconds+in.delay); k >= 0 && got[i].Data == s.truth[k].data {
					ok[k] = true
				}
			}
			for k := j + 2; k < len(s.truth); k++ {
				if !ok[k] {
					t.Errorf("t%d run %d step at %.2f ms: burst %d not decoded exactly", tier-Tier0, run, ms, k)
				}
			}
		}
	}
}

// TestRxLevelStepOneDrop steps an audio input up by 10 dB at the start of a
// burst. The symbols that clip before the gain drop stay at the rails of the
// ring, and they must not drop the gain a second time when the next SYNC
// takes them in: the front gain ends 4 times lower, not 16.
func TestRxLevelStepOneDrop(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.none.+0.bs.data12.0ppm.nom.a24000.inf")
	gain := func(tb testing.TB, tier Tier, step bool) float32 {
		s, err := mxBuild(tb, row)
		if err != nil {
			t.Fatal(err)
		}
		s.gain = [2]float64{math.Pow(10, -0.5), math.Pow(10, -0.5)}
		if step {
			s.step, s.gain[1] = s.truth[4].at, 1
		}
		in := mxInputOf(s, row)
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: in.hz, Tier: tier})
		if err != nil {
			t.Fatal(err)
		}
		var got []Burst
		for k := 0; k < len(in.audio); k += 1024 {
			got = r.AppendBursts(got, in.audio[k:min(k+1024, len(in.audio))])
		}
		r.AppendFlush(got)
		return r.f.gain
	}
	for run := uint64(1); run <= 8; run++ {
		for _, tier := range []Tier{Tier0, Tier1} {
			tb := pinnedTB{t, run}
			if k := gain(tb, tier, true) / gain(tb, tier, false); k < 1.0/8 {
				t.Errorf("run %d t%d: front gain fell %.1f times at a 10 dB step", run, tier-Tier0, 1/k)
			}
		}
	}
}
