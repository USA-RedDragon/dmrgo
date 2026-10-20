package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSoftExtEmbeddedLC(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 5, 41)
	a := discAudio(x, benchAudioHz)
	elc := func(soft bool) float64 {
		var out []Burst
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: Tier0, OnBurst: func(b *Burst) { out = append(out, *b) }})
		if err != nil {
			t.Fatal(err)
		}
		if soft {
			r.ext = newSoftExt(0, 2)
		}
		r.Write(a)
		r.Flush()
		s := scoreBench(truth, out)["elc"]
		return float64(s.Exact) / float64(max(s.Truth, 1))
	}
	hard, soft := elc(false), elc(true)
	t.Logf("embedded LC recovered: %.2f hard, %.2f with softExt", hard, soft)
	if soft < 0.8 || soft <= hard {
		t.Errorf("embedded LC recovered %.2f with softExt, %.2f without", soft, hard)
	}
}

func TestSoftExtLookbackClicks(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 30)
	a := discAudio(noisyBaseband(slots, 1400, 9, 604), benchAudioHz)
	t0 := scoreBench(truth, receiveAudioTier(t, a, Tier0))["rate12"]
	t1 := scoreBench(truth, receiveAudioTier(t, a, Tier1))["rate12"]
	if t1.Exact < t0.Exact {
		t.Errorf("rate 1/2 blocks recovered: %d of %d with Tier1, %d with Tier0", t1.Exact, t1.Truth, t0.Exact)
	}
}

func TestSoftExtLookback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	start := -1
	for i, ts := range truth {
		if i > 20 && ts.kind == "voice" && ts.pos == enums.VoiceBurstB {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("no voice burst B in the stream")
	}
	a := discAudio(noisyBaseband(slots[start:], 1400, 20, 7), benchAudioHz)
	first := func(lookback int) (start, b float64) {
		start, b = -1, -1
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: Tier0, OnBurst: func(x *Burst) {
			if start < 0 {
				start = x.Seconds
			}
			if b < 0 && x.Voice == enums.VoiceBurstB {
				b = x.Seconds
			}
		}})
		if err != nil {
			t.Fatal(err)
		}
		r.ext = newSoftExt(lookback, 2)
		r.Write(a)
		r.Flush()
		return start, b
	}
	plain, _ := first(0)
	_, b := first(4)
	if b < 0 || b >= plain {
		t.Errorf("with lookback, the first voice burst B is at %.3f s, the first burst without at %.3f s", b, plain)
	}
}
