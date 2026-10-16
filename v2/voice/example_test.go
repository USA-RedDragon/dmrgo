package voice_test

import (
	"fmt"
	"os"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// ExampleReceiver follows the call in a capture of one logical channel,
// with the bursts 60 ms apart, and prints its events.
func ExampleReceiver() {
	data, err := os.ReadFile("../layer2/testdata/parrot_kerchunk.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	events := make([]voice.Event, 0, voice.MaxEvents)
	var now time.Duration
	voiceBursts := 0
	for off := 0; off+33 <= len(data); off += 33 {
		raw := layer2.RawBurst(data[off : off+33])
		now += 60 * time.Millisecond
		events = rx.Burst(now, &raw, layer2.Hint{}, events[:0])
		for _, e := range events {
			switch e.Kind {
			case voice.EventCallStart:
				fmt.Printf("call from %d to %d (%v)\n", e.Call.Source, e.Call.Target, e.Call.Origin)
			case voice.EventVoice:
				voiceBursts++
			case voice.EventCallEnd:
				fmt.Printf("end: %v after %v, %d voice bursts\n", e.Reason, e.Stats.Duration, voiceBursts)
			default:
			}
		}
	}
	if at, ok := rx.Deadline(); ok {
		for _, e := range rx.Tick(at, events[:0]) {
			fmt.Println(e.Kind, "at", at)
		}
	}
	// Output:
	// call from 3191868 to 9990 (Header)
	// end: Terminator after 840ms, 13 voice bursts
	// SessionEnd at 3.9s
}
