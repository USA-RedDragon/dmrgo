package voice_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func ExampleTransmitter_Channel() {
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Access: enums.AccessPoliteOwnCC})
	if err != nil {
		fmt.Println(err)
		return
	}
	tx.Channel(0, true)
	if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 3120001}, nil); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tx.State())
	tx.Channel(200*time.Millisecond, false)
	tx.Stop()
	out := make([]voice.Output, 0, voice.MaxOutputs)
	for {
		at, ok := tx.Deadline()
		if !ok {
			break
		}
		for _, o := range tx.Tick(at, out[:0]) {
			if o.Kind == voice.OutputEnded {
				fmt.Println(o.Kind, o.Reason, o.Due)
			}
		}
	}
	tx.Channel(time.Second, true)
	_ = tx.Start(time.Second, voice.Call{Group: true, Target: 9, Source: 3120001}, nil)
	at, _ := tx.Deadline()
	for _, o := range tx.Tick(at, out[:0]) {
		fmt.Println(o.Kind, o.Reason, o.Due)
	}
	// Output:
	// Access
	// Ended Terminator 260ms
	// Ended Denied 1.54s
}
