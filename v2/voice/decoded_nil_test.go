package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestDecodedNilBurst(t *testing.T) {
	t.Parallel()

	vrx, err := voice.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	drx, err := data.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	pre := csbk.NewPreambles(nil)
	for _, h := range []layer2.Hint{{}, {SyncLost: true, End: layer2.EndStrong}} {
		if got := vrx.Decoded(0, nil, h, nil); len(got) != 0 {
			t.Errorf("voice: %+v", got)
		}
		if got := drx.Decoded(0, nil, h, nil); len(got) != 0 {
			t.Errorf("data: %+v", got)
		}
		if got := pre.Decoded(0, nil, h, nil); len(got) != 0 {
			t.Errorf("preambles: %+v", got)
		}
	}
}
