package bs_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func ExampleConfig_channelAuth() {
	r, err := bs.New(&bs.Config{ColorCode: 1, CACH: true, CallHang: -1, ChannelAuth: bs.ChannelAuthSingleSite})
	if err != nil {
		fmt.Println(err)
		return
	}
	wake, _ := layer2.BuildBurst(&pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKBSOutboundActivation, RawPayload: [8]byte{4: 1, 5: 0x30, 6: 0xB4, 7: 0x3C}}, 1, layer2.SyncSourceMS)
	lc, _ := pdu.NewVoiceLC(true, 9, 3120005, layer3.ServiceOptions{})
	tx, _ := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot()})
	vh := tx.AppendHeaders(nil)[0]
	out := make([]bs.Output, 0, bs.MaxOutputs)
	headers := 0
	for now := time.Duration(0); now < 1500*time.Millisecond; now += bs.SlotTime {
		switch now {
		case 0:
			_, _ = r.Inbound(now, &wake, layer2.Hint{Slot: enums.Slot1}, bs.Source{RF: true}, out[:0])
		case 300 * time.Millisecond:
			_, _ = r.Inbound(now, &vh, layer2.Hint{Slot: enums.Slot1}, bs.Source{RF: true}, out[:0])
			fmt.Println(r.SlotState(enums.Slot1))
		}
		for _, o := range r.Tick(now, out[:0]) {
			if o.Kind == bs.OutputBurst && o.Slot == enums.Slot1 && o.Burst.SlotType().DataType == enums.DataTypeVoiceLCHeader {
				headers++
			}
		}
	}
	fmt.Println(r.SlotState(enums.Slot1), headers)
	// Output:
	// ChanAuth
	// Idle 12
}
