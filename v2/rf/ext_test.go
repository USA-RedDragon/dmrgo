package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// TestSoftExtEmbeddedLC receives a BS stream through a DiscReceiver with and
// without softExt at 6.2 dB Eb/N0, where the hard decisions of the
// embedded LC fragments rarely decode, and checks that the soft decoding
// of the four fragments of each superframe recovers most of them.
func TestSoftExtEmbeddedLC(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 5, 41)
	a := discAudio(x, benchAudioHz)
	elc := func(soft bool) float64 {
		var out []Burst
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, OnBurst: func(b *Burst) { out = append(out, *b) }})
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

// TestSoftExtLookback enters a BS stream at a voice burst B, with no SYNC
// before it, and checks that softExt with lookback decodes the slots
// before the first SYNC that the core locks to, which the core alone
// loses: voice bursts B, C and D place by the EMB of burst B once the
// first run of the lookback slots knows the color code.
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
	// first returns the time of the first burst, and of the first voice
	// burst B.
	first := func(lookback int) (start, b float64) {
		start, b = -1, -1
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, OnBurst: func(x *Burst) {
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
