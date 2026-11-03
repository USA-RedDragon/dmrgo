package rf

import "testing"

// TestAudioTxTailAtRest checks that AppendFlush brings the pulse tail of
// the last burst to rest: the pulse of a symbol spans twice the lag, so a
// flush of fewer zero symbols cuts the last symbols of the transmission
// short. It fails when a sample of the last symbol period of the output is
// over 2% of the peak.
func TestAudioTxTailAtRest(t *testing.T) {
	t.Parallel()
	b := msBursts(t, "voice")[:1]
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		for _, rate := range []int{48000} {
			atx, err := newAudioTransmitter(usAudio(tier, rate, 16), openGate)
			if err != nil {
				t.Fatal(err)
			}
			x := atx.AppendFlush(atx.AppendSamples(nil, b))
			var peak, tail int
			for i, v := range x {
				a := max(int(v), -int(v))
				peak = max(peak, a)
				if i >= len(x)-rate/symbolRate {
					tail = max(tail, a)
				}
			}
			if 50*tail > peak {
				t.Errorf("Tier%d at %d Hz: last symbol period reaches %d of peak %d", tier-Tier0, rate, tail, peak)
			}
		}
	}
}
