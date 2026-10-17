package rf

import "testing"

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
			r.ext = newSoftExt()
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
