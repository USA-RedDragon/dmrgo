package rf

import (
	"fmt"
	"slices"
	"testing"
)

func TestRawSearchOffset(t *testing.T) {
	t.Parallel()
	streams := rxStreams(t)
	conds := rxConds()
	for _, sn := range []string{"bs", "ms", "msj"} {
		for _, cname := range []string{"offset +3000 Hz", "offset -3000 Hz"} {
			t.Run(fmt.Sprintf("%s/%s", sn, cname), func(t *testing.T) {
				t.Parallel()
				c := conds[slices.IndexFunc(conds, func(c rxCond) bool { return c.name == cname })]
				in := rxBuild(streams[sn], c, 12, true, rxSeed(sn+c.name))
				el := rxEligible(in)
				var got [2][]Burst
				for i, raw := range []uint8{0, 2} {
					o := discOptions()
					o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, raw
					r, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: Tier0, opt: &o, OnBurst: func(b *Burst) { got[i] = append(got[i], *b) }})
					if err != nil {
						t.Fatal(err)
					}
					for k := 0; k < len(in.audio); k += 1024 {
						r.Write(in.audio[k:min(k+1024, len(in.audio))])
					}
					r.Flush()
				}
				if len(got[0]) == 0 || len(got[1]) == 0 {
					t.Fatalf("bursts %d and %d", len(got[0]), len(got[1]))
				}
				if d := got[1][0].Seconds - got[0][0].Seconds; d < -0.001 || d > 0.001 {
					t.Errorf("first burst at %.4f s with RawSearch 2, %.4f s without", got[1][0].Seconds, got[0][0].Seconds)
				}
				a, b := rxMatch(el, got[0]), rxMatch(el, got[1])
				if float64(b.exact) < rxFloor*float64(a.exact) {
					t.Errorf("RawSearch 2: %d of %d payloads, without: %d", b.exact, b.total, a.exact)
				}
			})
		}
	}
}
