package layer2_test

import (
	"errors"
	"fmt"
	"time"

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

// ExampleNewBurstFromBytes decodes a Voice LC Header burst, reads
// the addresses of its Full LC and encodes it back to the same bytes.
func ExampleNewBurstFromBytes() {
	burst, err := layer2.NewBurstFromBytes(voiceLCHeader())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(burst.Kind, burst.SlotType.DataType)
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Printf("call from %d to %d\n", source, target)
	}
	encoded, _ := burst.Encode()
	fmt.Println("unchanged:", encoded == voiceLCHeader())
	// Output:
	// Data Voice LC Header
	// call from 3191868 to 9990
	// unchanged: true
}

// ExampleBurst_DecodeFromBytesWith rejects a burst with a color code
// that is not the expected one.
func ExampleBurst_DecodeFromBytesWith() {
	cc := enums.ColorCode(7)
	var b layer2.Burst
	err := b.DecodeFromBytesWith(voiceLCHeader(), &layer2.DecodeOptions{ColorCode: &cc})
	fmt.Println(errors.Is(err, layer2.ErrColorCodeMismatch), err)
	// Output:
	// true layer2: color code mismatch: burst has 1, want 7
}

// ExampleRawBurst reads the SYNC, the slot type and the BPTC payload of
// a burst without a full decode.
func ExampleRawBurst() {
	r := layer2.RawBurst(voiceLCHeader())
	sync, dist := r.Sync()
	st := r.SlotType()
	payload, res := r.BPTCPayload()
	fmt.Println(sync, dist, st.ColorCode, st.DataType)
	fmt.Printf("%x %d\n", payload, res.ErrorsCorrected)
	// Output:
	// BS-sourced data 0 1 Voice LC Header
	// 03000000270630b43c5dcfc1 0
}

// ExampleBuildDataBurst builds a Terminator with LC burst from a
// Full LC codeword and decodes it back.
func ExampleBuildDataBurst() {
	lc := pdu.LCBytes{0x00, 0x00, 0x00, 0x00, 0x00, 0x5b, 0x00, 0x30, 0x39}
	cw, err := lc.Codeword(enums.DataTypeTerminatorWithLC)
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

// ExampleVoiceTransmitter builds the bursts of a short group voice
// call: the header, one superframe of silence and the terminator.
func ExampleVoiceTransmitter() {
	var lc pdu.FullLinkControl
	if err := lc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3120000}); err != nil {
		fmt.Println(err)
		return
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
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

// ExamplePositionTracker_Place counts the bursts lost when an IPSC repeater
// sends one sync lost packet for the bursts it did not receive: the arrival
// times give the count that the sequence numbers do not.
func ExamplePositionTracker_Place() {
	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	a, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS)
	if err != nil {
		fmt.Println(err)
		return
	}
	var tr layer2.PositionTracker
	start := 10 * time.Second
	h := layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA, Seq: 100, HasSeq: true, At: start}
	tr.Place(&a, &h)
	h = layer2.Hint{Seq: 101, HasSeq: true, SyncLost: true, At: start + 60*time.Millisecond}
	tr.Place(nil, &h)
	h = layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA, Seq: 102, HasSeq: true, At: start + 720*time.Millisecond}
	lost, _ := tr.Place(&a, &h)
	fmt.Println(h.VoiceBurst, lost)
	// Output: A 11
}
