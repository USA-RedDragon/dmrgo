package voice_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func ExampleTransmitter_Burst() {
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{ColorCode: 1, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot(), ChanAuth: constants.TMSChanAuth})
	if err != nil {
		fmt.Println(err)
		return
	}
	call := voice.Call{Group: true, Target: 9, Source: 3120005}
	_ = tx.Start(0, call, &voice.CallOptions{HangTime: true})
	out := make([]voice.Output, 0, voice.MaxOutputs)
	for _, o := range tx.Tick(0, out[:0]) {
		fmt.Println(o.Kind, o.DataType, tx.State())
	}
	lc, _ := pdu.NewVoiceLC(true, 9, 3120005, layer3.ServiceOptions{})
	bs, _ := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
	repeated := bs.Header()
	tx.Burst(90*time.Millisecond, &repeated, layer2.Hint{}, out[:0])
	at, _ := tx.Deadline()
	fmt.Println(tx.State(), at)
	// Output:
	// Burst Voice LC Header ChanAuth
	// Header 90ms
}
