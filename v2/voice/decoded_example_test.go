package voice_test

import (
	"fmt"
	"os"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// A monitor that feeds one channel to several machines decodes each burst
// once with layer2.Burst.DecodeFromBytesWith, with the Kind and VoiceBurst
// of the Hint, and gives the decoded burst to the Decoded method of each
// machine. DecodeOptions.ColorCode does the color code check that
// ReceiverConfig.ColorCode does in Burst. A burst that does not decode goes
// to Decoded as nil, so that its Hint still applies.
func ExampleReceiver_Decoded() {
	stream, err := os.ReadFile("../layer2/testdata/parrot_kerchunk.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	vrx, err := voice.NewReceiver(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	drx, err := data.NewReceiver(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	pre := csbk.NewPreambles(nil)
	burst := layer2.NewBurst()
	cc := enums.ColorCode(1)
	opts := layer2.DecodeOptions{ColorCode: &cc}
	vev := make([]voice.Event, 0, voice.MaxEvents)
	dev := make([]data.Event, 0, data.MaxEvents)
	pout := make([]csbk.Output, 0, csbk.MaxOutputs)
	var now time.Duration
	for off := 0; off+33 <= len(stream); off += 33 {
		raw := layer2.RawBurst(stream[off : off+33])
		now += 60 * time.Millisecond
		h := layer2.Hint{}
		opts.Kind, opts.VoiceBurst = h.Kind, h.VoiceBurst
		b := burst
		if burst.DecodeFromBytesWith(raw, &opts) != nil {
			b = nil
		}
		vev = vrx.Decoded(now, b, h, vev[:0])
		dev = drx.Decoded(now, b, h, dev[:0])
		pout = pre.Decoded(now, b, h, pout[:0])
		for _, e := range vev {
			switch e.Kind {
			case voice.EventCallStart:
				fmt.Printf("voice: call from %d to %d\n", e.Call.Source, e.Call.Target)
			case voice.EventCallEnd:
				fmt.Printf("voice: end, %v\n", e.Reason)
			default:
			}
		}
		for _, e := range dev {
			fmt.Println("data:", e.Kind)
		}
		for _, o := range pout {
			fmt.Println("preambles:", o.Kind)
		}
	}
	// Output:
	// voice: call from 3191868 to 9990
	// voice: end, Terminator
}
