package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestTransmitter_ZeroSilenceIsAMBESilence(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, nil)
	if err := tx.Start(0, voice.Call{Group: true, Target: 91, Source: 3191868}, nil); err != nil {
		t.Fatal(err)
	}
	var voiced vocoder.AMBEFrame
	voiced[0] = 0x55
	if err := tx.Frame(voiced); err != nil {
		t.Fatal(err)
	}
	tx.Stop()
	frames := framesOf(t, outBursts(drain(t, tx, 100)))
	if len(frames) != 18 {
		t.Fatalf("%d frames, want 18", len(frames))
	}
	for i, f := range frames[1:] {
		if f != vocoder.AMBESilence() {
			t.Fatalf("fill frame %d is %x, want the AMBE silence frame", i+1, f)
		}
	}
}
