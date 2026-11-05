//go:build !tinygo

package rf

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestAudioColdStart(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"s0.none.+700.ms.data12.+500ppm.high.a12000.inf",
		"s0.none.-3000.ms.csbk.0ppm.high.a9600.40dB",
		"s0.none.+1500.ms.data1.0ppm.high.a9600.inf",
		"gap.none.-700.ms.data12.0ppm.high.a48000.40dB",
		"gap.none.-3000.ms.rc.0ppm.low.a24000.40dB",
	} {
		row := mxRowOf(t, name)
		s, err := mxBuild(t, row)
		if err != nil {
			t.Fatal(err)
		}
		in := mxInputOf(s, row)
		s.truth = slices.DeleteFunc(s.truth, func(tr mxTruth) bool { return !payloadInInput(tr.at - in.delay) })
		n := 5 * in.hz / 1000
		pre := make([]int16, n, n+len(in.audio))
		for i := range pre {
			pre[i] = in.audio[0]
		}
		in.audio = append(pre, in.audio...)
		in.delay -= float64(n) / float64(in.hz)
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			got, dev, err := mxReceive(in, mxRx{row.val(mxFormat), tier})
			if err != nil {
				t.Fatal(err)
			}
			res := &mxResult{}
			mxScore(s, in, tier, got, dev, row.val(mxCN) != "inf", res)
			if res.fail["missing"] || res.fail["bits"] {
				t.Errorf("%s t%d: %s", name, tier-Tier0, strings.Join(res.detail, " | "))
			}
		}
	}
}

func TestPayloadInInputEdge(t *testing.T) {
	t.Parallel()
	if payloadEdgeBits != math.Float64bits(-0.5/symbolRate) {
		t.Fatalf("payloadEdgeBits %#x, want %#x", uint64(payloadEdgeBits), math.Float64bits(-0.5/symbolRate))
	}
	for _, c := range []struct {
		s  float64
		in bool
	}{{1, true}, {0, true}, {math.Copysign(0, -1), true}, {-0.5 / symbolRate, true}, {-0.6 / symbolRate, false}, {-1, false}} {
		if got := payloadInInput(c.s); got != c.in {
			t.Errorf("payloadInInput(%g) = %v, want %v", c.s, got, c.in)
		}
	}
}
