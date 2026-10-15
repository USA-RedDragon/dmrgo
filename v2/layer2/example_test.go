package layer2_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func voiceLCHeader() [33]byte {
	return [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}
}

func ExampleNewBurstFromBytes() {
	burst, err := layer2.NewBurstFromBytes(voiceLCHeader())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(burst.Kind, burst.SlotType.DataType)
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Printf("%d -> %d\n", source, target)
	}
	encoded, _ := burst.Encode()
	fmt.Println("unchanged:", encoded == voiceLCHeader())
	// Output:
	// Data Voice LC Header
	// 3191868 -> 9990
	// unchanged: true
}

func ExampleBurst_DecodeFromBytesWith() {
	cc := uint8(7)
	var b layer2.Burst
	err := b.DecodeFromBytesWith(voiceLCHeader(), layer2.DecodeOptions{ColorCode: &cc})
	fmt.Println(errors.Is(err, layer2.ErrColorCodeMismatch), err)
	// Output:
	// true color code mismatch: burst has 1, want 7
}

func ExampleRawBurst() {
	r := layer2.RawBurst(voiceLCHeader())
	sync, dist := r.Sync()
	st, _ := r.SlotType()
	payload, res := r.BPTCPayload()
	fmt.Println(sync, dist, st.ColorCode, st.DataType)
	fmt.Printf("%x %d\n", payload, res.ErrorsCorrected)
	// Output:
	// BS-sourced data 0 1 Voice LC Header
	// 03000000270630b43c5dcfc1 0
}

func ExampleBuildDataBurst() {
	lc := [9]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x5b, 0x00, 0x30, 0x39}
	cw, err := pdu.FullLCCodeword(lc, enums.DataTypeTerminatorWithLC)
	if err != nil {
		fmt.Println(err)
		return
	}
	raw, err := layer2.BuildDataBurst(cw, enums.DataTypeTerminatorWithLC, 1, layer2.SyncSourceBS)
	if err != nil {
		fmt.Println(err)
		return
	}
	b, _ := layer2.NewBurstFromBytes(raw)
	tlc, _ := b.Data.(*pdu.FullLinkControl)
	target, source, _ := tlc.Addresses()
	fmt.Println(b.SlotType.DataType, b.FEC.PDU.Uncorrectable, source, target)
	// Output:
	// Terminator With LC false 12345 91
}

func ExampleVoiceTransmitter() {
	lc := pdu.FullLinkControl{
		FLCO:       enums.FLCOGroupVoiceChannelUser,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120000},
	}
	tx, err := layer2.NewVoiceTransmitter(lc, 1, layer2.SyncSourceBS, vocoder.EncodeAMBE72(vocoder.AMBESilence()))
	if err != nil {
		fmt.Println(err)
		return
	}
	bursts := tx.Headers()
	frames := make([]vocoder.AMBEFrame, 18)
	for i := range frames {
		frames[i] = vocoder.AMBESilence()
	}
	voice, _ := tx.AddFrames(frames...)
	bursts = append(bursts, voice...)
	end, _ := tx.Terminate()
	bursts = append(bursts, end...)
	for _, raw := range bursts {
		b, _ := layer2.NewBurstFromBytes(raw)
		if b.Kind == layer2.KindData {
			fmt.Println(b.Kind, b.SlotType.DataType)
		} else {
			fmt.Println(b.Kind, b.SyncPattern)
		}
	}
	// Output:
	// Data Voice LC Header
	// Voice BS-sourced voice
	// Voice Embedded Signalling
	// Voice Embedded Signalling
	// Voice Embedded Signalling
	// Voice Embedded Signalling
	// Voice Embedded Signalling
	// Data Terminator With LC
}
